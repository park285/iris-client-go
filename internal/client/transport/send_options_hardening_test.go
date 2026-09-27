package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestApplySendOptionsIgnoresNilOption(t *testing.T) {
	t.Parallel()

	got := applySendOptions([]SendOption{nil, WithThreadID("123")})
	if got.ThreadID == nil || *got.ThreadID != "123" {
		t.Fatalf("ThreadID = %v, want 123", got.ThreadID)
	}
}

func TestValidateSendOptionsNormalizesThreadAndClientRequestID(t *testing.T) {
	t.Parallel()

	threadID := " 12345 "
	clientRequestID := " chatbotgo:log-42:reply-v1 "
	threadScope := 2

	if err := validateSendOptions(sendOptions{ThreadID: &threadID, ThreadScope: &threadScope, ClientRequestID: &clientRequestID}); err != nil {
		t.Fatalf("validateSendOptions() error = %v, want nil", err)
	}
}

func TestValidateSendOptionsRejectsBlankThreadIDForScopedReply(t *testing.T) {
	t.Parallel()

	threadID := ""
	threadScope := 2
	err := validateSendOptions(sendOptions{ThreadID: &threadID, ThreadScope: &threadScope})

	if err == nil || err.Error() != "iris: threadId must not be blank" {
		t.Fatalf("validateSendOptions() error = %v, want blank threadId error", err)
	}
}

// Iris는 threadId 없이 온 threadScope를 JSON·multipart admission 모두에서 값과 관계없이 400으로
// 거절한다. SDK가 scope 1을 통과시키면 서버에서만 실패하므로 모든 전송 경로가 요청 전에 거절해야 한다.
func TestSendPathsRejectThreadScopeWithoutThreadIDBeforeRequest(t *testing.T) {
	t.Parallel()

	sends := map[string]func(context.Context, *APIClient, SendOption) error{
		"SendMessageAccepted": func(ctx context.Context, c *APIClient, opt SendOption) error {
			_, err := c.SendMessageAccepted(ctx, testRoom, "msg", opt)

			return err
		},
		"SendMarkdown": func(ctx context.Context, c *APIClient, opt SendOption) error {
			_, err := c.SendMarkdown(ctx, testRoom, "**msg**", opt)

			return err
		},
		"SendImage": func(ctx context.Context, c *APIClient, opt SendOption) error {
			_, err := c.SendImage(ctx, testRoom, []byte("image"), opt)

			return err
		},
		"SendMultipleImages": func(ctx context.Context, c *APIClient, opt SendOption) error {
			_, err := c.SendMultipleImages(ctx, testRoom, [][]byte{[]byte("image")}, opt)

			return err
		},
		"SendFile": func(ctx context.Context, c *APIClient, opt SendOption) error {
			_, err := c.SendFile(ctx, testRoom, NewReplyFileBytes("a.txt", "text/plain", []byte("a")), opt)

			return err
		},
	}

	for name, send := range sends {
		for _, scope := range []int{1, 2} {
			t.Run(name+"/threadScope="+strconv.Itoa(scope), func(t *testing.T) {
				t.Parallel()

				var requests atomic.Int32

				rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
					requests.Add(1)

					return nil, errors.New("request must not be sent")
				})
				client := NewAPIClient("http://localhost", "", WithRoundTripper(rt))

				err := send(t.Context(), client, WithThreadScope(scope))
				if err == nil || !strings.Contains(err.Error(), "iris: threadScope requires threadId") {
					t.Fatalf("%s(threadScope=%d) error = %v, want threadScope requires threadId", name, scope, err)
				}

				if requests.Load() != 0 {
					t.Fatalf("%s(threadScope=%d) sent %d requests, want 0", name, scope, requests.Load())
				}
			})
		}
	}
}

func TestValidateSendOptionsRejectsNonASCIIThreadID(t *testing.T) {
	t.Parallel()

	threadID := "１２３"
	err := validateSendOptions(sendOptions{ThreadID: &threadID})

	if err == nil || !strings.Contains(err.Error(), "threadId must be numeric") {
		t.Fatalf("validateSendOptions() error = %v, want numeric threadId error", err)
	}
}

func TestValidateSendOptionsRejectsInvalidAttachmentJSON(t *testing.T) {
	t.Parallel()

	for _, raw := range [][]byte{
		[]byte(` `),
		[]byte(`{"broken"`),
		[]byte(`[1,2,3]`),
		[]byte(`null`),
	} {
		if err := validateAttachmentJSON(raw, false); err == nil {
			t.Fatalf("validateAttachmentJSON(%q) error = nil, want error", string(raw))
		}
	}
}

func TestWithAttachmentJSONClonesInput(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"a":1}`)
	opt := WithAttachmentJSON(raw)

	raw[2] = 'x'

	got := applySendOptions([]SendOption{opt})
	if string(got.AttachmentJSON) != `{"a":1}` {
		t.Fatalf("AttachmentJSON = %s, want cloned original", got.AttachmentJSON)
	}
}

func TestNonTextRepliesRejectAttachmentJSON(t *testing.T) {
	t.Parallel()

	client := NewAPIClient(testExampleBaseURL, "", WithTransport(transportHTTP1))
	if _, err := client.SendMarkdown(t.Context(), testRoom, "**hello**", WithAttachmentJSON([]byte(`{"a":1}`))); err == nil || !strings.Contains(err.Error(), "attachmentJson requires text reply type") {
		t.Fatalf("SendMarkdown() error = %v, want attachment/text validation error", err)
	}

	if _, err := client.SendImage(t.Context(), testRoom, []byte(msgTypeImage), WithAttachmentJSON([]byte(`{"a":1}`))); err == nil || !strings.Contains(err.Error(), "attachmentJson requires text reply type") {
		t.Fatalf("SendImage() error = %v, want attachment/text validation error", err)
	}
}
