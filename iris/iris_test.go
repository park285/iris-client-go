package iris

import (
	"context"
	"log/slog"
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

	fullClientType := reflect.TypeOf((*FullClient)(nil)).Elem()
	if _, ok := fullClientType.MethodByName("Query"); ok {
		t.Fatal("FullClient must not expose legacy Query")
	}
	if _, ok := fullClientType.MethodByName("Decrypt"); ok {
		t.Fatal("FullClient must not expose legacy Decrypt")
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

	logger := slog.Default()
	ctx := context.Background()
	webhookCfg := ResolveWebhookSDKConfig([]HandlerOption{
		WithWebhookToken("wh-token"),
		WithWebhookLogger(logger),
		WithContext(ctx),
	})
	if webhookCfg.Token != "wh-token" {
		t.Fatalf("Token = %q, want %q", webhookCfg.Token, "wh-token")
	}
	if webhookCfg.Logger != logger {
		t.Fatal("Logger mismatch")
	}
	if webhookCfg.Ctx != ctx {
		t.Fatal("Ctx mismatch")
	}
}

func TestFacadeReexportsOperationalTypes(t *testing.T) {
	t.Parallel()

	var _ Metrics = NoopMetrics{}
	var _ Deduplicator = NoopDeduplicator{}

	handlerOpts := HandlerOptions{}
	if handlerOpts.QueueSize != 0 {
		t.Fatalf("QueueSize = %d, want 0 zero value", handlerOpts.QueueSize)
	}

	clientCfg := ClientSDKConfig{}
	if clientCfg.BaseURL != "" || clientCfg.BotToken != "" {
		t.Fatal("ClientSDKConfig zero value mismatch")
	}

	webhookCfg := WebhookSDKConfig{}
	if webhookCfg.Token != "" || webhookCfg.Logger != nil || webhookCfg.Ctx != nil {
		t.Fatal("WebhookSDKConfig zero value mismatch")
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
