package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	iris "park285/iris-client-go"
)

// H2CClient implements both Sender and AdminClient interfaces.
// Safe for concurrent use after creation.
type H2CClient struct {
	baseURL  string
	botToken string
	client   *http.Client
	logger   *slog.Logger
	opts     clientOptions
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
		client:   newHTTPClient(baseURL, o),
		logger:   logger,
		opts:     o,
	}
}

var (
	_ Sender      = (*H2CClient)(nil)
	_ AdminClient = (*H2CClient)(nil)
)

func (c *H2CClient) SendMessage(ctx context.Context, room, message string, opts ...iris.SendOption) error {
	o := iris.ApplySendOptions(opts)
	if err := iris.ValidateSendOptions(o); err != nil {
		return fmt.Errorf("validate send options: %w", err)
	}

	reqBody := iris.ReplyRequest{
		Type:        "text",
		Room:        room,
		Data:        message,
		ThreadID:    iris.NormalizeReplyThreadID(o.ThreadID),
		ThreadScope: iris.NormalizeReplyThreadScope(o.ThreadScope),
	}
	if err := c.postJSON(ctx, iris.PathReply, reqBody, nil); err != nil {
		return fmt.Errorf("send iris reply: %w", err)
	}

	return nil
}

func (c *H2CClient) SendImage(ctx context.Context, room, imageBase64 string) error {
	reqBody := iris.ReplyRequest{
		Type: "image",
		Room: room,
		Data: imageBase64,
	}
	if err := c.postJSON(ctx, iris.PathReply, reqBody, nil); err != nil {
		return fmt.Errorf("send iris image: %w", err)
	}

	return nil
}

func (c *H2CClient) GetConfig(ctx context.Context) (*iris.Config, error) {
	req, err := c.newRequest(ctx, http.MethodGet, iris.PathConfig, nil)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", iris.PathConfig, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", iris.PathConfig, err)
	}

	defer func() {
		//nolint:errcheck,gosec // Best-effort body close on deferred path.
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s: %w", iris.PathConfig, readErrorResponse(iris.PathConfig, resp))
	}

	var cfg iris.Config
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", iris.PathConfig, err)
	}

	return &cfg, nil
}

func (c *H2CClient) Decrypt(ctx context.Context, data string) (string, error) {
	reqBody := iris.DecryptRequest{
		B64Ciphertext: data,
		Enc:           0,
	}

	var respBody iris.DecryptResponse
	if err := c.postJSON(ctx, iris.PathDecrypt, reqBody, &respBody); err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}

	return respBody.PlainText, nil
}

func (c *H2CClient) postJSON(ctx context.Context, path string, body, out any) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return fmt.Errorf("encode request body: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, path, &buf)
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

	if resp.StatusCode >= 400 {
		return fmt.Errorf("post %s: %w", path, readErrorResponse(path, resp))
	}

	if out == nil {
		//nolint:errcheck,gosec // Best-effort drain.
		io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}

	return nil
}

func readErrorResponse(path string, resp *http.Response) error {
	//nolint:errcheck // Best-effort read.
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	return fmt.Errorf("iris %s returned %d: %s", path, resp.StatusCode, strings.TrimSpace(string(payload)))
}

func (c *H2CClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	if token := iris.ResolveToken(c.botToken); token != "" {
		req.Header.Set(iris.HeaderBotToken, token)
	}

	return req, nil
}
