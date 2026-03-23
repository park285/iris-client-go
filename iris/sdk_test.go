package iris_test

import (
	"context"
	"os"
	"testing"

	iris "github.com/park285/iris-client-go/iris"
)

type stubHandler struct{}

func (stubHandler) HandleMessage(_ context.Context, _ *iris.Message) {}

func TestNewClient_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")

	client, err := iris.NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	os.Unsetenv("IRIS_BASE_URL")
	os.Unsetenv("IRIS_BOT_TOKEN")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}

func TestNewClient_MissingBotToken(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	os.Unsetenv("IRIS_BOT_TOKEN")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing bot token")
	}
}

func TestNewClient_OptionOverridesEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")

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
	os.Unsetenv("IRIS_WEBHOOK_TOKEN")

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
