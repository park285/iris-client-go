package transport

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

const (
	testTraceID = "0123456789abcdef0123456789abcdef"
	testSpanID  = "0123456789abcdef"
)

func TestNewSignedRequestInjectsTraceparent(t *testing.T) {
	t.Parallel()

	ctx := trace.ContextWithSpanContext(context.Background(), testSpanContext(t))
	client := NewH2CClient("http://iris.invalid", "", WithHMACSecret("trace-secret"))

	req, err := client.newSignedRequest(ctx, http.MethodGet, PathReady, nil, SecretRoleBotControl)
	if err != nil {
		t.Fatalf("newSignedRequest() error = %v", err)
	}

	traceparent := req.Header.Get("traceparent")
	parts := strings.Split(traceparent, "-")
	if len(parts) != 4 {
		t.Fatalf("traceparent = %q, want four fields", traceparent)
	}
	if got := parts[1]; got != testTraceID {
		t.Fatalf("traceparent trace ID = %q, want %q", got, testTraceID)
	}
}

func TestNewSignedRequestWithoutSpanContextOmitsTraceparent(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://iris.invalid", "", WithHMACSecret("trace-secret"))

	req, err := client.newSignedRequest(context.Background(), http.MethodGet, PathReady, nil, SecretRoleBotControl)
	if err != nil {
		t.Fatalf("newSignedRequest() error = %v", err)
	}

	if got := req.Header.Get("traceparent"); got != "" {
		t.Fatalf("traceparent = %q, want empty", got)
	}
}

func TestNewSignedStreamRequestInjectsTraceparent(t *testing.T) {
	t.Parallel()

	const body = "stream body"
	ctx := trace.ContextWithSpanContext(context.Background(), testSpanContext(t))
	client := NewH2CClient("http://iris.invalid", "", WithHMACSecret("trace-secret"))

	req, err := client.newSignedStreamRequest(
		ctx,
		http.MethodPost,
		PathReply,
		strings.NewReader(body),
		sha256HexBytes([]byte(body)),
		SecretRoleBotControl,
	)
	if err != nil {
		t.Fatalf("newSignedStreamRequest() error = %v", err)
	}

	if got := req.Header.Get("traceparent"); got == "" {
		t.Fatal("traceparent header missing")
	}
}

func TestTraceparentDoesNotAffectHMACSignature(t *testing.T) {
	t.Parallel()

	const (
		secret = "trace-hmac-secret"
		method = http.MethodPost
		path   = PathReply
		body   = `{"room":"room","data":"message"}`
	)

	client := NewH2CClient("http://iris.invalid", "", WithHMACSecret(secret))
	withTrace, err := client.newSignedRequest(
		trace.ContextWithSpanContext(context.Background(), testSpanContext(t)),
		method,
		path,
		[]byte(body),
		SecretRoleBotControl,
	)
	if err != nil {
		t.Fatalf("newSignedRequest(with trace) error = %v", err)
	}
	withoutTrace, err := client.newSignedRequest(context.Background(), method, path, []byte(body), SecretRoleBotControl)
	if err != nil {
		t.Fatalf("newSignedRequest(without trace) error = %v", err)
	}

	if withTrace.Header.Get("traceparent") == "" {
		t.Fatal("traceparent header missing with span context")
	}
	if got := withoutTrace.Header.Get("traceparent"); got != "" {
		t.Fatalf("traceparent = %q without span context, want empty", got)
	}

	assertRequestHMACValid(t, withTrace, secret, path)
	assertRequestHMACValid(t, withoutTrace, secret, path)
}

func testSpanContext(t *testing.T) trace.SpanContext {
	t.Helper()

	traceID, err := trace.TraceIDFromHex(testTraceID)
	if err != nil {
		t.Fatalf("TraceIDFromHex() error = %v", err)
	}
	spanID, err := trace.SpanIDFromHex(testSpanID)
	if err != nil {
		t.Fatalf("SpanIDFromHex() error = %v", err)
	}
	return trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	})
}

func assertRequestHMACValid(t *testing.T, req *http.Request, secret, path string) {
	t.Helper()

	want := mustSignIrisRequestWithBodySHA256(
		t,
		secret,
		req.Method,
		path,
		req.Header.Get(HeaderIrisTimestamp),
		req.Header.Get(HeaderIrisNonce),
		req.Header.Get(HeaderIrisBodySHA256),
	)
	if got := req.Header.Get(HeaderIrisSignature); got != want {
		t.Fatalf("X-Iris-Signature = %q, want %q", got, want)
	}
}
