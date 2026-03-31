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
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

// H2CClient는 생성 후 동시 사용에 안전합니다.
type H2CClient struct {
	baseURL     string
	botToken    string
	hmacSecret  string
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

	hmacSecret := o.hmacSecret
	if hmacSecret == "" {
		hmacSecret = botToken
	}

	return &H2CClient{
		baseURL:    baseURL,
		botToken:   botToken,
		hmacSecret: hmacSecret,
		client:     resolveHTTPClient(baseURL, o),
		logger:     logger,
		opts:       o,
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
	if err := c.postJSON(ctx, PathReply, reqBody, nil); err != nil {
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

	resp, err := c.postMultipart(ctx, PathReply, metadata, images)
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

	resp, err := c.postMultipart(ctx, PathReply, metadata, images)
	if err != nil {
		return nil, fmt.Errorf("send iris multiple images: %w", err)
	}

	return resp, nil
}

func (c *H2CClient) GetConfig(ctx context.Context) (*ConfigResponse, error) {
	return doGet[ConfigResponse](c, ctx, PathConfig)
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
	if err := c.postJSON(ctx, PathReply, reqBody, &resp); err != nil {
		return nil, fmt.Errorf("send iris reply-markdown: %w", err)
	}

	return &resp, nil
}

func (c *H2CClient) GetReplyStatus(ctx context.Context, requestID string) (*ReplyStatusSnapshot, error) {
	return doGet[ReplyStatusSnapshot](c, ctx, PathReplyStatus+"/"+requestID)
}

func (c *H2CClient) UpdateConfig(ctx context.Context, name string, cfgReq ConfigUpdateRequest) (*ConfigUpdateResponse, error) {
	path := PathConfig + "/" + name

	var resp ConfigUpdateResponse
	if err := c.postJSON(ctx, path, cfgReq, &resp); err != nil {
		return nil, fmt.Errorf("update config %s: %w", name, err)
	}

	return &resp, nil
}

func (c *H2CClient) GetBridgeHealth(ctx context.Context) (*BridgeHealthResult, error) {
	return doGet[BridgeHealthResult](c, ctx, PathDiagnosticsBridge)
}

func (c *H2CClient) Query(ctx context.Context, queryReq QueryRequest) (*QueryResponse, error) {
	var resp QueryResponse
	if err := c.postJSON(ctx, PathQuery, queryReq, &resp); err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}

	return &resp, nil
}

func (c *H2CClient) Decrypt(ctx context.Context, data string) (string, error) {
	reqBody := DecryptRequest{
		B64Ciphertext: data,
		Enc:           0,
	}

	var respBody DecryptResponse
	if err := c.postJSON(ctx, PathDecrypt, reqBody, &respBody); err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return respBody.PlainText, nil
}

func (c *H2CClient) postJSON(ctx context.Context, path string, body, out any) error {
	payload, err := jsonx.Marshal(body)
	if err != nil {
		return fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	return c.postWithRetry(ctx, path, func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newSignedRequest(attemptCtx, http.MethodPost, path, payload)
		if err != nil {
			return nil, fmt.Errorf("post %s: %w", path, err)
		}

		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, out)
}

func (c *H2CClient) postMultipart(ctx context.Context, path string, metadata replyImageMetadata, images [][]byte) (*ReplyAcceptedResponse, error) {
	metadataBytes, err := jsonx.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode metadata: %w", path, err)
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writer.WriteField("metadata", string(metadataBytes)); err != nil {
		return nil, fmt.Errorf("post %s: write metadata field: %w", path, err)
	}

	for i, img := range images {
		ct := detectImageContentType(img)
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="image-%d"`, i))
		h.Set("Content-Type", ct)
		partWriter, err := writer.CreatePart(h)
		if err != nil {
			return nil, fmt.Errorf("post %s: create image part: %w", path, err)
		}
		if _, err := partWriter.Write(img); err != nil {
			return nil, fmt.Errorf("post %s: write image data: %w", path, err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("post %s: close multipart: %w", path, err)
	}

	payload := body.Bytes()
	contentType := writer.FormDataContentType()
	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, path, func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newMultipartSignedRequest(attemptCtx, http.MethodPost, path, metadataBytes, bytes.NewReader(payload), contentType)
		if err != nil {
			return nil, fmt.Errorf("post %s: %w", path, err)
		}
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
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

func (c *H2CClient) newSignedRequest(ctx context.Context, method, path string, bodyBytes []byte) (*http.Request, error) {
	var body io.Reader
	if bodyBytes != nil {
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret := c.signingSecret(); secret != "" {
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

func (c *H2CClient) newMultipartSignedRequest(ctx context.Context, method, path string, metadataBytes []byte, body io.Reader, contentType string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	if secret := c.signingSecret(); secret != "" {
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

func (c *H2CClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if secret := c.signingSecret(); secret != "" {
		timestamp := fmt.Sprintf("%d", time.Now().UnixMilli())
		nonce := generateNonce()
		sig := signIrisRequest(secret, method, path, timestamp, nonce, "")
		req.Header.Set(HeaderIrisTimestamp, timestamp)
		req.Header.Set(HeaderIrisNonce, nonce)
		req.Header.Set(HeaderIrisSignature, sig)
	}

	return req, nil
}

func (c *H2CClient) signingSecret() string {
	return strings.TrimSpace(c.hmacSecret)
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
