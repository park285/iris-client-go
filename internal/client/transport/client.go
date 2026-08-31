package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	jsonv1 "encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/park285/iris-client-go/v2/internal/baseendpoint"
	clientmultipart "github.com/park285/iris-client-go/v2/internal/client/multipart"
	"github.com/park285/iris-client-go/v2/internal/client/randomhex"
	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

type SecretRole int

const (
	SecretRoleInbound SecretRole = iota
	SecretRoleBotControl
	SecretRoleCertReload
)

type authSecrets struct {
	inboundSecret   string
	botControlToken string
	certReloadToken string
	sharedSecret    string
}

type APIClient struct {
	baseURL         string
	botToken        string
	auth            authSecrets
	signers         map[string]*signing.HMACSigner
	client          *http.Client
	streamClient    *http.Client
	logger          *slog.Logger
	opts            clientOptions
	initErr         error
	closeMu         sync.Mutex
	transportCloser io.Closer
	cachedProbe     atomic.Value
}

func NewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	o := applyClientOptions(opts)

	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sharedSecret := o.hmacSecret
	if sharedSecret == "" {
		sharedSecret = botToken
	}

	parsedBaseEndpoint, parseErr := baseendpoint.Parse(baseURL)
	if parseErr == nil {
		baseURL = parsedBaseEndpoint.String()
	} else {
		baseURL = ""
	}

	var (
		httpClient      *http.Client
		transportCloser io.Closer
		initErr         error
	)

	if parseErr != nil {
		initErr = fmt.Errorf("iris: invalid base endpoint: %w", parseErr)
		httpClient = cloneHTTPClientWithRedirectPolicy(&http.Client{
			Timeout:   o.Timeout,
			Transport: errorRoundTripper{err: initErr},
		})
	} else {
		httpClient, transportCloser, initErr = resolveHTTPClient(baseURL, o)
	}

	streamClient := cloneHTTPClient(httpClient)

	streamClient.Timeout = 0

	auth := authSecrets{
		inboundSecret:   o.inboundSecret,
		botControlToken: o.botControlToken,
		certReloadToken: o.certReloadToken,
		sharedSecret:    sharedSecret,
	}

	return &APIClient{
		baseURL:         baseURL,
		botToken:        botToken,
		auth:            auth,
		signers:         buildHMACSigners(auth),
		client:          httpClient,
		streamClient:    streamClient,
		logger:          logger,
		opts:            o,
		initErr:         initErr,
		transportCloser: transportCloser,
	}
}

func buildHMACSigners(auth authSecrets) map[string]*signing.HMACSigner {
	signers := make(map[string]*signing.HMACSigner, 4)

	for _, secret := range []string{
		strings.TrimSpace(auth.inboundSecret),
		strings.TrimSpace(auth.botControlToken),
		strings.TrimSpace(auth.certReloadToken),
		strings.TrimSpace(auth.sharedSecret),
	} {
		if secret == "" {
			continue
		}

		if _, ok := signers[secret]; !ok {
			signers[secret] = signing.NewHMACSigner(secret)
		}
	}

	return signers
}

func resolveHTTPClient(baseURL string, opts clientOptions) (*http.Client, io.Closer, error) {
	if opts.HTTPClient != nil {
		return cloneHTTPClientWithRedirectPolicy(opts.HTTPClient), nil, nil //nolint:nilnil // 호출자가 소유한 클라이언트는 닫을 closer가 없다.
	}

	if opts.RoundTripper != nil {
		return cloneHTTPClientWithRedirectPolicy(&http.Client{ //nolint:nilnil // 호출자가 소유한 RoundTripper는 닫을 closer가 없다.
			Timeout:   opts.Timeout,
			Transport: opts.RoundTripper,
		}), nil, nil
	}

	httpClient, closer, err := newHTTPClientWithCloser(baseURL, opts)
	if err != nil {
		// 초기화에 실패해도 호출마다 initErr를 돌려주는 클라이언트를 함께 넘긴다.
		return cloneHTTPClientWithRedirectPolicy(&http.Client{ //nolint:nilnil // errorRoundTripper 클라이언트와 initErr를 함께 돌려주는 계약이다.
			Timeout:   opts.Timeout,
			Transport: errorRoundTripper{err: err},
		}), nil, err
	}

	return httpClient, closer, nil
}

var _ Sender = (*APIClient)(nil)

func (c *APIClient) SendMessage(ctx context.Context, room, message string, opts ...SendOption) error {
	reqBody, err := newTextReplyRequest(room, message, opts)
	if err != nil {
		return fmt.Errorf("validate send options: %w", err)
	}

	if err := c.postDiscard(ctx, PathReply, reqBody, SecretRoleBotControl); err != nil {
		return fmt.Errorf("send iris reply: %w", err)
	}

	return nil
}

func (c *APIClient) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.transportCloser == nil {
		return nil
	}

	err := c.transportCloser.Close()

	c.transportCloser = nil

	if err != nil {
		return fmt.Errorf("close transport: %w", err)
	}

	return nil
}

func (c *APIClient) InitError() error {
	return c.initErr
}

func (c *APIClient) SendMessageAccepted(ctx context.Context, room, message string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	reqBody, err := newTextReplyRequest(room, message, opts)
	if err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris reply: %w", err)
	}

	return resp, nil
}

func newTextReplyRequest(room, message string, opts []SendOption) (ReplyRequest, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return ReplyRequest{}, err //nolint:wrapcheck // 호출자가 validate send options로 감싼다.
	}

	return ReplyRequest{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeText,
		Room:            room,
		Data:            message,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Mentions:        cloneReplyMentions(o.Mentions),
		AttachmentJSON:  normalizeAttachmentJSON(o.AttachmentJSON),
	}, nil
}

func (c *APIClient) SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	if err := validateImageReplyOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	images := [][]byte{imageData}
	if err := clientmultipart.ValidateReplyImages(images); err != nil {
		return nil, fmt.Errorf("validate image payload: %w", err)
	}

	contentTypes, err := imageContentTypesForSend(images, o.ImageContentType)
	if err != nil {
		return nil, fmt.Errorf("validate image content type: %w", err)
	}

	metadata := replyImageMetadata{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeImage,
		Room:            room,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Images:          buildImageManifest(images, contentTypes),
	}

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, contentTypes, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris image: %w", err)
	}

	return resp, nil
}

func (c *APIClient) SendMultipleImages(ctx context.Context, room string, images [][]byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	if err := validateImageReplyOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	if err := clientmultipart.ValidateReplyImages(images); err != nil {
		return nil, fmt.Errorf("validate image payloads: %w", err)
	}

	if o.ImageContentType != nil {
		return nil, fmt.Errorf("validate image content type: %w", errors.New("iris: imageContentType is supported only for SendImage"))
	}

	contentTypes, err := imageContentTypesForSend(images, nil)
	if err != nil {
		return nil, fmt.Errorf("validate image content type: %w", err)
	}

	metadata := replyImageMetadata{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeImageMultiple,
		Room:            room,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Images:          buildImageManifest(images, contentTypes),
	}

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, contentTypes, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris multiple images: %w", err)
	}

	return resp, nil
}

func (c *APIClient) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	return c.doGet[ConfigResponse](ctx, PathConfig, SecretRoleInbound)
}

func (c *APIClient) SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	if hasAttachmentJSON(o.AttachmentJSON) {
		return nil, fmt.Errorf("validate send options: %w", errAttachmentJSONRequiresText)
	}

	reqBody := ReplyRequest{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeMarkdown,
		Room:            room,
		Data:            markdown,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Mentions:        cloneReplyMentions(o.Mentions),
	}

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris reply-markdown: %w", err)
	}

	return resp, nil
}

func (c *APIClient) GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error) {
	path, err := appendSafePathSegment(PathReplyStatus, "request ID", requestID)
	if err != nil {
		return nil, fmt.Errorf("get reply status: %w", err)
	}

	return c.doGet[ReplyStatusSnapshot](ctx, path, SecretRoleBotControl)
}

func (c *APIClient) UpdateConfig(ctx context.Context, name string, cfgReq ConfigUpdateRequest) (*ConfigUpdateResponse, error) {
	path, err := appendSafePathSegment(PathConfig, "config name", name)
	if err != nil {
		return nil, fmt.Errorf("update config: %w", err)
	}

	resp, err := c.postJSON[ConfigUpdateResponse](ctx, path, cfgReq, SecretRoleInbound)
	if err != nil {
		return nil, fmt.Errorf("update config %s: %w", name, err)
	}

	return resp, nil
}

func (c *APIClient) GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error) {
	return c.doGet[BridgeHealthResult](ctx, PathDiagnosticsBridge, SecretRoleBotControl)
}

func (c *APIClient) GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error) {
	return c.doGet[NativeCoreDiagnostics](ctx, PathDiagnosticsNativeCore, SecretRoleBotControl)
}

func (c *APIClient) GetRuntimeDiagnostics(ctx context.Context) (jsonv1.RawMessage, error) {
	raw, err := c.rawJSON(ctx, http.MethodGet, PathDiagnosticsRuntime, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("get runtime diagnostics: %w", err)
	}

	return raw, nil
}

func (c *APIClient) GetChatroomFields(ctx context.Context, chatID int64) (jsonv1.RawMessage, error) {
	raw, err := c.rawJSON(ctx, http.MethodGet, PathDiagnosticsChatroom+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("get chatroom fields: %w", err)
	}

	return raw, nil
}

func (c *APIClient) OpenChatroom(ctx context.Context, chatID int64) (jsonv1.RawMessage, error) {
	raw, err := c.rawJSON(ctx, http.MethodPost, PathDiagnosticsChatroomOpen+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("open chatroom: %w", err)
	}

	return raw, nil
}

func (c *APIClient) GetTextPingDiagnostics(ctx context.Context, chatID int64) (jsonv1.RawMessage, error) {
	raw, err := c.rawJSON(ctx, http.MethodGet, PathDiagnosticsTextPing+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("get text-ping diagnostics: %w", err)
	}

	return raw, nil
}

func (c *APIClient) WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error) {
	path := PathDiagnosticsTextPing + "/" + strconv.FormatInt(chatID, 10) + "/warm"

	resp, err := c.postJSON[TextPingWarmResponse](ctx, path, nil, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("warm text-ping %d: %w", chatID, err)
	}

	return resp, nil
}

func (c *APIClient) ReloadH3Certificate(ctx context.Context) (*CertReloadResponse, error) {
	raw, err := c.rawJSON(ctx, http.MethodPost, PathAdminCertReload, SecretRoleCertReload)
	if err != nil {
		return nil, fmt.Errorf("reload h3 certificate: %w", err)
	}

	var resp CertReloadResponse

	if err := jsonv2.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("reload h3 certificate: decode response: %w", err)
	}

	return &resp, nil
}

func (c *APIClient) QueryRoomSummary(ctx context.Context, chatID int64) (*RoomSummary, error) {
	resp, err := c.postJSON[RoomSummary](ctx, PathQueryRoomSummary, QueryRoomSummaryRequest{ChatID: chatID}, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("query room summary: %w", err)
	}

	return resp, nil
}

func (c *APIClient) QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error) {
	resp, err := c.postJSON[StatsResponse](ctx, PathQueryMemberStats, req, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("query member stats: %w", err)
	}

	return resp, nil
}

func (c *APIClient) QueryRecentThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error) {
	resp, err := c.postJSON[ThreadListResponse](ctx, PathQueryRecentThreads, QueryRecentThreadsRequest{ChatID: chatID}, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("query recent threads: %w", err)
	}

	return resp, nil
}

func (c *APIClient) QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error) {
	resp, err := c.postJSON[RecentMessagesResponse](ctx, PathQueryRecentMessages, req, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}

	return resp, nil
}

func (c *APIClient) postJSON[T any](ctx context.Context, path string, body any, role SecretRole) (*T, error) {
	buildRequest, err := c.newSignedJSONRequest(path, body, role)
	if err != nil {
		return nil, err //nolint:wrapcheck // newSignedJSONRequest가 post 경로 맥락으로 이미 래핑한다.
	}

	return c.retryPostJSON[T](ctx, path, requestHasClientRequestID(body), buildRequest)
}

func (c *APIClient) postDiscard(ctx context.Context, path string, body any, role SecretRole) error {
	buildRequest, err := c.newSignedJSONRequest(path, body, role)
	if err != nil {
		return err //nolint:wrapcheck // newSignedJSONRequest가 post 경로 맥락으로 이미 래핑한다.
	}

	return c.retryPostDiscard(ctx, path, requestHasClientRequestID(body), buildRequest) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
}

func (c *APIClient) newSignedJSONRequest(path string, body any, role SecretRole) (requestBuilder, error) {
	payload, err := jsonv2.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	return func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newSignedRequest(attemptCtx, http.MethodPost, path, payload, role)
		if err != nil {
			return nil, fmt.Errorf("post %s: %w", path, err)
		}

		req.Header.Set("Content-Type", contentTypeJSON)

		return req, nil
	}, nil
}

func (c *APIClient) postMultipart(
	ctx context.Context,
	path string,
	metadata replyImageMetadata,
	images [][]byte,
	contentTypes []string,
	role SecretRole,
) (*ReplyAcceptedResponse, error) {
	metadataBytes, err := jsonv2.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode metadata: %w", path, err)
	}

	bodyFactory, err := clientmultipart.NewBodyFactory(generateMultipartBoundary(), metadataBytes, images, contentTypes)
	if err != nil {
		return nil, fmt.Errorf("validate multipart envelope: %w", err)
	}

	return c.retryPostJSON[ReplyAcceptedResponse](ctx, path, metadata.ClientRequestID != nil, func(attemptCtx context.Context) (*http.Request, error) {
		body, err := bodyFactory.NewBody()
		if err != nil {
			return nil, fmt.Errorf("post %s: create multipart body: %w", path, err)
		}

		req, err := c.newSignedStreamRequest(attemptCtx, http.MethodPost, path, body, bodyFactory.BodySHA256(), role)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("post %s: %w", path, err), body.Close())
		}

		req.Header.Set("Content-Type", bodyFactory.ContentType())

		req.ContentLength = bodyFactory.BodyLength()
		req.GetBody = bodyFactory.NewBody

		return req, nil
	})
}

func generateMultipartBoundary() string {
	return randomhex.Generate()
}

func normalizeClientRequestID(id *string) *string {
	if id == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*id)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

// 이 판정은 postWithRetry의 재시도 가능 여부에만 쓰이고, 재시도는 PathReply에서만 켜진다.
// Karing 요청 타입은 그 경로에 닿지 않으므로 여기서 다루지 않는다.
func requestHasClientRequestID(body any) bool {
	request, ok := body.(ReplyRequest)
	return ok && normalizeClientRequestID(request.ClientRequestID) != nil
}

func readErrorResponse(path string, resp *http.Response) error {
	payload := readErrorBody(resp.Body)
	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		URL:        path,
		RetryAfter: parseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now()),
		Body:       truncateErrorBody(payload),
	}

	return withHTTPErrorCode(httpErr, parseHTTPErrorCode(string(payload))) //nolint:wrapcheck // HTTPError 값을 조립하는 생성자 호출이다.
}

func (c *APIClient) newSignedRequest(ctx context.Context, method, path string, bodyBytes []byte, role SecretRole) (*http.Request, error) {
	var body io.Reader

	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}

	return c.newSignedStreamRequest(ctx, method, path, body, signing.SHA256HexBytes(bodyBytes), role) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
}

func (c *APIClient) newSignedStreamRequest(ctx context.Context, method, path string, body io.Reader, bodySHA256 string, role SecretRole) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	secret := c.secretFor(role)
	if secret == "" {
		switch role {
		case SecretRoleCertReload:
			return nil, ErrCertReloadTokenRequired
		case SecretRoleInbound:
			return nil, ErrInboundSecretRequired
		case SecretRoleBotControl:
		}
	} else if err := signing.SetIrisHMACHeaders(req, c.signerFor(secret), method, path, bodySHA256); err != nil {
		return nil, fmt.Errorf("sign iris request: %w", err)
	}

	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))

	return req, nil
}

func (c *APIClient) signerFor(secret string) *signing.HMACSigner {
	if signer, ok := c.signers[secret]; ok {
		return signer
	}

	return signing.NewHMACSigner(secret)
}

func (c *APIClient) secretFor(role SecretRole) string {
	switch role {
	case SecretRoleInbound:
		if s := strings.TrimSpace(c.auth.inboundSecret); s != "" {
			return s
		}

		// 서버 /config*는 inbound 역할 비밀키로만 검증한다. bot token(=botControl 자격)으로
		// 폴백하면 진단 불가능한 401이 되므로, 명시적 shared secret(WithHMACSecret)만 허용한다.
		return strings.TrimSpace(c.opts.hmacSecret)
	case SecretRoleBotControl:
		if s := strings.TrimSpace(c.auth.botControlToken); s != "" {
			return s
		}
	case SecretRoleCertReload:
		return strings.TrimSpace(c.auth.certReloadToken)
	}

	return strings.TrimSpace(c.auth.sharedSecret)
}

func detectImageContentType(data []byte) string {
	switch {
	case len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return mimeImagePNG
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp"
	case len(data) >= 4 && string(data[0:4]) == "GIF8":
		return "image/gif"
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return "video/mp4"
	default:
		return mimeApplicationOctetStream
	}
}

func buildImageManifest(images [][]byte, contentTypes []string) []imagePartSpec {
	specs := make([]imagePartSpec, len(images))
	for i, img := range images {
		hash := sha256.Sum256(img)

		specs[i] = imagePartSpec{
			Index:       i,
			SHA256Hex:   hex.EncodeToString(hash[:]),
			ByteLength:  int64(len(img)),
			ContentType: contentTypes[i],
		}
	}

	return specs
}

func imageContentTypesForSend(images [][]byte, explicitContentType *string) ([]string, error) {
	contentTypes := make([]string, len(images))

	if explicitContentType != nil {
		contentType, err := normalizeReplyMediaContentType(*explicitContentType)
		if err != nil {
			return nil, err //nolint:wrapcheck // 검증 오류가 필드 맥락을 이미 담고 있어 그대로 전달한다.
		}

		if len(images) != 1 {
			return nil, errors.New("iris: imageContentType is supported only for SendImage")
		}

		contentTypes[0] = contentType

		return contentTypes, nil
	}

	for i, image := range images {
		contentTypes[i] = detectImageContentType(image)
	}

	return contentTypes, nil
}

func normalizeReplyMediaContentType(contentType string) (string, error) {
	normalized := strings.TrimSpace(contentType)
	if idx := strings.IndexByte(normalized, ';'); idx >= 0 {
		normalized = normalized[:idx]
	}

	normalized = strings.ToLower(strings.TrimSpace(normalized))
	if !isAllowedReplyMediaContentType(normalized) {
		return "", fmt.Errorf("iris: unsupported image content type %q", contentType)
	}

	return normalized, nil
}

func isAllowedReplyMediaContentType(contentType string) bool {
	switch contentType {
	case mimeImagePNG, "image/jpeg", "image/webp", "image/gif", "video/mp4":
		return true
	default:
		return false
	}
}
