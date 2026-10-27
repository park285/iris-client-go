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
	"net/http"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/propagation"

	"github.com/park285/iris-client-go/v4/internal/baseendpoint"
	clientmultipart "github.com/park285/iris-client-go/v4/internal/client/multipart"
	"github.com/park285/iris-client-go/v4/internal/client/randomhex"
	"github.com/park285/iris-client-go/v4/internal/client/signing"
)

type APIClient struct {
	baseURL string
	signer  *signing.HMACSigner
	client  *http.Client
	opts    clientOptions
	// http1ProbeOnly는 SDK가 만든 http1 transport라 loopback probe route만 보낼 수 있음을 뜻한다.
	http1ProbeOnly  bool
	closeMu         sync.Mutex
	transportCloser io.Closer
}

// newAPIClient는 이미 적용·정규화된 옵션으로 client를 만든다. 옵션 클로저를 다시 실행하지 않는다.
// Base URL·transport·CA 오류는 호출마다 늦게 드러내지 않고 여기서 반환한다.
func newAPIClient(baseURL, botToken string, o clientOptions) (*APIClient, error) {
	endpoint, err := baseendpoint.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("iris: invalid base endpoint: %w", err)
	}

	transport, err := resolveTransport(o.Transport)
	if err != nil {
		return nil, err
	}

	c := &APIClient{
		baseURL: endpoint.String(),
		signer:  newBotControlSigner(botToken),
		opts:    o,
	}

	if o.HTTPClient != nil {
		// 호출자가 소유한 client는 닫을 closer가 없고 route 제한도 호출자가 소유한다.
		c.client = cloneHTTPClientWithRedirectPolicy(o.HTTPClient)

		return c, nil
	}

	httpClient, closer, err := newHTTPClientWithCloser(endpoint, transport, o)
	if err != nil {
		return nil, err
	}

	c.client = httpClient
	c.transportCloser = closer
	c.http1ProbeOnly = transport == TransportHTTP1

	return c, nil
}

// newBotControlSigner는 bot-control 자격의 서명기를 한 번만 만든다. 자격이 비면 요청을 서명하지 않는다.
func newBotControlSigner(botToken string) *signing.HMACSigner {
	secret := strings.TrimSpace(botToken)
	if secret == "" {
		return nil
	}

	return signing.NewHMACSigner(secret)
}

var _ Sender = (*APIClient)(nil)

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

func (c *APIClient) SendMessageAccepted(ctx context.Context, room, message string, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	reqBody, err := newTextReplyRequest(room, message, opts)
	if err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody, reqBody.ClientRequestID != nil)
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
		Mentions:        cloneReplyMentions(o.Mentions),
	}

	resp, err := c.postJSON[ReplyAcceptedResponse](ctx, PathReply, reqBody, reqBody.ClientRequestID != nil)
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
	resp, err := c.postJSON[StatsResponse](ctx, PathQueryMemberStats, req, true)
	if err != nil {
		return nil, fmt.Errorf("query member stats: %w", err)
	}

	return resp, nil
}

func (c *APIClient) QueryRecentMessages(ctx context.Context, req QueryRecentMessagesRequest) (*RecentMessagesResponse, error) {
	resp, err := c.postJSON[RecentMessagesResponse](ctx, PathQueryRecentMessages, req, true)
	if err != nil {
		return nil, fmt.Errorf("query recent messages: %w", err)
	}

	return resp, nil
}

// idempotent는 같은 요청을 다시 보내도 부수효과가 중복되지 않는지다. 조회 POST는 참이고, reply는
// clientRequestId가 있을 때만 참이다. 이 값이 TransportError.Idempotent와 SDK 재시도 판정을 정한다.
func (c *APIClient) postJSON[T any](ctx context.Context, path string, body any, idempotent bool) (*T, error) {
	buildRequest, err := c.newSignedJSONRequest(path, body)
	if err != nil {
		return nil, err
	}

	return c.retryPostJSON[T](ctx, path, idempotent, buildRequest)
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
	req, err := c.newRequest(ctx, method, path, body)
	if err != nil {
		return nil, err
	}

	if c.signer != nil {
		if err := signing.SetIrisHMACHeaders(req, c.signer, method, path, bodySHA256); err != nil {
			return nil, fmt.Errorf("sign iris request: %w", err)
		}
	}

	return req, nil
}

// newRequest는 서명 없는 요청을 만든다. SDK 소유 http1 transport에서는 loopback probe route만 허용하고,
// 그 밖의 요청은 보내기 전에 ErrNotSent로 거절한다.
func (c *APIClient) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if c.http1ProbeOnly && !isHTTP1ProbeRoute(method, path) {
		return nil, fmt.Errorf("%w: %s %s: %w", ErrNotSent, method, path, errHTTP1ProbeOnly)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("build iris request: %w", err)
	}

	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(req.Header))

	return req, nil
}

// errUnrecognizedImageFormat은 magic byte로 형식을 판별하지 못한 이미지다. Iris는 판별 불가 바이트를
// ImageFormatMismatch로 반드시 거절하므로, 업로드 전에 요청 없이 실패시킨다.
var errUnrecognizedImageFormat = errors.New("iris: unrecognized image format")

func detectImageContentType(data []byte) (string, error) {
	switch {
	case len(data) >= 4 && data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G':
		return mimeImagePNG, nil
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg", nil
	case len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		return "image/webp", nil
	case len(data) >= 4 && string(data[0:4]) == "GIF8":
		return "image/gif", nil
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		return "video/mp4", nil
	default:
		return "", errUnrecognizedImageFormat
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

// imageContentTypesForSend는 이미지마다 magic byte로 content type을 정한다. 명시한 content type은
// 판별 결과와 같을 때만 쓴다. 다르면 Iris가 형식 불일치로 거절하므로 요청 전에 실패한다.
func imageContentTypesForSend(images [][]byte, explicitContentType *string) ([]string, error) {
	contentTypes := make([]string, len(images))

	for i, image := range images {
		detected, err := detectImageContentType(image)
		if err != nil {
			return nil, fmt.Errorf("image %d: %w", i, err)
		}

		contentTypes[i] = detected
	}

	if explicitContentType == nil {
		return contentTypes, nil
	}

	contentType, err := normalizeReplyMediaContentType(*explicitContentType)
	if err != nil {
		return nil, err
	}

	if len(images) != 1 {
		return nil, errors.New("iris: imageContentType is supported only for SendImage")
	}

	if contentType != contentTypes[0] {
		return nil, fmt.Errorf("iris: imageContentType %q does not match detected image format %q", contentType, contentTypes[0])
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
