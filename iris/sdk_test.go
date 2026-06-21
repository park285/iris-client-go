package iris_test

import (
	"context"
	"os"
	"strings"
	"testing"

	iris "github.com/park285/iris-client-go/iris"
)

type stubHandler struct{}

func (stubHandler) HandleMessage(_ context.Context, _ *iris.Message) {}

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

func TestNewWebhookHandler_NilHandler(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	_, err := iris.NewWebhookHandler(nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}

func TestFacadeReexportsWebhookSDKHelpers(t *testing.T) {
	t.Parallel()

	var (
		_ iris.NoopMetrics
		_ iris.NoopDeduplicator
		_ iris.HandlerOptions
		_ iris.WebhookSDKConfig
		_ iris.ClientSDKConfig
		_ iris.TaskPool
		_ = iris.WithTaskPool
		_ = iris.WithMention(iris.ReplyMention{UserID: 1, Nickname: "tester"})
		_ = iris.WithMention(iris.ReplyMention{UserID: "talk-text-id", Nickname: "tester"})
		_ = iris.WithMentions(iris.ReplyMention{UserID: 2, At: []int{1}, Len: 6})
	)
}
