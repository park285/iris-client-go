package iris_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	iris "github.com/park285/iris-client-go/iris"
	"github.com/park285/iris-client-go/webhook"
)

type stubHandler struct{}

func (stubHandler) HandleMessage(_ context.Context, _ *webhook.Message) {}

type stubAdmitter struct{}

func (stubAdmitter) AdmitMessage(_ context.Context, _ *webhook.Message) error { return nil }

func TestNewClient_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")
	t.Setenv("IRIS_TRANSPORT", "h2c")

	client, err := iris.NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	if err := os.Unsetenv("IRIS_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BASE_URL) error = %v", err)
	}
	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}

func TestNewClient_MissingBotToken(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing bot token")
	}
}

func TestNewClient_WhitespaceBotTokenOption(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	t.Setenv("IRIS_TRANSPORT", "h2c")
	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient(iris.WithBotToken("   "))
	if err == nil {
		t.Fatal("expected error for whitespace-only bot token")
	}
	if !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\" error, got %v", err)
	}
}

func TestNewClient_WhitespaceBotTokenEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	t.Setenv("IRIS_TRANSPORT", "h2c")
	t.Setenv("IRIS_BOT_TOKEN", "   ")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for whitespace-only bot token")
	}
	if !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\" error, got %v", err)
	}
}

func TestNewWebhookHandler_WhitespaceToken(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "   ")

	_, err := iris.NewWebhookHandler(stubHandler{})
	if err == nil {
		t.Fatal("expected error for whitespace-only webhook token")
	}
	if !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\" error, got %v", err)
	}
}

func TestNewClient_OptionOverridesEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")
	t.Setenv("IRIS_TRANSPORT", "h2c")

	client, err := iris.NewClient(iris.WithBaseURL("http://opt-host:4000"))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewWebhookHandler_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	handler, err := iris.NewWebhookHandler(stubHandler{})
	if err != nil {
		t.Fatalf("NewWebhookHandler() error = %v", err)
	}
	if handler == nil {
		t.Fatal("NewWebhookHandler() returned nil")
	}
	_ = handler.Close()
}

func TestNewWebhookHandler_MissingToken(t *testing.T) {
	if err := os.Unsetenv("IRIS_WEBHOOK_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_WEBHOOK_TOKEN) error = %v", err)
	}

	_, err := iris.NewWebhookHandler(stubHandler{})
	if err == nil {
		t.Fatal("expected error for missing webhook token")
	}
}

func TestNewWebhookHandler_SecretWithoutTokenSucceeds(t *testing.T) {
	if err := os.Unsetenv("IRIS_WEBHOOK_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_WEBHOOK_TOKEN) error = %v", err)
	}

	handler, err := iris.NewWebhookHandler(stubHandler{},
		webhook.WithWebhookSecret("signed-webhook-secret"),
	)
	if err != nil {
		t.Fatalf("NewWebhookHandler() error = %v", err)
	}
	if handler == nil {
		t.Fatal("NewWebhookHandler() returned nil")
	}
	_ = handler.Close()
}

func TestNewWebhookHandler_NilHandler(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	_, err := iris.NewWebhookHandler(nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestNewDurableWebhookHandler_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	handler, err := iris.NewDurableWebhookHandler(stubAdmitter{})
	if err != nil {
		t.Fatalf("NewDurableWebhookHandler() error = %v", err)
	}
	if handler == nil {
		t.Fatal("NewDurableWebhookHandler() returned nil")
	}
	_ = handler.Close()
}

func TestNewDurableWebhookHandler_NilAdmitter(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	_, err := iris.NewDurableWebhookHandler(nil)
	if !errors.Is(err, webhook.ErrMessageAdmitterRequired) {
		t.Fatalf("NewDurableWebhookHandler(nil) error = %v, want %v", err, webhook.ErrMessageAdmitterRequired)
	}
}

func TestNewDurableWebhookHandler_NilAdmitterWinsOverMissingToken(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "")
	if err := os.Unsetenv("IRIS_WEBHOOK_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_WEBHOOK_TOKEN) error = %v", err)
	}

	_, err := iris.NewDurableWebhookHandler(nil)
	if !errors.Is(err, webhook.ErrMessageAdmitterRequired) {
		t.Fatalf("NewDurableWebhookHandler(nil) error = %v, want %v", err, webhook.ErrMessageAdmitterRequired)
	}
}

func TestNewDurableWebhookHandler_MissingToken(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "")
	if err := os.Unsetenv("IRIS_WEBHOOK_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_WEBHOOK_TOKEN) error = %v", err)
	}

	_, err := iris.NewDurableWebhookHandler(stubAdmitter{})
	if err == nil {
		t.Fatal("expected error for missing webhook token")
	}
}

func TestFacadeReexportsClientSDKHelpers(t *testing.T) {
	t.Parallel()

	var (
		_ iris.ClientSDKConfig
		_ = iris.WithMention(iris.ReplyMention{UserID: 1, Nickname: "tester"})
		_ = iris.WithMention(iris.ReplyMention{UserID: "talk-text-id", Nickname: "tester"})
		_ = iris.WithMentions(iris.ReplyMention{UserID: 2, At: []int{1}, Len: 6})
	)
}

func TestFacadeConfiguresDedicatedCertReloadToken(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get(iris.HeaderIrisSignature) == "" {
			t.Error("cert reload request has no signature")
		}
		if err := json.NewEncoder(w).Encode(iris.CertReloadResponse{Status: "reloaded"}); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := iris.NewH2CClient(
		server.URL,
		"unused-bot-token",
		iris.WithHTTPClient(server.Client()),
		iris.WithCertReloadToken("cert-reload-secret"),
	)
	result, err := client.ReloadH3Certificate(t.Context())
	if err != nil {
		t.Fatalf("ReloadH3Certificate() error = %v", err)
	}
	if result.Status != "reloaded" {
		t.Fatalf("Status = %q, want reloaded", result.Status)
	}

	missingTokenClient := iris.NewH2CClient(server.URL, "unused-bot-token", iris.WithHTTPClient(server.Client()))
	_, err = missingTokenClient.ReloadH3Certificate(t.Context())
	if !errors.Is(err, iris.ErrCertReloadTokenRequired) {
		t.Fatalf("ReloadH3Certificate() missing-token error = %v, want ErrCertReloadTokenRequired", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("server request count = %d, want 1", got)
	}
}

func TestFacadeExposesH3DialGuard(t *testing.T) {
	t.Parallel()

	var opt iris.ClientOption = iris.WithH3DialGuard(func(net.IP) error {
		return nil
	})
	if opt == nil {
		t.Fatal("WithH3DialGuard() returned nil")
	}
}

func TestFacadeClassifiesH3DialGuardDenial(t *testing.T) {
	t.Parallel()

	blocked := errors.New("blocked h3 egress")
	var attempts atomic.Int32
	client, err := iris.NewClient(
		iris.WithBaseURL("https://localhost:443"),
		iris.WithBotToken("token"),
		iris.WithTransport("h3"),
		iris.WithH3AllowSystemRoots(true),
		iris.WithReplyRetry(2),
		iris.WithH3DialGuard(func(net.IP) error {
			attempts.Add(1)

			return blocked
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	_, err = client.SendMessageAccepted(t.Context(), "room", "msg", iris.WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want H3 egress deny")
	}
	if !iris.IsH3EgressDenied(err) {
		t.Fatalf("SendMessageAccepted() error = %v, want H3 egress denied", err)
	}
	if !errors.Is(err, blocked) {
		t.Fatalf("SendMessageAccepted() error = %v, want %v", err, blocked)
	}
	if attempts.Load() != 1 {
		t.Fatalf("guard attempts = %d, want 1", attempts.Load())
	}
}
