package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/park285/iris-client-go/internal/jsonx"
)

type SecretRole int

const (
	SecretRoleInbound SecretRole = iota
	SecretRoleBotControl
)

type authSecrets struct {
	inboundSecret   string
	botControlToken string
	sharedSecret    string
}

type H2CClient struct {
	baseURL         string
	botToken        string
	auth            authSecrets
	client          *http.Client
	logger          *slog.Logger
	opts            clientOptions
	initErr         error
	closeMu         sync.Mutex
	transportCloser io.Closer
	cachedProbe     atomic.Value
}

func NewH2CClient(baseURL, botToken string, opts ...ClientOption) *H2CClient {
	o := applyClientOptions(opts)

	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")

	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}

	sharedSecret := o.hmacSecret
	if sharedSecret == "" {
		sharedSecret = botToken
	}

	httpClient, transportCloser, initErr := resolveHTTPClient(baseURL, o)

	return &H2CClient{
		baseURL:  baseURL,
		botToken: botToken,
		auth: authSecrets{
			inboundSecret:   o.inboundSecret,
			botControlToken: o.botControlToken,
			sharedSecret:    sharedSecret,
		},
		client:          httpClient,
		logger:          logger,
		opts:            o,
		initErr:         initErr,
		transportCloser: transportCloser,
	}
}

func resolveHTTPClient(baseURL string, opts clientOptions) (*http.Client, io.Closer, error) {
	if opts.HTTPClient != nil {
		return opts.HTTPClient, nil, nil
	}

	if opts.RoundTripper != nil {
		return &http.Client{
			Timeout:   opts.Timeout,
			Transport: opts.RoundTripper,
		}, nil, nil
	}

	httpClient, closer, err := newHTTPClientWithCloser(baseURL, opts)
	if err != nil {
		return &http.Client{
			Timeout:   opts.Timeout,
			Transport: errorRoundTripper{err: err},
		}, nil, err
	}

	return httpClient, closer, nil
}

var (
	_ Sender           = (*H2CClient)(nil)
	_ AdminClient      = (*H2CClient)(nil)
	_ CertReloadClient = (*H2CClient)(nil)
)

var _ error = (*HTTPError)(nil)

func isRetryableError(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

var _ error = (*TransportError)(nil)

func isRetryableTransportError(err error) bool {
	return errors.Is(err, ErrTransport)
}

func (c *H2CClient) SendMessage(ctx context.Context, room, message string, opts ...SendOption) error {
	if _, err := c.sendMessage(ctx, room, message, nil, opts...); err != nil {
		return err
	}

	return nil
}

func (c *H2CClient) Close() error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.transportCloser == nil {
		return nil
	}

	err := c.transportCloser.Close()
	c.transportCloser = nil

	return err
}

func (c *H2CClient) InitError() error {
	return c.initErr
}

func (c *H2CClient) SendMessageAccepted(ctx context.Context, room, message string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	var resp ReplyAcceptedResponse
	if _, err := c.sendMessage(ctx, room, message, &resp, opts...); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (c *H2CClient) sendMessage(ctx context.Context, room, message string, resp *ReplyAcceptedResponse, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	reqBody := ReplyRequest{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeText,
		Room:            room,
		Data:            message,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Mentions:        cloneReplyMentions(o.Mentions),
		AttachmentJSON:  normalizeAttachmentJSON(o.AttachmentJSON),
	}
	var responseTarget any
	if resp != nil {
		responseTarget = resp
	}

	if err := c.postJSON(ctx, PathReply, reqBody, responseTarget, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris reply: %w", err)
	}

	return resp, nil
}

func (c *H2CClient) SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}
	if err := validateImageReplyOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	images := [][]byte{imageData}
	if err := validateReplyImages(images); err != nil {
		return nil, fmt.Errorf("validate image payload: %w", err)
	}

	metadata := replyImageMetadata{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeImage,
		Room:            room,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Images:          buildImageManifest(images),
	}

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris image: %w", err)
	}

	return resp, nil
}

func (c *H2CClient) SendMultipleImages(ctx context.Context, room string, images [][]byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}
	if err := validateImageReplyOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}
	if err := validateReplyImages(images); err != nil {
		return nil, fmt.Errorf("validate image payloads: %w", err)
	}

	metadata := replyImageMetadata{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeImageMultiple,
		Room:            room,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Images:          buildImageManifest(images),
	}

	resp, err := c.postMultipart(ctx, PathReply, metadata, images, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris multiple images: %w", err)
	}

	return resp, nil
}

func (c *H2CClient) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	return doGet[ConfigResponse](c, ctx, PathConfig, SecretRoleInbound)
}

func (c *H2CClient) SendMarkdown(ctx context.Context, room, markdown string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
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

	var resp ReplyAcceptedResponse
	if err := c.postJSON(ctx, PathReply, reqBody, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris reply-markdown: %w", err)
	}

	return &resp, nil
}

func (c *H2CClient) GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error) {
	path, err := appendSafePathSegment(PathReplyStatus, "request ID", requestID)
	if err != nil {
		return nil, fmt.Errorf("get reply status: %w", err)
	}
	return doGet[ReplyStatusSnapshot](c, ctx, path, SecretRoleBotControl)
}

func (c *H2CClient) UpdateConfig(ctx context.Context, name string, cfgReq ConfigUpdateRequest) (*ConfigUpdateResponse, error) {
	path, err := appendSafePathSegment(PathConfig, "config name", name)
	if err != nil {
		return nil, fmt.Errorf("update config: %w", err)
	}

	var resp ConfigUpdateResponse
	if err := c.postJSON(ctx, path, cfgReq, &resp, SecretRoleInbound); err != nil {
		return nil, fmt.Errorf("update config %s: %w", name, err)
	}

	return &resp, nil
}

func (c *H2CClient) GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error) {
	return doGet[BridgeHealthResult](c, ctx, PathDiagnosticsBridge, SecretRoleBotControl)
}

func (c *H2CClient) GetNativeCoreDiagnostics(ctx context.Context) (*NativeCoreDiagnostics, error) {
	return doGet[NativeCoreDiagnostics](c, ctx, PathDiagnosticsNativeCore, SecretRoleBotControl)
}

func (c *H2CClient) GetRuntimeDiagnostics(ctx context.Context) (jsonx.RawMessage, error) {
	return c.getRawJSON(ctx, PathDiagnosticsRuntime, SecretRoleBotControl)
}

func (c *H2CClient) GetChatroomFields(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	return c.getRawJSON(ctx, PathDiagnosticsChatroom+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
}

func (c *H2CClient) OpenChatroom(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	return c.postRawJSON(ctx, PathDiagnosticsChatroomOpen+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
}

func (c *H2CClient) GetTextPingDiagnostics(ctx context.Context, chatID int64) (jsonx.RawMessage, error) {
	return c.getRawJSON(ctx, PathDiagnosticsTextPing+"/"+strconv.FormatInt(chatID, 10), SecretRoleBotControl)
}

func (c *H2CClient) WarmTextPing(ctx context.Context, chatID int64) (*TextPingWarmResponse, error) {
	path := PathDiagnosticsTextPing + "/" + strconv.FormatInt(chatID, 10) + "/warm"
	var resp TextPingWarmResponse
	if err := c.postJSON(ctx, path, nil, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("warm text-ping %d: %w", chatID, err)
	}
	return &resp, nil
}

func (c *H2CClient) ReloadH3Certificate(ctx context.Context) (*CertReloadResponse, error) {
	raw, err := c.postRawJSON(ctx, PathAdminCertReload, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("reload h3 certificate: %w", err)
	}

	var resp CertReloadResponse
	if err := jsonx.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("reload h3 certificate: decode response: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) getRawJSON(ctx context.Context, path string, role SecretRole) (jsonx.RawMessage, error) {
	req, err := c.newSignedRequest(ctx, http.MethodGet, path, nil, role)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: "get", URL: req.URL.String(), Err: err}
	}
	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s: %w", path, readErrorResponse(path, resp))
	}
	return io.ReadAll(resp.Body)
}

func (c *H2CClient) postRawJSON(ctx context.Context, path string, role SecretRole) (jsonx.RawMessage, error) {
	req, err := c.newSignedRequest(ctx, http.MethodPost, path, nil, role)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: "post", URL: req.URL.String(), Err: err}
	}
	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("post %s: %w", path, readErrorResponse(path, resp))
	}
	return io.ReadAll(resp.Body)
}

type QueryClient interface {
	QueryRoomSummary(ctx context.Context, chatID int64) (*RoomSummary, error)
	QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error)
	QueryRecentThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error)
	QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error)
}

var _ QueryClient = (*H2CClient)(nil)

func (c *H2CClient) QueryRoomSummary(ctx context.Context, chatID int64) (*RoomSummary, error) {
	var resp RoomSummary
	if err := c.postJSON(ctx, PathQueryRoomSummary, QueryRoomSummaryRequest{ChatID: chatID}, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query room summary: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error) {
	var resp StatsResponse
	if err := c.postJSON(ctx, PathQueryMemberStats, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query member stats: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) QueryRecentThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error) {
	var resp ThreadListResponse
	if err := c.postJSON(ctx, PathQueryRecentThreads, QueryRecentThreadsRequest{ChatID: chatID}, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query recent threads: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error) {
	var resp RecentMessagesResponse
	if err := c.postJSON(ctx, PathQueryRecentMessages, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) postJSON(ctx context.Context, path string, body, out any, role SecretRole) error {
	payload, err := jsonx.Marshal(body)
	if err != nil {
		return fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	return c.postWithRetry(ctx, path, requestHasClientRequestID(body), func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newSignedRequest(attemptCtx, http.MethodPost, path, payload, role)
		if err != nil {
			return nil, fmt.Errorf("post %s: %w", path, err)
		}

		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, out)
}

func (c *H2CClient) postMultipart(ctx context.Context, path string, metadata replyImageMetadata, images [][]byte, role SecretRole) (*ReplyAcceptedResponse, error) {
	metadataBytes, err := jsonx.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode metadata: %w", path, err)
	}

	bodyFactory := newMultipartBodyFactory(metadataBytes, images)
	if err := validateReplyMultipartEnvelope(metadataBytes, bodyFactory.BodyLength()); err != nil {
		return nil, fmt.Errorf("validate multipart envelope: %w", err)
	}

	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, path, metadata.ClientRequestID != nil, func(attemptCtx context.Context) (*http.Request, error) {
		body, err := bodyFactory.NewBody()
		if err != nil {
			return nil, fmt.Errorf("post %s: create multipart body: %w", path, err)
		}

		req, err := c.newSignedStreamRequest(attemptCtx, http.MethodPost, path, body, bodyFactory.BodySHA256(), role)
		if err != nil {
			_ = body.Close()
			return nil, fmt.Errorf("post %s: %w", path, err)
		}
		req.Header.Set("Content-Type", bodyFactory.ContentType())
		req.ContentLength = bodyFactory.BodyLength()
		req.GetBody = bodyFactory.NewBody
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func generateMultipartBoundary() string {
	return generateRandomHex16("iris-multipart")
}

func (c *H2CClient) postWithRetry(
	ctx context.Context,
	path string,
	hasIdempotencyKey bool,
	buildRequest func(context.Context) (*http.Request, error),
	out any,
) error {
	maxAttempts := 1
	if c.opts.ReplyRetryMax > 0 && path == PathReply {
		maxAttempts = c.opts.ReplyRetryMax
	}

	backoff := 50 * time.Millisecond
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := buildRequest(ctx)
		if err != nil {
			return err
		}

		err = c.doRequest(req, path, out)
		if err == nil {
			return nil
		}

		if !isRetryableReplyError(err, hasIdempotencyKey) || attempt == maxAttempts {
			return err
		}

		timer := time.NewTimer(retryDelayForError(err, backoff))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		backoff = min(backoff*2, time.Second)
	}

	return fmt.Errorf("post %s: retries exhausted", path)
}

func isRetryableReplyError(err error, hasIdempotencyKey bool) bool {
	return isRetryableError(err) || hasIdempotencyKey && isRetryableTransportError(err)
}

func (c *H2CClient) doRequest(req *http.Request, path string, out any) error {
	if c.initErr != nil {
		return &TransportError{Op: "init", URL: path, Err: c.initErr}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &TransportError{Op: "post", URL: path, Err: err}
	}

	defer func() {
		//nolint:errcheck,gosec // Best-effort body close on deferred path.
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("post %s: %w", path, readErrorResponse(path, resp))
	}

	if out == nil {
		//nolint:errcheck,gosec // Best-effort drain.
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := jsonx.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
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

func requestHasClientRequestID(body any) bool {
	switch request := body.(type) {
	case ReplyRequest:
		return normalizeClientRequestID(request.ClientRequestID) != nil
	case KaringSendRequest:
		return normalizeClientRequestID(request.ClientRequestID) != nil
	case KaringContentListRequest:
		return normalizeClientRequestID(request.ClientRequestID) != nil
	case KaringHololiveRequest:
		return normalizeClientRequestID(request.ClientRequestID) != nil
	default:
		return false
	}
}

func readErrorResponse(path string, resp *http.Response) error {
	return &HTTPError{
		StatusCode: resp.StatusCode,
		URL:        path,
		RetryAfter: parseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now()),
		Body:       truncateBody(resp.Body),
	}
}

func (c *H2CClient) newSignedRequest(ctx context.Context, method, path string, bodyBytes []byte, role SecretRole) (*http.Request, error) {
	var body io.Reader
	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret := c.secretFor(role); secret != "" {
		if err := setIrisHMACHeaders(req, secret, method, path, sha256HexBytes(bodyBytes)); err != nil {
			return nil, fmt.Errorf("sign iris request: %w", err)
		}
	}

	return req, nil
}

func (c *H2CClient) newSignedStreamRequest(ctx context.Context, method, path string, body io.ReadCloser, bodySHA256 string, role SecretRole) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret := c.secretFor(role); secret != "" {
		if err := setIrisHMACHeaders(req, secret, method, path, bodySHA256); err != nil {
			return nil, fmt.Errorf("sign iris request: %w", err)
		}
	}

	return req, nil
}

func (c *H2CClient) newRequest(ctx context.Context, method, path string, body io.Reader, role SecretRole) (*http.Request, error) {
	requestBody := body
	var bodyBytes []byte

	secret := c.secretFor(role)
	if secret != "" && body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("read iris request body: %w", err)
		}
		requestBody = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, requestBody)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret != "" {
		if err := setIrisHMACHeaders(req, secret, method, path, sha256HexBytes(bodyBytes)); err != nil {
			return nil, fmt.Errorf("sign iris request: %w", err)
		}
	}

	return req, nil
}

func (c *H2CClient) secretFor(role SecretRole) string {
	switch role {
	case SecretRoleInbound:
		if s := strings.TrimSpace(c.auth.inboundSecret); s != "" {
			return s
		}
	case SecretRoleBotControl:
		if s := strings.TrimSpace(c.auth.botControlToken); s != "" {
			return s
		}
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
	default:
		return "application/octet-stream"
	}
}

func buildImageManifest(images [][]byte) []imagePartSpec {
	specs := make([]imagePartSpec, len(images))
	for i, img := range images {
		hash := sha256.Sum256(img)
		specs[i] = imagePartSpec{
			Index:       i,
			SHA256Hex:   hex.EncodeToString(hash[:]),
			ByteLength:  int64(len(img)),
			ContentType: detectImageContentType(img),
		}
	}
	return specs
}
