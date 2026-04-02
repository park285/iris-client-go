package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

// SecretRole은 요청 서명에 사용할 비밀키 역할을 나타냅니다.
type SecretRole int

const (
	// SecretRoleInbound는 /config 계열 라우트에 사용됩니다.
	SecretRoleInbound SecretRole = iota
	// SecretRoleBotControl은 /reply, /rooms 등 봇 제어 라우트에 사용됩니다.
	SecretRoleBotControl
)

// authSecrets는 라우트별 서명 비밀키를 보관합니다.
type authSecrets struct {
	inboundSecret   string
	botControlToken string
	sharedSecret    string
}

// H2CClient는 생성 후 동시 사용에 안전합니다.
type H2CClient struct {
	baseURL     string
	botToken    string
	auth        authSecrets
	client      *http.Client
	logger      *slog.Logger
	opts        clientOptions
	cachedProbe atomic.Value // *cachedPingProbe 저장
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

	return &H2CClient{
		baseURL:  baseURL,
		botToken: botToken,
		auth: authSecrets{
			inboundSecret:   o.inboundSecret,
			botControlToken: o.botControlToken,
			sharedSecret:    sharedSecret,
		},
		client: resolveHTTPClient(baseURL, o),
		logger: logger,
		opts:   o,
	}
}

func resolveHTTPClient(baseURL string, opts clientOptions) *http.Client {
	if opts.HTTPClient != nil {
		return opts.HTTPClient
	}

	if opts.RoundTripper != nil {
		return &http.Client{
			Timeout:   opts.Timeout,
			Transport: opts.RoundTripper,
		}
	}

	return newHTTPClient(baseURL, opts)
}

var (
	_ Sender      = (*H2CClient)(nil)
	_ AdminClient = (*H2CClient)(nil)
)

type retryableHTTPError struct {
	statusCode int
	body       string
}

func (e *retryableHTTPError) Error() string {
	return fmt.Sprintf("iris returned %d: %s", e.statusCode, e.body)
}

func isRetryableError(err error) bool {
	var httpErr *retryableHTTPError
	return errors.As(err, &httpErr) && httpErr.statusCode == http.StatusTooManyRequests
}

func (c *H2CClient) SendMessage(ctx context.Context, room, message string, opts ...SendOption) error {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return fmt.Errorf("validate send options: %w", err)
	}

	reqBody := ReplyRequest{
		Type:        "text",
		Room:        room,
		Data:        message,
		ThreadID:    normalizeReplyThreadID(o.ThreadID),
		ThreadScope: normalizeReplyThreadScope(o.ThreadScope),
	}
	if err := c.postJSON(ctx, PathReply, reqBody, nil, SecretRoleBotControl); err != nil {
		return fmt.Errorf("send iris reply: %w", err)
	}

	return nil
}

func (c *H2CClient) SendImage(ctx context.Context, room string, imageData []byte, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	images := [][]byte{imageData}
	metadata := replyImageMetadata{
		Type:        "image",
		Room:        room,
		ThreadID:    normalizeReplyThreadID(o.ThreadID),
		ThreadScope: normalizeReplyThreadScope(o.ThreadScope),
		Images:      buildImageManifest(images),
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

	metadata := replyImageMetadata{
		Type:        "image_multiple",
		Room:        room,
		ThreadID:    normalizeReplyThreadID(o.ThreadID),
		ThreadScope: normalizeReplyThreadScope(o.ThreadScope),
		Images:      buildImageManifest(images),
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

	reqBody := ReplyRequest{
		Type:        "markdown",
		Room:        room,
		Data:        markdown,
		ThreadID:    normalizeReplyThreadID(o.ThreadID),
		ThreadScope: normalizeReplyThreadScope(o.ThreadScope),
	}

	var resp ReplyAcceptedResponse
	if err := c.postJSON(ctx, PathReply, reqBody, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris reply-markdown: %w", err)
	}

	return &resp, nil
}

func (c *H2CClient) GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error) {
	return doGet[ReplyStatusSnapshot](c, ctx, PathReplyStatus+"/"+requestID, SecretRoleBotControl)
}

func (c *H2CClient) UpdateConfig(ctx context.Context, name string, cfgReq ConfigUpdateRequest) (*ConfigUpdateResponse, error) {
	path := PathConfig + "/" + name

	var resp ConfigUpdateResponse
	if err := c.postJSON(ctx, path, cfgReq, &resp, SecretRoleInbound); err != nil {
		return nil, fmt.Errorf("update config %s: %w", name, err)
	}

	return &resp, nil
}

func (c *H2CClient) GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error) {
	return doGet[BridgeHealthResult](c, ctx, PathDiagnosticsBridge, SecretRoleBotControl)
}

// QueryClient는 허용된 조회 연산을 제공하는 인터페이스입니다.
type QueryClient interface {
	QueryRoomSummary(ctx context.Context, chatID int64) (*RoomSummary, error)
	QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error)
	QueryRecentThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error)
	QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error)
}

var _ QueryClient = (*H2CClient)(nil)

// QueryRoomSummary는 지정한 채팅방의 요약 정보를 조회합니다.
func (c *H2CClient) QueryRoomSummary(ctx context.Context, chatID int64) (*RoomSummary, error) {
	var resp RoomSummary
	if err := c.postJSON(ctx, PathQueryRoomSummary, QueryRoomSummaryRequest{ChatID: chatID}, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query room summary: %w", err)
	}
	return &resp, nil
}

// QueryMemberStats는 지정한 채팅방의 멤버 통계를 조회합니다.
func (c *H2CClient) QueryMemberStats(ctx context.Context, req QueryMemberStatsRequest) (*StatsResponse, error) {
	var resp StatsResponse
	if err := c.postJSON(ctx, PathQueryMemberStats, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query member stats: %w", err)
	}
	return &resp, nil
}

// QueryRecentThreads는 지정한 채팅방의 최근 스레드를 조회합니다.
func (c *H2CClient) QueryRecentThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error) {
	var resp ThreadListResponse
	if err := c.postJSON(ctx, PathQueryRecentThreads, QueryRecentThreadsRequest{ChatID: chatID}, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("query recent threads: %w", err)
	}
	return &resp, nil
}

// QueryRecentMessages는 지정한 채팅방의 최근 메시지를 조회합니다.
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

	return c.postWithRetry(ctx, path, func(attemptCtx context.Context) (*http.Request, error) {
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

	// 결정적 boundary — Content-Type이 재시도 간 일정하게 유지됨
	boundary := generateMultipartBoundary()
	contentType := "multipart/form-data; boundary=" + boundary

	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, path, func(attemptCtx context.Context) (*http.Request, error) {
		pr, pw := io.Pipe()
		go func() {
			err := writeMultipartBody(pw, boundary, metadataBytes, images)
			pw.CloseWithError(err)
		}()

		req, err := c.newMultipartSignedRequest(attemptCtx, http.MethodPost, path, metadataBytes, pr, contentType, role)
		if err != nil {
			pr.Close()
			return nil, fmt.Errorf("post %s: %w", path, err)
		}
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// writeMultipartBody는 multipart body를 writer에 스트리밍합니다.
// 이미지 바이트가 중간 버퍼 없이 직접 전송됩니다.
func writeMultipartBody(w io.Writer, boundary string, metadataBytes []byte, images [][]byte) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("set boundary: %w", err)
	}

	if err := mw.WriteField("metadata", string(metadataBytes)); err != nil {
		return fmt.Errorf("write metadata field: %w", err)
	}

	for i, img := range images {
		ct := detectImageContentType(img)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="image-%d"`, i))
		h.Set("Content-Type", ct)
		partWriter, err := mw.CreatePart(h)
		if err != nil {
			return fmt.Errorf("create image part: %w", err)
		}
		if _, err := partWriter.Write(img); err != nil {
			return fmt.Errorf("write image data: %w", err)
		}
	}

	return mw.Close()
}

// generateMultipartBoundary는 multipart 요청용 랜덤 boundary를 생성합니다.
func generateMultipartBoundary() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (c *H2CClient) postWithRetry(ctx context.Context, path string, buildRequest func(context.Context) (*http.Request, error), out any) error {
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

		if !isRetryableError(err) || attempt == maxAttempts {
			return err
		}

		timer := time.NewTimer(backoff)
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

func (c *H2CClient) doRequest(req *http.Request, path string, out any) error {
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}

	defer func() {
		//nolint:errcheck,gosec // Best-effort body close on deferred path.
		resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusTooManyRequests {
		return &retryableHTTPError{
			statusCode: resp.StatusCode,
			body:       readErrorBody(resp.Body),
		}
	}

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

func readErrorResponse(path string, resp *http.Response) error {
	return fmt.Errorf("iris %s returned %d: %s", path, resp.StatusCode, readErrorBody(resp.Body))
}

func readErrorBody(body io.Reader) string {
	//nolint:errcheck // Best-effort capture for error text plus full drain for connection reuse.
	payload, _ := io.ReadAll(io.LimitReader(body, 8<<10))
	//nolint:errcheck // Best-effort drain of any remaining response bytes.
	io.Copy(io.Discard, body)
	return strings.TrimSpace(string(payload))
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
		timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
		nonce := generateNonce()
		bodyStr := ""
		bodyHash := sha256.Sum256(nil)
		if bodyBytes != nil {
			bodyStr = string(bodyBytes)
			bodyHash = sha256.Sum256(bodyBytes)
		}
		sig := signIrisRequest(secret, method, path, timestamp, nonce, bodyStr)
		req.Header.Set(HeaderIrisTimestamp, timestamp)
		req.Header.Set(HeaderIrisNonce, nonce)
		req.Header.Set(HeaderIrisSignature, sig)
		req.Header.Set(HeaderIrisBodySHA256, hex.EncodeToString(bodyHash[:]))
	}

	return req, nil
}

func (c *H2CClient) newMultipartSignedRequest(ctx context.Context, method, path string, metadataBytes []byte, body io.Reader, contentType string, role SecretRole) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if secret := c.secretFor(role); secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
		nonce := generateNonce()
		sig := signIrisRequest(secret, method, path, timestamp, nonce, string(metadataBytes))
		req.Header.Set(HeaderIrisTimestamp, timestamp)
		req.Header.Set(HeaderIrisNonce, nonce)
		req.Header.Set(HeaderIrisSignature, sig)
		metadataHash := sha256.Sum256(metadataBytes)
		req.Header.Set(HeaderIrisBodySHA256, hex.EncodeToString(metadataHash[:]))
	}

	return req, nil
}

func (c *H2CClient) newRequest(ctx context.Context, method, path string, body io.Reader, role SecretRole) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret := c.secretFor(role); secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
		nonce := generateNonce()
		sig := signIrisRequest(secret, method, path, timestamp, nonce, "")
		req.Header.Set(HeaderIrisTimestamp, timestamp)
		req.Header.Set(HeaderIrisNonce, nonce)
		req.Header.Set(HeaderIrisSignature, sig)
	}

	return req, nil
}

// secretFor는 역할에 따라 적절한 서명 비밀키를 반환합니다.
// 역할별 비밀키가 없으면 공유 비밀키로 폴백합니다.
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

// detectImageContentType는 매직 바이트로 이미지 MIME 타입을 판별합니다.
func detectImageContentType(data []byte) string {
	switch {
	case len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return "image/png"
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

// buildImageManifest는 이미지 목록에 대한 매니페스트를 생성합니다.
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
