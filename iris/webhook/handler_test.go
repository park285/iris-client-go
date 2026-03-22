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
