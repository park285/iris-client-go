package preset

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	legacyclient "github.com/park285/iris-client-go/client"
	iris "github.com/park285/iris-client-go/iris"
	iriswebhook "github.com/park285/iris-client-go/iris/webhook"
	legacywebhook "github.com/park285/iris-client-go/webhook"
)

type testMessageHandler struct{}

func (testMessageHandler) HandleMessage(context.Context, *legacywebhook.Message) {}

type testMetrics struct {
	legacywebhook.NoopMetrics
}

type testDeduplicator struct {
	legacywebhook.NoopDeduplicator
}

func TestClientOptionsBuildLegacyAndBotFacingClients(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := ClientConfig{
		Logger:                logger,
		Transport:             "http1",
		Timeout:               2 * time.Second,
		DialTimeout:           3 * time.Second,
		ResponseHeaderTimeout: 4 * time.Second,
		IdleConnTimeout:       5 * time.Second,
		MaxIdleConns:          6,
		MaxIdleConnsPerHost:   7,
	}

	opts := ClientOptions(cfg)
	if len(opts) == 0 {
		t.Fatal("ClientOptions() must return shared client options")
	}

	assertClientConfig(t, legacyclient.NewH2CClient("http://localhost:3000", "token", opts...), cfg)
	assertClientConfig(t, iris.NewH2CClient("http://localhost:3000", "token", opts...), cfg)
}

func TestClientOptionsAllowCallerOverride(t *testing.T) {
	t.Parallel()

	opts := append(
		ClientOptions(ClientConfig{
			Transport:    "h2c",
			MaxIdleConns: 6,
		}),
		legacyclient.WithTransport("http1"),
		legacyclient.WithMaxIdleConns(99),
	)

	client := legacyclient.NewH2CClient("http://localhost:3000", "token", opts...)
	clientOpts := structField(t, client, "opts")

	if got := clientOpts.FieldByName("Transport").String(); got != "http1" {
		t.Fatalf("transport = %q, want %q", got, "http1")
	}

	if got := int(clientOpts.FieldByName("MaxIdleConns").Int()); got != 99 {
		t.Fatalf("max idle conns = %d, want %d", got, 99)
	}
}

func TestWebhookOptionsBuildLegacyAndBotFacingHandlers(t *testing.T) {
	t.Parallel()

	metrics := &testMetrics{}
	dedup := &testDeduplicator{}
	cfg := WebhookConfig{
		Metrics:        metrics,
		Deduplicator:   dedup,
		WorkerCount:    2,
		QueueSize:      3,
		EnqueueTimeout: 4 * time.Millisecond,
		HandlerTimeout: 5 * time.Second,
		RequireHTTP2:   true,
		DedupTTL:       6 * time.Second,
		DedupTimeout:   7 * time.Millisecond,
		MaxBodyBytes:   8,
	}

	opts := WebhookOptions(cfg)
	if len(opts) == 0 {
		t.Fatal("WebhookOptions() must return shared webhook options")
	}

	legacyHandler := legacywebhook.NewHandler(context.Background(), "token", testMessageHandler{}, slog.Default(), opts...)
	t.Cleanup(func() {
		_ = legacyHandler.Close()
	})
	assertWebhookConfig(t, legacyHandler, cfg)

	botHandler := iriswebhook.NewHandler(context.Background(), "token", testMessageHandler{}, slog.Default(), opts...)
	t.Cleanup(func() {
		_ = botHandler.Close()
	})
	assertWebhookConfig(t, botHandler, cfg)
}

func TestWebhookOptionsAllowCallerOverride(t *testing.T) {
	t.Parallel()

	opts := append(
		WebhookOptions(WebhookConfig{
			WorkerCount:    1,
			QueueSize:      2,
			EnqueueTimeout: 3 * time.Millisecond,
		}),
		legacywebhook.WithQueueSize(99),
		legacywebhook.WithHandlerTimeout(11*time.Second),
	)

	handler := legacywebhook.NewHandler(context.Background(), "token", testMessageHandler{}, slog.Default(), opts...)
	t.Cleanup(func() {
		_ = handler.Close()
	})

	handlerOpts := structField(t, handler, "options")
	if got := int(handlerOpts.FieldByName("QueueSize").Int()); got != 99 {
		t.Fatalf("queue size = %d, want %d", got, 99)
	}

	if got := time.Duration(handlerOpts.FieldByName("HandlerTimeout").Int()); got != 11*time.Second {
		t.Fatalf("handler timeout = %s, want %s", got, 11*time.Second)
	}
}

func assertClientConfig(t *testing.T, client any, want ClientConfig) {
	t.Helper()

	clientValue := structField(t, client, "opts")
	if got := clientValue.FieldByName("Transport").String(); got != want.Transport {
		t.Fatalf("transport = %q, want %q", got, want.Transport)
	}

	if got := time.Duration(clientValue.FieldByName("Timeout").Int()); got != want.Timeout {
		t.Fatalf("timeout = %s, want %s", got, want.Timeout)
	}

	if got := time.Duration(clientValue.FieldByName("DialTimeout").Int()); got != want.DialTimeout {
		t.Fatalf("dial timeout = %s, want %s", got, want.DialTimeout)
	}

	if got := time.Duration(clientValue.FieldByName("ResponseHeaderTimeout").Int()); got != want.ResponseHeaderTimeout {
		t.Fatalf("response header timeout = %s, want %s", got, want.ResponseHeaderTimeout)
	}

	if got := time.Duration(clientValue.FieldByName("IdleConnTimeout").Int()); got != want.IdleConnTimeout {
		t.Fatalf("idle conn timeout = %s, want %s", got, want.IdleConnTimeout)
	}

	if got := int(clientValue.FieldByName("MaxIdleConns").Int()); got != want.MaxIdleConns {
		t.Fatalf("max idle conns = %d, want %d", got, want.MaxIdleConns)
	}

	if got := int(clientValue.FieldByName("MaxIdleConnsPerHost").Int()); got != want.MaxIdleConnsPerHost {
		t.Fatalf("max idle conns per host = %d, want %d", got, want.MaxIdleConnsPerHost)
	}

	if loggerField := structField(t, client, "logger"); loggerField.IsNil() {
		t.Fatal("logger must be configured by ClientOptions")
	}
}

func assertWebhookConfig(t *testing.T, handler any, want WebhookConfig) {
	t.Helper()

	optionsValue := structField(t, handler, "options")
	if got := int(optionsValue.FieldByName("WorkerCount").Int()); got != want.WorkerCount {
		t.Fatalf("worker count = %d, want %d", got, want.WorkerCount)
	}

	if got := int(optionsValue.FieldByName("QueueSize").Int()); got != want.QueueSize {
		t.Fatalf("queue size = %d, want %d", got, want.QueueSize)
	}

	if got := time.Duration(optionsValue.FieldByName("EnqueueTimeout").Int()); got != want.EnqueueTimeout {
		t.Fatalf("enqueue timeout = %s, want %s", got, want.EnqueueTimeout)
	}

	if got := time.Duration(optionsValue.FieldByName("HandlerTimeout").Int()); got != want.HandlerTimeout {
		t.Fatalf("handler timeout = %s, want %s", got, want.HandlerTimeout)
	}

	if got := optionsValue.FieldByName("RequireHTTP2").Bool(); got != want.RequireHTTP2 {
		t.Fatalf("require http2 = %t, want %t", got, want.RequireHTTP2)
	}

	if got := time.Duration(optionsValue.FieldByName("DedupTTL").Int()); got != want.DedupTTL {
		t.Fatalf("dedup ttl = %s, want %s", got, want.DedupTTL)
	}

	if got := time.Duration(optionsValue.FieldByName("DedupTimeout").Int()); got != want.DedupTimeout {
		t.Fatalf("dedup timeout = %s, want %s", got, want.DedupTimeout)
	}

	if got := optionsValue.FieldByName("MaxBodyBytes").Int(); got != want.MaxBodyBytes {
		t.Fatalf("max body bytes = %d, want %d", got, want.MaxBodyBytes)
	}

	metricsType := structField(t, handler, "metrics").Elem().Type()
	if wantType := reflect.TypeOf(want.Metrics); metricsType != wantType {
		t.Fatalf("metrics type = %s, want %s", metricsType, wantType)
	}

	dedupType := structField(t, handler, "dedup").Elem().Type()
	if wantType := reflect.TypeOf(want.Deduplicator); dedupType != wantType {
		t.Fatalf("dedup type = %s, want %s", dedupType, wantType)
	}
}

func structField(t *testing.T, target any, name string) reflect.Value {
	t.Helper()

	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		t.Fatalf("target must be a non-nil pointer, got %T", target)
	}

	field := value.Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("field %q not found on %T", name, target)
	}

	return field
}
