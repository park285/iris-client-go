package webhook

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubMessageHandler struct{}

func (stubMessageHandler) HandleMessage(context.Context, *Message) {}

func TestNewHandlerCreatesClosableWrapper(t *testing.T) {
	t.Parallel()

	handler := NewHandler(context.Background(), "token", stubMessageHandler{}, slog.Default())
	if handler == nil {
		t.Fatal("NewHandler() returned nil")
	}

	if err := handler.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestHandlerImplementsHTTPHandler(t *testing.T) {
	t.Parallel()

	handler := NewHandler(context.Background(), "token", stubMessageHandler{}, slog.Default())
	defer handler.Close()

	req := httptest.NewRequest(http.MethodGet, "/webhook/iris", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestWebhookFacadeReexportsSDKHelpers(t *testing.T) {
	t.Parallel()

	var _ Metrics = NoopMetrics{}
	var _ Deduplicator = NoopDeduplicator{}

	opts := HandlerOptions{}
	if opts.QueueSize != 0 {
		t.Fatalf("QueueSize = %d, want 0 zero value", opts.QueueSize)
	}

	logger := slog.Default()
	ctx := context.Background()
	cfg := ResolveSDKConfig([]HandlerOption{
		WithWebhookToken("wh-token"),
		WithWebhookLogger(logger),
		WithContext(ctx),
	})
	if cfg.Token != "wh-token" {
		t.Fatalf("Token = %q, want %q", cfg.Token, "wh-token")
	}
	if cfg.Logger != logger {
		t.Fatal("Logger mismatch")
	}
	if cfg.Ctx != ctx {
		t.Fatal("Ctx mismatch")
	}
}
