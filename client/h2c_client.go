package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/park285/iris-client-go/internal/jsonx"
)

// H2CClient implements both Sender and AdminClient interfaces.
// Safe for concurrent use after creation.
type H2CClient struct {
	baseURL     string
	botToken    string
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

	return &H2CClient{
		baseURL:  baseURL,
		botToken: botToken,
		client:   resolveHTTPClient(baseURL, o),
		logger:   logger,
		opts:     o,
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

func (c *H2CClient) SendImage(ctx context.Context, room, imageBase64 string, opts ...SendOption) error {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return fmt.Errorf("validate send options: %w", err)
	}

	reqBody := ReplyRequest{
		Type:        "image",
		Room:        room,
		Data:        imageBase64,
		ThreadID:    normalizeReplyThreadID(o.ThreadID),
		ThreadScope: normalizeReplyThreadScope(o.ThreadScope),
	}
	if err := c.postJSON(ctx, PathReply, reqBody, nil); err != nil {
		return fmt.Errorf("send iris image: %w", err)
	}

	return nil
}

func (c *H2CClient) GetConfig(ctx context.Context) (*Config, error) {
	req, err := c.newRequest(ctx, http.MethodGet, PathConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", PathConfig, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", PathConfig, err)
	}

	defer func() {
		//nolint:errcheck,gosec // Best-effort body close on deferred path.
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s: %w", PathConfig, readErrorResponse(PathConfig, resp))
	}

	var cfg Config
	if err := jsonx.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", PathConfig, err)
	}

	return &cfg, nil
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
	if c.opts.ReplyRetryMax <= 0 || path != PathReply {
		return c.doPostJSON(ctx, path, body, out)
	}

	backoff := 50 * time.Millisecond
	for attempt := 1; attempt <= c.opts.ReplyRetryMax; attempt++ {
		err := c.doPostJSON(ctx, path, body, out)
		if err == nil {
			return nil
		}

		if !isRetryableError(err) || attempt == c.opts.ReplyRetryMax {
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

func (c *H2CClient) doPostJSON(ctx context.Context, path string, body, out any) error {
	payload, err := jsonx.Marshal(body)
	if err != nil {
		return fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}

	req.Header.Set("Content-Type", "application/json")

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

func (c *H2CClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if token := strings.TrimSpace(c.botToken); token != "" {
		req.Header.Set(HeaderBotToken, token)
	}

	return req, nil
}
