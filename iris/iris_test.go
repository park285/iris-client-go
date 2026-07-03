package iris

import (
	"context"
	"errors"
	"reflect"
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

func TestFacadeContractsExcludeLegacyMethods(t *testing.T) {
	t.Parallel()

	clientType := reflect.TypeOf((*Client)(nil)).Elem()
	if _, ok := clientType.MethodByName("Query"); ok {
		t.Fatal("Client must not expose legacy Query")
	}
	if _, ok := clientType.MethodByName("Decrypt"); ok {
		t.Fatal("Client must not expose legacy Decrypt")
	}

}

func TestFacadeKeepsCertReloadOptional(t *testing.T) {
	t.Parallel()

	clientType := reflect.TypeOf((*Client)(nil)).Elem()
	if _, ok := clientType.MethodByName("ReloadH3Certificate"); ok {
		t.Fatal("Client must not require ReloadH3Certificate")
	}
}

func TestFacadeSDKResolversExposeExpectedConfig(t *testing.T) {
	t.Parallel()

	clientCfg := ResolveClientSDKConfig([]ClientOption{
		WithBaseURL("http://localhost:3000"),
		WithBotToken("bot-token"),
	})
	if clientCfg.BaseURL != "http://localhost:3000" {
		t.Fatalf("BaseURL = %q, want %q", clientCfg.BaseURL, "http://localhost:3000")
	}
	if clientCfg.BotToken != "bot-token" {
		t.Fatalf("BotToken = %q, want %q", clientCfg.BotToken, "bot-token")
	}
}

func TestFacadeReexportsOperationalTypes(t *testing.T) {
	t.Parallel()

	clientCfg := ClientSDKConfig{}
	if clientCfg.BaseURL != "" || clientCfg.BotToken != "" {
		t.Fatal("ClientSDKConfig zero value mismatch")
	}
	certReload := CertReloadResponse{}
	if certReload.Status != "" {
		t.Fatal("CertReloadResponse zero value mismatch")
	}
}

func TestFacadeReexportsErrorContracts(t *testing.T) {
	t.Parallel()

	err := &HTTPError{StatusCode: 503, URL: "/reply"}
	if !errors.Is(err, ErrRetryable) {
		t.Fatal("HTTPError 503 must match ErrRetryable through facade")
	}

	var got *HTTPError
	if !errors.As(err, &got) {
		t.Fatal("HTTPError alias must be extractable through facade")
	}
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
