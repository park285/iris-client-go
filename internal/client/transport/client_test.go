package transport

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v4/internal/testsupport"
)

func TestNewAPIClientDefaults(t *testing.T) {
	c := mustNewAPIClient(" http://127.0.0.1:9/ ", " token ", WithHTTPClient(&http.Client{}))
	if c.baseURL != testExampleBaseURL {
		t.Fatalf("baseURL = %q, want %s", c.baseURL, testExampleBaseURL)
	}

	if c.client == nil {
		t.Fatal("client = nil, want initialized http client")
	}
}

func TestClientCloseClosesHTTP3Transport(t *testing.T) {
	client := mustNewAPIClient("https://example.com", "token", WithTransport("h3"), withTestH3CA(t))

	if client.transportCloser == nil {
		t.Fatal("transportCloser = nil, want HTTP/3 transport closer")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if client.transportCloser != nil {
		t.Fatal("transportCloser was not cleared after Close")
	}
}

func TestAPIClientSendMessage(t *testing.T) {
	var (
		got          ReplyRequest
		gotSignature string
	)

	server := newReplyCaptureServer(t, &got, &gotSignature)
	defer server.Close()

	client := mustNewAPIClient(server.URL, " bot-token ", WithTransport(TransportHTTP1))
	if err := sendMessage(t.Context(), client, testRoomA, "hello", WithThreadID("12345")); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	assertSendMessageRequest(t, gotSignature, got)
}

func TestAPIClientSendMessageIncludesClientRequestID(t *testing.T) {
	t.Parallel()

	var got ReplyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertReplyPostRequest(t, r)

		if err := jsonv2.UnmarshalRead(r.Body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)

		testsupport.WriteResponse(t, w, "{}")
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))
	if err := sendMessage(t.Context(), client, testRoomA, "hello", WithClientRequestID("chatbotgo:log-42:reply-v1")); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if got.ClientRequestID == nil || *got.ClientRequestID != "chatbotgo:log-42:reply-v1" {
		t.Fatalf("ClientRequestID = %v, want chatbotgo:log-42:reply-v1", got.ClientRequestID)
	}
}

func TestAPIClientSendMessageAcceptedReturnsReplyAcceptedResponse(t *testing.T) {
	var got ReplyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertReplyPostRequest(t, r)

		if err := jsonv2.UnmarshalRead(r.Body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "reply-123",
			Room:      testRoomA,
			Type:      msgTypeText,
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, " bot-token ", WithTransport(TransportHTTP1))

	resp, err := client.SendMessageAccepted(t.Context(), testRoomA, "hello", WithThreadID("12345"))
	if err != nil {
		t.Fatalf("SendMessageAccepted() error = %v", err)
	}

	assertSendMessageRequest(t, "signed", got)

	if resp == nil {
		t.Fatal("SendMessageAccepted() response = nil")
	}

	if resp.RequestID != "reply-123" || resp.Delivery != testDeliveryQueued || resp.Type != msgTypeText {
		t.Fatalf("SendMessageAccepted() response = %+v, want queued text reply-123", resp)
	}
}

func TestAPIClientSendMessageAcceptedIncludesMentions(t *testing.T) {
	t.Parallel()

	var got ReplyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertReplyPostRequest(t, r)

		if err := jsonv2.UnmarshalRead(r.Body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "reply-mention",
			Room:      testRoomA,
			Type:      msgTypeText,
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "bot-token", WithTransport(TransportHTTP1))

	resp, err := client.SendMessageAccepted(t.Context(), testRoomA, "@홍길동 hello",
		WithMention(ReplyMention{UserID: 123456789, Nickname: testSenderName}))
	if err != nil {
		t.Fatalf("SendMessageAccepted() error = %v", err)
	}

	if resp == nil || resp.RequestID != "reply-mention" {
		t.Fatalf("SendMessageAccepted() response = %+v, want reply-mention", resp)
	}

	if len(got.Mentions) != 1 {
		t.Fatalf("mentions = %+v, want single mention", got.Mentions)
	}

	// SDK는 reply 요청을 encode만 하므로 test server는 기본 JSON decode로 숫자 userId를 float64로 읽는다.
	mention := got.Mentions[0]
	if mention.UserID != float64(123456789) || mention.Nickname != testSenderName || len(mention.At) != 0 || mention.Len != 0 {
		t.Fatalf("mention = %+v", mention)
	}
}

func TestAPIClientSendMessageValidationError(t *testing.T) {
	client := mustNewAPIClient(testExampleBaseURL, "", WithTransport(TransportHTTP1))

	err := sendMessage(t.Context(), client, testRoomA, "hello", WithThreadID("abc"))
	if err == nil {
		t.Fatal("SendMessage() error = nil, want validation error")
	}

	if !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("SendMessage() error = %q, want thread validation error", err.Error())
	}
}

func TestAPIClientSendImage(t *testing.T) {
	var (
		gotPath      string
		gotMetadata  replyImageMetadata
		gotImageData []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		metadata, images := readMultipartReplyRequest(t, r)

		gotMetadata = metadata

		if len(images) != 1 {
			t.Fatalf("image parts = %d, want 1", len(images))
		}

		gotImageData = images[0]

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{Success: true, Delivery: testDeliveryAsync, RequestID: "req-img", Room: "room-b", Type: msgTypeImage}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	imgBytes := []byte{0x89, 0x50, 0x4E, 0x47}
	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	resp, err := client.SendImage(t.Context(), "room-b", imgBytes, WithClientRequestID("chatbotgo:log-42:image-v1"))
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotMetadata.Type != msgTypeImage || gotMetadata.Room != "room-b" {
		t.Fatalf("unexpected metadata: %+v", gotMetadata)
	}

	if gotMetadata.ClientRequestID == nil || *gotMetadata.ClientRequestID != "chatbotgo:log-42:image-v1" {
		t.Fatalf("ClientRequestID = %v, want chatbotgo:log-42:image-v1", gotMetadata.ClientRequestID)
	}

	if !bytes.Equal(gotImageData, imgBytes) {
		t.Fatalf("image data = %v, want %v", gotImageData, imgBytes)
	}

	if resp == nil || resp.RequestID != "req-img" || resp.Delivery != testDeliveryAsync {
		t.Fatalf("unexpected response: %+v", resp)
	}

	assertImageManifest(t, gotMetadata.Images, [][]byte{imgBytes}, mimeImagePNG)
}

func TestAPIClientSendImagePreservesExplicitVideoMP4ContentType(t *testing.T) {
	t.Parallel()

	var (
		gotMetadata         replyImageMetadata
		gotImagePartHeaders []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata, images, contentTypes := readMultipartReplyRequestWithPartContentTypes(t, r)

		gotMetadata = metadata
		gotImagePartHeaders = contentTypes

		if len(images) != 1 {
			t.Fatalf("image parts = %d, want 1", len(images))
		}

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{Success: true, Delivery: "sent", RequestID: "req-video", Room: "room-b", Type: msgTypeImage}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	mp4Bytes := []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2'}
	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	if _, err := client.SendImage(t.Context(), "room-b", mp4Bytes, WithImageContentType("video/mp4")); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if len(gotMetadata.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(gotMetadata.Images))
	}

	if gotMetadata.Images[0].ContentType != "video/mp4" {
		t.Fatalf("metadata contentType = %q, want video/mp4", gotMetadata.Images[0].ContentType)
	}

	if !reflect.DeepEqual(gotImagePartHeaders, []string{"video/mp4"}) {
		t.Fatalf("multipart image content types = %#v, want video/mp4", gotImagePartHeaders)
	}
}

func TestAPIClientSendImageRejectsMentions(t *testing.T) {
	t.Parallel()

	client := mustNewAPIClient(testExampleBaseURL, "", WithTransport(TransportHTTP1))

	_, err := client.SendImage(t.Context(), testRoom, []byte(msgTypeImage),
		WithMention(ReplyMention{UserID: 123456789, Nickname: testSenderName}))
	if err == nil {
		t.Fatal("SendImage() error = nil, want mention validation error")
	}

	if !strings.Contains(err.Error(), "mentions are supported only for text and markdown replies") {
		t.Fatalf("SendImage() error = %q, want mention validation error", err.Error())
	}
}

func TestAPIClientSendMultipleImages(t *testing.T) {
	var (
		gotPath     string
		gotMetadata replyImageMetadata
		gotImages   [][]byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		metadata, images := readMultipartReplyRequest(t, r)

		gotMetadata = metadata
		gotImages = images

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{Success: true, Delivery: testDeliveryQueued, RequestID: "req-multi", Room: "room-c", Type: "image_multiple"}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	images := [][]byte{[]byte("\x89PNG\r\n\x1a\nimg1"), []byte("\x89PNG\r\n\x1a\nimg2")}
	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	resp, err := client.SendMultipleImages(t.Context(), "room-c", images,
		WithThreadID("999"))
	if err != nil {
		t.Fatalf("SendMultipleImages() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotMetadata.Type != "image_multiple" || gotMetadata.Room != "room-c" {
		t.Fatalf("unexpected metadata: %+v", gotMetadata)
	}

	if gotMetadata.ThreadID == nil || *gotMetadata.ThreadID != "999" {
		t.Fatalf("ThreadID = %v, want 999", gotMetadata.ThreadID)
	}

	if len(gotImages) != 2 || !bytes.Equal(gotImages[0], images[0]) || !bytes.Equal(gotImages[1], images[1]) {
		t.Fatalf("unexpected images: %q", gotImages)
	}

	if resp == nil || resp.RequestID != "req-multi" || resp.Delivery != testDeliveryQueued {
		t.Fatalf("unexpected response: %+v", resp)
	}

	assertImageManifest(t, gotMetadata.Images, images, mimeImagePNG)
}

func TestSendImageLargePayload(t *testing.T) {
	t.Parallel()

	var (
		receivedMetadata replyImageMetadata
		receivedImage    []byte
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		metadata, images := readMultipartReplyRequest(t, r)

		receivedMetadata = metadata

		if len(images) != 1 {
			t.Fatalf("image parts = %d, want 1", len(images))
		}

		receivedImage = images[0]

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{Success: true, Delivery: testDeliveryAsync, RequestID: "req-large", Room: testRoom, Type: msgTypeImage}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))

	defer server.Close()

	largePayload := append([]byte(testPNGMagic), bytes.Repeat([]byte("A"), 1<<20)...)
	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	if _, err := client.SendImage(t.Context(), testRoom, largePayload); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if receivedMetadata.Type != msgTypeImage || receivedMetadata.Room != testRoom {
		t.Fatalf("unexpected metadata: %+v", receivedMetadata)
	}

	if len(receivedImage) != len(largePayload) {
		t.Fatalf("received size = %d, want %d", len(receivedImage), len(largePayload))
	}

	// 대용량 이미지 매니페스트 검증
	if len(receivedMetadata.Images) != 1 {
		t.Fatalf("Images length = %d, want 1", len(receivedMetadata.Images))
	}

	spec := receivedMetadata.Images[0]
	if spec.ByteLength != int64(len(largePayload)) {
		t.Fatalf("Images[0].ByteLength = %d, want %d", spec.ByteLength, len(largePayload))
	}

	if spec.ContentType != mimeImagePNG {
		t.Fatalf("Images[0].ContentType = %q, want %s", spec.ContentType, mimeImagePNG)
	}
}

// Iris는 magic byte로 판별되지 않거나 선언 type과 다른 이미지를 ImageFormatMismatch로 반드시 거절한다.
// SDK는 서명·업로드 전에 같은 판정으로 실패하고 네트워크 요청을 보내지 않는다.
func TestSendImagesRejectUnrecognizedOrMismatchedFormatWithoutRequest(t *testing.T) {
	t.Parallel()

	png := []byte(testPNGMagic + "img")

	tests := []struct {
		name string
		send func(context.Context, *APIClient) error
		want string
	}{
		{name: "single unrecognized", send: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendImage(ctx, testRoom, []byte("not an image"))

			return err
		}, want: "iris: unrecognized image format"},
		{name: "multiple with one unrecognized", send: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendMultipleImages(ctx, testRoom, [][]byte{png, []byte("not an image")})

			return err
		}, want: "image 1: iris: unrecognized image format"},
		{name: "explicit type mismatch", send: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendImage(ctx, testRoom, png, WithImageContentType("image/jpeg"))

			return err
		}, want: `does not match detected image format "image/png"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32

			client := mustNewAPIClient("http://localhost", "token", withRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)

				return nil, errors.New("request must not be sent")
			})))

			err := tt.send(t.Context(), client)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("send error = %v, want %q", err, tt.want)
			}

			if got := requests.Load(); got != 0 {
				t.Fatalf("requests = %d, want 0", got)
			}
		})
	}
}

func TestPostJSON429Retry(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) <= 2 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}

		w.WriteHeader(http.StatusOK)

		testsupport.WriteResponse(t, w, "{}")
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1), WithReplyRetry(3))
	if err := sendMessage(t.Context(), client, testRoom, "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want 3", attempts.Load())
	}
}

func TestSendMessageRejectsNon2xxStatus(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusMultipleChoices, http.StatusNotModified} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			client := mustNewAPIClient(server.URL, "token", WithTransport(TransportHTTP1))
			if err := sendMessage(t.Context(), client, testRoom, "message"); err == nil {
				t.Fatalf("SendMessage() status %d error = nil, want non-2xx failure", status)
			}
		})
	}
}

func TestPostJSON429RetryExhausted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1), WithReplyRetry(2))

	err := sendMessage(t.Context(), client, testRoom, "msg")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
}

func TestSend_429RetriesAndExposesErrRateLimited(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))

	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1), WithReplyRetry(3))

	err := sendMessage(t.Context(), client, testRoom, "msg")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}

	// 재시도 루프 종료는 마지막 attempt 판정 하나가 소유하므로 maxAttempts를 넘지 않아야 한다.
	if attempts.Load() != 3 {
		t.Fatalf("attempts = %d, want exactly 3 (1 + 2 retries)", attempts.Load())
	}

	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited after retries exhausted, got %v", err)
	}

	if !errors.Is(err, ErrRetryable) {
		t.Fatal("expected ErrRateLimited to also be ErrRetryable")
	}

	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatalf("expected errors.AsType to extract *HTTPError, got %v", err)
	}

	if httpErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want %d", httpErr.StatusCode, http.StatusTooManyRequests)
	}
}

func TestSend_5xxReturnsErrRetryable(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))
	err := sendMessage(t.Context(), client, testRoom, "msg")

	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected ErrRetryable on 503, got %v", err)
	}
}

func TestPostJSON500NotRetried(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))

	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1), WithReplyRetry(3))

	if err := sendMessage(t.Context(), client, testRoom, "msg"); err == nil {
		t.Fatal("SendMessage() error = nil, want 5xx failure")
	}

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1 (5xx should not retry)", attempts.Load())
	}
}

func TestPostJSONTransportErrorRetriesOnlyWithClientRequestID(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if attempts.Add(1) == 1 {
			return nil, errors.New("temporary network failure")
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"delivery":"queued","requestId":"req-1","room":"room","type":"text"}`)),
			Header:     make(http.Header),
		}, nil
	})

	client := mustNewAPIClient("http://localhost", "", withRoundTripper(rt), WithReplyRetry(2))
	if _, err := client.SendMessageAccepted(t.Context(), testRoom, "msg", WithClientRequestID("chatbotgo:log-42:reply-v1")); err != nil {
		t.Fatalf("SendMessageAccepted() error = %v", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
}

func TestPostJSONH3EgressDeniedDoesNotRetryWithClientRequestID(t *testing.T) {
	blocked := errors.New("blocked h3 egress")

	var attempts atomic.Int32

	client := mustNewAPIClient("https://localhost:443", "", WithTransport("h3"), withTestH3CA(t), WithReplyRetry(2), WithH3DialGuard(func(context.Context, net.IP) error {
		attempts.Add(1)

		return blocked
	}))

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	_, err := client.SendMessageAccepted(t.Context(), testRoom, "msg", WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want H3 egress deny")
	}

	if !errors.Is(err, ErrH3EgressDenied) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrH3EgressDenied", err)
	}

	if !errors.Is(err, blocked) {
		t.Fatalf("SendMessageAccepted() error = %v, want %v", err, blocked)
	}

	if errors.Is(err, ErrRetryable) {
		t.Fatal("H3 egress deny must not be retryable")
	}

	if attempts.Load() != 1 {
		t.Fatalf("guard attempts = %d, want 1", attempts.Load())
	}
}

func TestPostJSONTransportErrorDoesNotRetryWithoutClientRequestID(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		attempts.Add(1)

		return nil, errors.New("temporary network failure")
	})

	client := mustNewAPIClient("http://localhost", "", withRoundTripper(rt), WithReplyRetry(2))
	if err := sendMessage(t.Context(), client, testRoom, "msg"); err == nil {
		t.Fatal("SendMessage() error = nil, want transport error")
	}

	if attempts.Load() != 1 {
		t.Fatalf("attempts = %d, want 1", attempts.Load())
	}
}

func TestDoPostJSONUsesReplayableFixedSizeRequestBody(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.ContentLength <= 0 {
			t.Fatalf("ContentLength = %d, want fixed-size request body", r.ContentLength)
		}

		if r.GetBody == nil {
			t.Fatal("GetBody = nil, want replayable request body")
		}

		original, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(request body) error = %v", err)
		}

		replay, err := r.GetBody()
		if err != nil {
			t.Fatalf("GetBody() error = %v", err)
		}

		defer func() {
			if closeErr := replay.Close(); closeErr != nil {
				t.Errorf("replay.Close() error = %v", closeErr)
			}
		}()

		replayed, err := io.ReadAll(replay)
		if err != nil {
			t.Fatalf("ReadAll(replayed body) error = %v", err)
		}

		if int64(len(original)) != r.ContentLength {
			t.Fatalf("ContentLength = %d, want %d", r.ContentLength, len(original))
		}

		if !bytes.Equal(replayed, original) {
			t.Fatalf("replayed body = %q, want %q", replayed, original)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})

	client := mustNewAPIClient("http://localhost", "token", withRoundTripper(rt))
	if err := sendMessage(t.Context(), client, testRoom, "msg"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// withRoundTripper는 테스트 transport를 WithHTTPClient로 주입한다. Timeout은 기본 per-attempt
// timeout(10s)과 같게 둬 기존 테스트의 시간 의미를 유지한다.
func withRoundTripper(rt http.RoundTripper) ClientOption {
	return WithHTTPClient(&http.Client{Transport: rt, Timeout: 10 * time.Second})
}

type trackingReadCloser struct {
	payload        string
	readBytes      int
	closed         bool
	drainedOnClose bool
}

func (r *trackingReadCloser) Read(p []byte) (int, error) {
	if r.readBytes >= len(r.payload) {
		return 0, io.EOF
	}

	n := copy(p, r.payload[r.readBytes:])

	r.readBytes += n

	return n, nil
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	r.drainedOnClose = r.readBytes == len(r.payload)

	return nil
}

func TestAPIClientErrorResponses(t *testing.T) {
	tests := []apiErrorResponseTestCase{
		{
			name: "send message returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				return sendMessage(t.Context(), c, testRoom, "msg")
			},
			wantIn: "send iris reply: post /reply: iris /reply returned 500: boom",
		},
		{
			name: "send image returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				_, err := c.SendImage(t.Context(), testRoom, []byte(testPNGMagic+"img"))

				return err
			},
			wantIn: "send iris image: post /reply: iris /reply returned 500: boom",
		},
		{
			name: "send multiple images returns wrapped error",
			path: PathReply,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				_, err := c.SendMultipleImages(t.Context(), testRoom, [][]byte{[]byte(testPNGMagic + "img")})

				return err
			},
			wantIn: "send iris multiple images: post /reply: iris /reply returned 500: boom",
		},
		{
			name:   "get rooms returns http error",
			path:   PathRooms,
			call:   getRoomsError,
			wantIn: "iris /rooms returned 500: boom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runAPIErrorResponseTest(t, tt)
		})
	}
}

func TestClientFailureResponsesAreFullyDrainedBeforeClose(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status int
		call   func(*testing.T, *APIClient) error
	}{
		{
			name:   "reply 429 body is drained",
			status: http.StatusTooManyRequests,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				return sendMessage(t.Context(), c, testRoom, "msg")
			},
		},
		{
			name:   "reply 500 body is drained",
			status: http.StatusInternalServerError,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				return sendMessage(t.Context(), c, testRoom, "msg")
			},
		},
		{
			name:   "rooms 500 body is drained",
			status: http.StatusInternalServerError,
			call: func(t *testing.T, c *APIClient) error {
				t.Helper()

				_, err := c.GetRooms(t.Context())

				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := &trackingReadCloser{payload: strings.Repeat("x", 16<<10)}
			rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: tt.status,
					Body:       body,
					Header:     make(http.Header),
				}, nil
			})

			client := mustNewAPIClient("http://localhost", "", withRoundTripper(rt), WithReplyRetry(1))

			err := tt.call(t, client)
			if err == nil {
				t.Fatal("error = nil, want failure")
			}

			if !body.closed {
				t.Fatal("response body was not closed")
			}

			if !body.drainedOnClose {
				t.Fatalf("response body closed before drain: read=%d total=%d", body.readBytes, len(body.payload))
			}
		})
	}
}

type apiErrorResponseTestCase struct {
	name   string
	path   string
	call   func(*testing.T, *APIClient) error
	wantIn string
}

func newReplyCaptureServer(t *testing.T, got *ReplyRequest, gotSignature *string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertReplyPostRequest(t, r)

		*gotSignature = r.Header.Get(HeaderIrisSignature)

		if err := jsonv2.UnmarshalRead(r.Body, got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		w.WriteHeader(http.StatusOK)

		testsupport.WriteResponse(t, w, "{}")
	}))
}

func readMultipartReplyRequest(t *testing.T, r *http.Request) (replyImageMetadata, [][]byte) {
	t.Helper()

	metadata, images, _ := readMultipartReplyRequestWithPartContentTypes(t, r)

	return metadata, images
}

func readMultipartReplyRequestWithPartContentTypes(t *testing.T, r *http.Request) (replyImageMetadata, [][]byte, []string) {
	t.Helper()

	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}

	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}

	mr, err := r.MultipartReader()
	if err != nil {
		t.Fatalf("MultipartReader() error = %v", err)
	}

	var (
		metadata       replyImageMetadata
		seenMeta       bool
		imageParts     [][]byte
		imagePartMIMEs []string
	)

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			t.Fatalf("NextPart() error = %v", err)
		}

		payload, err := io.ReadAll(part)
		if err != nil {
			testsupport.CloseNow(t, "part.Close", part.Close)

			t.Fatalf("ReadAll(part) error = %v", err)
		}

		if err := part.Close(); err != nil {
			t.Fatalf("part.Close() error = %v", err)
		}

		switch part.FormName() {
		case "metadata":
			if seenMeta {
				t.Fatal("metadata part duplicated")
			}

			if err := jsonv2.Unmarshal(payload, &metadata); err != nil {
				t.Fatalf("jsonv2.Unmarshal(metadata) error = %v", err)
			}

			seenMeta = true
		case msgTypeImage:
			imageParts = append(imageParts, payload)
			imagePartMIMEs = append(imagePartMIMEs, part.Header.Get("Content-Type"))
		default:
			t.Fatalf("unexpected form part %q", part.FormName())
		}
	}

	if !seenMeta {
		t.Fatal("metadata part missing")
	}

	return metadata, imageParts, imagePartMIMEs
}

func assertImageManifest(t *testing.T, specs []imagePartSpec, images [][]byte, wantContentType string) {
	t.Helper()

	if len(specs) != len(images) {
		t.Fatalf("Images length = %d, want %d", len(specs), len(images))
	}

	for i, img := range images {
		spec := specs[i]
		if spec.Index != i {
			t.Fatalf("Images[%d].Index = %d, want %d", i, spec.Index, i)
		}

		if spec.ByteLength != int64(len(img)) {
			t.Fatalf("Images[%d].ByteLength = %d, want %d", i, spec.ByteLength, len(img))
		}

		if spec.ContentType != wantContentType {
			t.Fatalf("Images[%d].ContentType = %q, want %q", i, spec.ContentType, wantContentType)
		}

		if spec.SHA256Hex == "" {
			t.Fatalf("Images[%d].SHA256Hex is empty", i)
		}
	}
}

func assertReplyPostRequest(t *testing.T, r *http.Request) {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("method = %s, want %s", r.Method, http.MethodPost)
	}

	if r.URL.Path != PathReply {
		t.Fatalf("path = %s, want %s", r.URL.Path, PathReply)
	}
}

func assertSendMessageRequest(t *testing.T, gotSignature string, got ReplyRequest) {
	t.Helper()

	if gotSignature == "" {
		t.Fatal("signature header missing")
	}

	if got.Type != msgTypeText || got.Room != testRoomA || got.Data != "hello" {
		t.Fatalf("unexpected request body: %+v", got)
	}

	if got.ThreadID == nil || *got.ThreadID != "12345" {
		t.Fatalf("ThreadID = %v, want 12345", got.ThreadID)
	}
}

func runAPIErrorResponseTest(t *testing.T, tt apiErrorResponseTestCase) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tt.path {
			t.Fatalf("path = %s, want %s", r.URL.Path, tt.path)
		}

		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	err := tt.call(t, client)
	if err == nil {
		t.Fatal("error = nil, want failure")
	}

	if !strings.Contains(err.Error(), tt.wantIn) {
		t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantIn)
	}
}

func getRoomsError(t *testing.T, c *APIClient) error {
	t.Helper()

	_, err := c.GetRooms(t.Context())
	if err != nil {
		return fmt.Errorf("get rooms: %w", err)
	}

	return nil
}

func TestAPIClientSendMarkdown(t *testing.T) {
	t.Parallel()

	var (
		gotPath string
		gotBody ReplyRequest
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := jsonv2.UnmarshalRead(r.Body, &gotBody); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		resp := ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryAsync,
			RequestID: "req-123",
			Room:      testRoomA,
			Type:      msgTypeText,
		}
		if err := jsonv2.MarshalWrite(w, resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	result, err := client.SendMarkdown(t.Context(), testRoomA, "# Hello",
		WithThreadID("12345"),
		WithClientRequestID("chatbotgo:log-42:markdown-v1"),
	)
	if err != nil {
		t.Fatalf("SendMarkdown() error = %v", err)
	}

	if gotPath != PathReply {
		t.Fatalf("path = %q, want %q", gotPath, PathReply)
	}

	if gotBody.Type != "markdown" || gotBody.Room != testRoomA || gotBody.Data != "# Hello" {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}

	if gotBody.ClientRequestID == nil || *gotBody.ClientRequestID != "chatbotgo:log-42:markdown-v1" {
		t.Fatalf("ClientRequestID = %v, want chatbotgo:log-42:markdown-v1", gotBody.ClientRequestID)
	}

	if gotBody.ThreadID == nil || *gotBody.ThreadID != "12345" {
		t.Fatalf("ThreadID = %v, want 12345", gotBody.ThreadID)
	}

	if !result.Success || result.RequestID != "req-123" || result.Delivery != testDeliveryAsync {
		t.Fatalf("unexpected response: %+v", result)
	}
}

func TestAPIClientSendMarkdownIncludesMentions(t *testing.T) {
	t.Parallel()

	var got ReplyRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertReplyPostRequest(t, r)

		if err := jsonv2.UnmarshalRead(r.Body, &got); err != nil {
			t.Fatalf("decode body: %v", err)
		}

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "reply-markdown-mention",
			Room:      testRoomA,
			Type:      "markdown",
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "bot-token", WithTransport(TransportHTTP1))

	result, err := client.SendMarkdown(t.Context(), testRoomA, "@홍길동 **hello**",
		WithMention(ReplyMention{UserID: 123456789, Nickname: testSenderName}))
	if err != nil {
		t.Fatalf("SendMarkdown() error = %v", err)
	}

	if result == nil || result.RequestID != "reply-markdown-mention" {
		t.Fatalf("SendMarkdown() response = %+v, want reply-markdown-mention", result)
	}

	if got.Type != "markdown" || len(got.Mentions) != 1 {
		t.Fatalf("request = %+v, want markdown with single mention", got)
	}
}

func TestAPIClientSendMarkdownValidationError(t *testing.T) {
	t.Parallel()

	client := mustNewAPIClient(testExampleBaseURL, "", WithTransport(TransportHTTP1))

	_, err := client.SendMarkdown(t.Context(), testRoom, "md", WithThreadID("abc"))
	if err == nil {
		t.Fatal("SendMarkdown() error = nil, want validation error")
	}

	if !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("error = %q, want thread validation error", err.Error())
	}
}

func TestAPIClientGetReplyStatus(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		detail := "delivered"
		resp := ReplyStatusSnapshot{
			RequestID:        "req-abc",
			State:            "completed",
			UpdatedAtEpochMs: 1711600000000,
			Detail:           &detail,
		}

		if err := jsonv2.MarshalWrite(w, resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	snap, err := client.GetReplyStatus(t.Context(), "req-abc")
	if err != nil {
		t.Fatalf("GetReplyStatus() error = %v", err)
	}

	if gotPath != "/reply-status/req-abc" {
		t.Fatalf("path = %q, want /reply-status/req-abc", gotPath)
	}

	if snap.RequestID != "req-abc" || snap.State != "completed" || snap.UpdatedAtEpochMs != 1711600000000 {
		t.Fatalf("unexpected response: %+v", snap)
	}

	if snap.Detail == nil || *snap.Detail != "delivered" {
		t.Fatalf("Detail = %v, want delivered", snap.Detail)
	}
}

func TestAPIClientGetReplyStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

	_, err := client.GetReplyStatus(t.Context(), "missing")
	if err == nil {
		t.Fatal("expected error for 404")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %q, want 404 mention", err.Error())
	}
}

func TestDoPostJSONPipeCleanupOnTransportError(t *testing.T) {
	transportErr := errors.New("connection refused")
	rt := roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, transportErr
	})

	client := mustNewAPIClient("http://localhost", "token", withRoundTripper(rt))

	before := runtime.NumGoroutine()

	for range 10 {
		if err := sendMessage(t.Context(), client, testRoom, "msg"); err == nil {
			t.Fatal("SendMessage() error = nil, want transport failure")
		}
	}

	// encoder goroutine이 해제될 시간
	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()

	// goroutine 누수가 없으면 차이가 작아야 함
	if after-before > 5 {
		t.Fatalf("goroutine leak: before=%d, after=%d (diff=%d)", before, after, after-before)
	}
}

func TestAPIClientBotTokenSignsReply(t *testing.T) {
	t.Parallel()

	botControlSecret := "bot-control-xyz"

	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()

		w.WriteHeader(http.StatusOK)

		testsupport.WriteResponse(t, w, "{}")
	}))

	defer srv.Close()

	c := mustNewAPIClient(srv.URL, botControlSecret,
		WithHTTPClient(srv.Client()),
	)

	err := sendMessage(t.Context(), c, "room1", "hello")
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if capturedHeaders.Get("X-Iris-Signature") == "" {
		t.Fatal("expected signature header")
	}
}

func TestPostMultipartUsesReplayableFixedSizeRequestBody(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.ContentLength <= 0 {
			t.Fatalf("ContentLength = %d, want fixed-size multipart request body", r.ContentLength)
		}

		if r.GetBody == nil {
			t.Fatal("GetBody = nil, want replayable multipart request body")
		}

		original, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll(request body) error = %v", err)
		}

		replay, err := r.GetBody()
		if err != nil {
			t.Fatalf("GetBody() error = %v", err)
		}

		defer func() {
			if closeErr := replay.Close(); closeErr != nil {
				t.Errorf("replay.Close() error = %v", closeErr)
			}
		}()

		replayed, err := io.ReadAll(replay)
		if err != nil {
			t.Fatalf("ReadAll(replayed body) error = %v", err)
		}

		if int64(len(original)) != r.ContentLength {
			t.Fatalf("ContentLength = %d, want %d", r.ContentLength, len(original))
		}

		if !bytes.Equal(replayed, original) {
			t.Fatalf("replayed body = %q, want %q", replayed, original)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{contentTypeJSON}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true,"delivery":"queued","requestId":"r1","room":"room","type":"image"}`)),
		}, nil
	})

	c := mustNewAPIClient("http://localhost", "tok", withRoundTripper(rt))
	img := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}

	_, err := c.SendImage(t.Context(), testRoom, img)
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}
}

func TestPostMultipart429RetryRegeneratesBody(t *testing.T) {
	t.Parallel()

	// 429 후 재시도 시 multipart body가 정상적으로 재생성되는지 검증
	var (
		attempts    atomic.Int32
		lastBodyLen int
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll(body) error on attempt %d: %v", attempts.Load(), err)
		}

		if len(body) == 0 {
			t.Errorf("empty body on attempt %d", attempts.Load())
		}

		lastBodyLen = len(body)

		if attempts.Load() == 1 {
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}

		w.Header().Set("Content-Type", contentTypeJSON)

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "r1",
			Room:      testRoom,
			Type:      msgTypeImage,
		}); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))

	defer server.Close()

	c := mustNewAPIClient(server.URL, "tok", WithHTTPClient(server.Client()), WithReplyRetry(3))
	img := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}

	_, err := c.SendImage(t.Context(), testRoom, img)
	if err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}

	if lastBodyLen == 0 {
		t.Fatal("last attempt had empty body")
	}
}
