package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/park285/iris-client-go/v3/internal/baseendpoint"
	clientmultipart "github.com/park285/iris-client-go/v3/internal/client/multipart"
	"github.com/park285/iris-client-go/v3/internal/client/randomhex"
	"github.com/park285/iris-client-go/v3/internal/client/signing"
)

type APIClient struct {
	baseURL         string
	signer          *signing.HMACSigner
	client          *http.Client
	logger          *slog.Logger
	opts            clientOptions
	initErr         error
	closeMu         sync.Mutex
	transportCloser io.Closer
}

func NewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	return newAPIClient(baseURL, botToken, applyClientOptions(opts))
}

// newAPIClient는 이미 적용·정규화된 옵션으로 client를 만든다. 옵션 클로저를 다시 실행하지 않는다.
func newAPIClient(baseURL, botToken string, o clientOptions) *APIClient {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	parsedBaseEndpoint, parseErr := baseendpoint.Parse(baseURL)
	if parseErr == nil {
		baseURL = parsedBaseEndpoint.String()
	} else {
		baseURL = ""
	}

	var (
		httpClient      http.Client
		transportCloser io.Closer
		initErr         error
	)

	if parseErr != nil {
		initErr = fmt.Errorf("iris: invalid base endpoint: %w", parseErr)
		httpClient = *cloneHTTPClientWithRedirectPolicy(&http.Client{
			Timeout:   o.Timeout,
			Transport: errorRoundTripper{err: initErr},
		})
	} else {
		httpClient, transportCloser, initErr = resolveHTTPClient(baseURL, o)
	}

	return &APIClient{
		baseURL:         baseURL,
		signer:          newBotControlSigner(botToken),
		client:          &httpClient,
		logger:          logger,
		opts:            o,
		initErr:         initErr,
		transportCloser: transportCloser,
	}
}

// newBotControlSigner는 bot-control 자격의 서명기를 한 번만 만든다. 자격이 비면 요청을 서명하지 않는다.
func newBotControlSigner(botToken string) *signing.HMACSigner {
	secret := strings.TrimSpace(botToken)
	if secret == "" {
		return nil
	}

	return signing.NewHMACSigner(secret)
}

// 초기화 실패도 errorRoundTripper를 제공하므로 client 값은 항상 존재한다.
func resolveHTTPClient(baseURL string, opts clientOptions) (http.Client, io.Closer, error) {
	if opts.HTTPClient != nil {
		return *cloneHTTPClientWithRedirectPolicy(opts.HTTPClient), nil, nil //nolint:nilnil // 호출자가 소유한 클라이언트는 닫을 closer가 없다.
	}

	httpClient, closer, err := newHTTPClientWithCloser(baseURL, opts)
	if err != nil {
		// 초기화에 실패해도 호출마다 initErr를 돌려주는 클라이언트를 함께 넘긴다.
		return *cloneHTTPClientWithRedirectPolicy(&http.Client{
			Timeout:   opts.Timeout,
			Transport: errorRoundTripper{err: err},
		}), nil, err
	}

	return *httpClient, closer, nil
}

var _ Sender = (*APIClient)(nil)

func (c *APIClient) SendMessage(ctx context.Context, room, message string, opts ...SendOption) error {
	reqBody, err := newTextReplyRequest(room, message, opts)
	if err != nil {
		return fmt.Errorf("validate send options: %w", err)
	}

	if err := c.postDiscard(ctx, PathReply, reqBody); err != nil {
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

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody)
	if err != nil {
		return nil, fmt.Errorf("send iris reply: %w", err)
	}

	return resp, nil
}

func newTextReplyRequest(room, message string, opts []SendOption) (ReplyRequest, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return ReplyRequest{}, err
	}

	return ReplyRequest{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeText,
		Room:            room,
		Data:            message,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Mentions:        cloneReplyMentions(o.Mentions),
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

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, contentTypes)
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

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, contentTypes)
	if err != nil {
		return nil, fmt.Errorf("send iris multiple images: %w", err)
	}

	return resp, nil
}

func (c *APIClient) SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
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

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody)
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

	return c.doGet[ReplyStatusSnapshot](ctx, path)
}

func (c *APIClient) QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error) {
	resp, err := c.postJSON[StatsResponse](ctx, PathQueryMemberStats, req)
	if err != nil {
		return nil, fmt.Errorf("query member stats: %w", err)
	}

	return resp, nil
}

func (c *APIClient) QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error) {
	resp, err := c.postJSON[RecentMessagesResponse](ctx, PathQueryRecentMessages, req)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}

	return resp, nil
}

func (c *APIClient) postJSON[T any](ctx context.Context, path string, body any) (*T, error) {
	buildRequest, err := c.newSignedJSONRequest(path, body)
	if err != nil {
		return nil, err
	}

	return c.retryPostJSON[T](ctx, path, requestHasClientRequestID(body), buildRequest)
}

func (c *APIClient) postDiscard(ctx context.Context, path string, body any) error {
	buildRequest, err := c.newSignedJSONRequest(path, body)
	if err != nil {
		return err
	}

	return c.retryPostDiscard(ctx, path, requestHasClientRequestID(body), buildRequest)
}

func (c *APIClient) newSignedJSONRequest(path string, body any) (requestBuilder, error) {
	payload, err := jsonv2.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	// 본문 해시는 재시도마다 같으므로 builder 밖에서 한 번만 계산한다.
	bodySHA256 := signing.SHA256HexBytes(payload)

	return func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newSignedStreamRequest(attemptCtx, http.MethodPost, path, bytes.NewReader(payload), bodySHA256)
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

		req, err := c.newSignedStreamRequest(attemptCtx, http.MethodPost, path, body, bodyFactory.BodySHA256())
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

// 이 판정은 retryPost의 재시도 가능 여부에만 쓰이고, 재시도는 PathReply에서만 켜진다.
func requestHasClientRequestID(body any) bool {
	request, ok := body.(ReplyRequest)
	return ok && normalizeClientRequestID(request.ClientRequestID) != nil
}

func readErrorResponse(path string, resp *http.Response) error {
	payload, complete := readErrorBody(resp.Body)
	httpErr := &HTTPError{
		StatusCode: resp.StatusCode,
		URL:        path,
		RetryAfter: parseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now()),
		Body:       truncateErrorBody(payload),
	}

	if !complete {
		return httpErr
	}

	return withHTTPErrorCode(httpErr, parseHTTPErrorCode(string(payload)))
}

func (c *APIClient) newSignedRequest(ctx context.Context, method, path string, bodyBytes []byte) (*http.Request, error) {
	var body io.Reader

	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}

	return c.newSignedStreamRequest(ctx, method, path, body, signing.SHA256HexBytes(bodyBytes))
}

func (c *APIClient) newSignedStreamRequest(ctx context.Context, method, path string, body io.Reader, bodySHA256 string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if c.signer != nil {
		if err := signing.SetIrisHMACHeaders(req, c.signer, method, path, bodySHA256); err != nil {
			return nil, fmt.Errorf("sign iris request: %w", err)
		}
	}

	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))

	return req, nil
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
			return nil, err
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
