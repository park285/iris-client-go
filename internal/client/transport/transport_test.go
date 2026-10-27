package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/park285/iris-client-go/v4/internal/testsupport"
)

func TestParseTransport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value   string
		want    string
		wantErr bool
	}{
		{value: "h3", want: TransportH3},
		{value: " H3 ", want: TransportH3},
		{value: "  HTTP1 ", want: TransportHTTP1},
		{value: "", wantErr: true},
		{value: "http3", wantErr: true},
		{value: "http/3", wantErr: true},
		{value: "quic", wantErr: true},
		{value: "http", wantErr: true},
		{value: "http/1.1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Parallel()

			got, err := ParseTransport(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParseTransport(%q) = %q, want error", tt.value, got)
				}

				return
			}

			if err != nil || got != tt.want {
				t.Fatalf("ParseTransport(%q) = (%q, %v), want %q", tt.value, got, err, tt.want)
			}
		})
	}
}

func TestResolveTransportDefaultsToH3OnlyWhenEmpty(t *testing.T) {
	t.Parallel()

	if got, err := resolveTransport(" "); err != nil || got != TransportH3 {
		t.Fatalf("resolveTransport(blank) = (%q, %v), want h3", got, err)
	}

	if _, err := resolveTransport("quic"); err == nil {
		t.Fatal("resolveTransport(quic) error = nil, want rejection")
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	return parsed
}

func TestSelectTransport(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{withTestH3CA(t)})

	tests := []struct {
		name      string
		baseURL   string
		transport string
		wantType  string
		wantErr   bool
	}{
		{name: "http1 loopback", baseURL: "http://127.0.0.1:8080", transport: TransportHTTP1, wantType: TransportHTTP1},
		{name: "http1 localhost", baseURL: "http://localhost:8080", transport: TransportHTTP1, wantType: TransportHTTP1},
		{name: "http1 rejects non-loopback host", baseURL: "http://example.com", transport: TransportHTTP1, wantErr: true},
		{name: "h3 rejects http", baseURL: testExampleBaseURL, transport: TransportH3, wantErr: true},
		{name: "h3 https", baseURL: "https://example.com", transport: TransportH3, wantType: TransportH3},
		{name: "unknown transport errors", baseURL: "https://example.com", transport: "weird", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, closer, err := selectTransport(mustParseURL(t, tt.baseURL), tt.transport, opts)
			if closer != nil {
				t.Cleanup(func() { _ = closer.Close() })
			}

			if tt.wantErr {
				if err == nil {
					t.Fatal("selectTransport() error = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("selectTransport() error = %v", err)
			}

			assertSelectedTransport(t, got, tt.wantType)
		})
	}
}

func assertSelectedTransport(t *testing.T, got http.RoundTripper, wantType string) {
	t.Helper()

	switch wantType {
	case TransportHTTP1:
		testsupport.AssertType[*http.Transport](t, "selectTransport()", got)
	case TransportH3:
		testsupport.AssertType[*http3.Transport](t, "selectTransport()", got)
	default:
		t.Fatalf("unknown wantType %q", wantType)
	}
}

func TestSelectTransportH3ReturnsHTTP3TransportWithCloser(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{withTestH3CA(t)})

	rt, closer, err := selectTransport(mustParseURL(t, "https://example.com"), TransportH3, opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	t.Cleanup(func() { _ = closer.Close() })

	h3Transport, ok := rt.(*http3.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	if got := h3Transport.QUICConfig.InitialPacketSize; got != 1200 {
		t.Fatalf("InitialPacketSize = %d, want 1200", got)
	}
}

func TestSelectTransportH3AppliesDialGuard(t *testing.T) {
	t.Parallel()

	blocked := errors.New("blocked h3 egress")

	var gotIP net.IP

	opts := applyClientOptions([]ClientOption{
		withTestH3CA(t),
		WithH3DialGuard(func(_ context.Context, ip net.IP) error {
			gotIP = append(net.IP(nil), ip...)

			return blocked
		}),
	})

	rt, closer, err := selectTransport(mustParseURL(t, "https://example.com"), TransportH3, opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	t.Cleanup(func() { _ = closer.Close() })

	h3Transport, ok := rt.(*http3.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	_, err = h3Transport.Dial(t.Context(), "127.0.0.1:443", &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
	if !errors.Is(err, ErrH3EgressDenied) || !errors.Is(err, blocked) || !errors.Is(err, ErrNotSent) {
		t.Fatalf("Dial() error = %v, want ErrNotSent wrapping ErrH3EgressDenied and %v", err, blocked)
	}

	if !gotIP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("guard IP = %v, want 127.0.0.1", gotIP)
	}
}

func TestNewHTTP1TransportUsesFixedLimits(t *testing.T) {
	t.Parallel()

	tr := newHTTP1Transport()

	if tr.Protocols == nil || tr.Protocols.String() != "{HTTP1}" {
		t.Fatalf("Protocols = %v, want HTTP/1.1 only", tr.Protocols)
	}

	if tr.MaxIdleConns != defaultHTTP1MaxIdleConns || tr.MaxIdleConnsPerHost != defaultHTTP1MaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConns = %d/%d, want %d/%d", tr.MaxIdleConns, tr.MaxIdleConnsPerHost,
			defaultHTTP1MaxIdleConns, defaultHTTP1MaxIdleConnsPerHost)
	}

	if tr.MaxConnsPerHost != defaultMaxConnsPerHost {
		t.Fatalf("MaxConnsPerHost = %d, want %d", tr.MaxConnsPerHost, defaultMaxConnsPerHost)
	}

	if tr.IdleConnTimeout != defaultHTTP1IdleConnTimeout || tr.ResponseHeaderTimeout != defaultHTTP1HeaderTimeout {
		t.Fatalf("IdleConnTimeout = %v, ResponseHeaderTimeout = %v", tr.IdleConnTimeout, tr.ResponseHeaderTimeout)
	}
}

func TestNewHTTPClientAppliesTimeout(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTimeout(2 * time.Second)})

	client, closer, err := newHTTPClientWithCloser(mustParseURL(t, "http://127.0.0.1:1"), TransportHTTP1, opts)
	if err != nil {
		t.Fatalf("newHTTPClientWithCloser() error = %v", err)
	}

	if closer != nil {
		t.Fatalf("closer = %v, want nil for net/http transport", closer)
	}

	if client.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", client.Timeout)
	}
}

// SDK 소유 http1 transport는 Iris HTTP/1 listener가 처리하는 loopback GET /health·/ready만 보낸다.
// 보호 route는 서명된 본문을 평문으로 내보내기 전에 ErrNotSent로 거절하고 네트워크 요청은 0건이다.
func TestHTTP1TransportAllowsOnlyLoopbackProbes(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		if r.Method != http.MethodGet || r.URL.Path != PathReady {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL), WithBotToken("token"), WithTransport(TransportHTTP1))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if pingErr := client.Ping(t.Context()); pingErr != nil {
		t.Fatalf("Ping() error = %v", pingErr)
	}

	_, err = client.SendMessageAccepted(t.Context(), testRoomA, "hello")
	if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrNotSent and not ErrTransport", err)
	}

	if _, err := client.GetRooms(t.Context()); !errors.Is(err, ErrNotSent) {
		t.Fatalf("GetRooms() error = %v, want ErrNotSent", err)
	}

	if got := requests.Load(); got != 1 {
		t.Fatalf("server requests = %d, want only the /ready probe", got)
	}

	if _, err := NewClient(WithBaseURL("http://example.com"), WithBotToken("token"), WithTransport(TransportHTTP1)); err == nil {
		t.Fatal("NewClient(http1, non-loopback) error = nil, want rejection")
	}
}
