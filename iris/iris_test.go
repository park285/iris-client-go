package iris

import (
	"context"
	"testing"
)

func TestNewH2CClientReturnsBotFacingClient(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://localhost:3000", "token")
	if client == nil {
		t.Fatal("NewH2CClient() returned nil")
	}
}

func TestClientInterfaceIncludesSenderAndAdmin(t *testing.T) {
	t.Parallel()

	var _ Client = NewH2CClient("http://localhost:3000", "token")
}

func TestSendOptionsExposeThreadHelpers(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://localhost:3000", "token")

	if err := client.SendMessage(context.Background(), "room", "hello",
		WithThreadID("12345"),
		WithThreadScope(2),
	); err == nil {
		t.Fatal("expected transport error with localhost target, got nil")
	}
}
