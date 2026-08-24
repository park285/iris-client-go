package transport

import (
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/park285/iris-client-go/v2/internal/testsupport"
)

func TestResolveTransport(t *testing.T) {
	t.Setenv("IRIS_TRANSPORT", "  HTTP/1.1 ")

	tests := []struct {
		name     string
		explicit string
		want     string
	}{
		{name: "explicit wins", explicit: "  HTTP1 ", want: transportHTTP1},
		{name: "env fallback", explicit: "", want: transportHTTP1},
		{name: "h3 alias", explicit: " HTTP/3 ", want: "h3"},
		{name: "quic alias", explicit: " QUIC ", want: "h3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveTransport(tt.explicit); got != tt.want {
				t.Fatalf("resolveTransport() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTransportEmptyWhenUnset(t *testing.T) {
	t.Setenv("IRIS_TRANSPORT", "")

	if got := resolveTransport(""); got != "h3" {
		t.Fatalf("resolveTransport() = %q, want h3", got)
	}
}

func TestSelectTransport(t *testing.T) {
	opts := applyClientOptions([]ClientOption{WithLogger(slog.Default()), WithH3AllowSystemRoots(true)})

	tests := []struct {
		name      string
		baseURL   string
		transport string
		wantType  string
		wantErr   bool
	}{
		{name: "explicit http1", baseURL: testExampleBaseURL, transport: transportHTTP1, wantType: transportHTTP1},
		{name: "default h3 rejects http", baseURL: testExampleBaseURL, transport: "", wantErr: true},
		{name: "https defaults to h3", baseURL: "https://example.com", transport: "", wantType: "h3"},
		{name: "unknown transport errors", baseURL: testExampleBaseURL, transport: "weird", wantErr: true},
		{name: "invalid url errors", baseURL: "://bad", transport: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localOpts := opts

			localOpts.Transport = tt.transport

			got, _, err := selectTransport(tt.baseURL, localOpts)
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
	case transportHTTP1:
		testsupport.AssertType[*http.Transport](t, "selectTransport()", got)
	case "h3":
		testsupport.AssertType[*http3.Transport](t, "selectTransport()", got)
	default:
		t.Fatalf("unknown wantType %q", wantType)
	}
}

func TestSelectTransportExplicitH3RequiresHTTPS(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3"), WithH3AllowSystemRoots(true)})

	if _, _, err := selectTransport("https://example.com", opts); err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}
}

func TestSelectTransportExplicitH3RejectsHTTP(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3")})

	if _, _, err := selectTransport(testExampleBaseURL, opts); err == nil {
		t.Fatal("selectTransport() error = nil, want h3 to reject http URL")
	}
}

func TestSelectTransportExplicitH3ReturnsHTTP3Transport(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3"), WithH3AllowSystemRoots(true)})

	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	h3Transport, ok := rt.(*http3.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	if got := h3Transport.QUICConfig.InitialPacketSize; got != 1200 {
		t.Fatalf("InitialPacketSize = %d, want 1200", got)
	}

	if closer == nil {
		t.Fatal("closer = nil, want HTTP/3 transport closer")
	}
}

func TestSelectTransportH3AppliesDialGuard(t *testing.T) {
	t.Parallel()

	blocked := errors.New("blocked h3 egress")

	var gotIP net.IP

	opts := applyClientOptions([]ClientOption{
		WithTransport("h3"),
		WithH3AllowSystemRoots(true),
		WithH3DialGuard(func(ip net.IP) error {
			gotIP = append(net.IP(nil), ip...)

			return blocked
		}),
	})

	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	h3Transport, ok := rt.(*http3.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	if h3Transport.Dial == nil {
		t.Fatal("Dial is nil, want guard-wrapped dial")
	}

	_, err = h3Transport.Dial(t.Context(), "127.0.0.1:443", &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
	if !errors.Is(err, ErrH3EgressDenied) {
		t.Fatalf("Dial() error = %v, want ErrH3EgressDenied", err)
	}

	if !errors.Is(err, blocked) {
		t.Fatalf("Dial() error = %v, want %v", err, blocked)
	}

	if !gotIP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("guard IP = %v, want 127.0.0.1", gotIP)
	}
}

func TestSelectTransportExplicitH3AliasesReturnHTTP3Transport(t *testing.T) {
	t.Parallel()

	for _, transport := range []string{"http3", "http/3", "quic"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()

			opts := applyClientOptions([]ClientOption{WithTransport(transport), WithH3AllowSystemRoots(true)})

			rt, closer, err := selectTransport("https://example.com", opts)
			if err != nil {
				t.Fatalf("selectTransport() error = %v", err)
			}

			if _, ok := rt.(*http3.Transport); !ok {
				t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
			}

			if closer == nil {
				t.Fatal("closer = nil, want HTTP/3 transport closer")
			}
		})
	}
}

func TestDefaultTransportUsesH3ForHTTPS(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithH3AllowSystemRoots(true)})

	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	if _, ok := rt.(*http3.Transport); !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	if closer == nil {
		t.Fatal("closer = nil, want HTTP/3 transport closer")
	}
}

func TestSelectTransportHTTP1ModeEnablesOnlyHTTP1(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport(transportHTTP1)})

	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http.Transport", rt)
	}

	if tr.Protocols == nil || tr.Protocols.String() != "{HTTP1}" {
		t.Fatalf("Protocols = %v, want HTTP/1.1 only", tr.Protocols)
	}
}

func TestMaxConnsPerHostApplied(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{
		WithTransport(transportHTTP1),
		WithMaxConnsPerHost(42),
	})

	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}

	if tr.MaxConnsPerHost != 42 {
		t.Fatalf("MaxConnsPerHost = %d, want 42", tr.MaxConnsPerHost)
	}
}

func TestMaxConnsPerHostDefaultsAppliedToHTTP1(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport(transportHTTP1)})

	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http.Transport", rt)
	}

	if tr.MaxConnsPerHost != 32 {
		t.Fatalf("MaxConnsPerHost = %d, want default 32", tr.MaxConnsPerHost)
	}
}

func TestNewHTTP1TransportAppliesOptions(t *testing.T) {
	opts := applyClientOptions([]ClientOption{
		WithDialTimeout(4 * time.Second),
		WithTLSHandshakeTimeout(6 * time.Second),
		WithResponseHeaderTimeout(7 * time.Second),
		WithIdleConnTimeout(8 * time.Second),
		WithMaxIdleConns(11),
		WithMaxIdleConnsPerHost(12),
		WithMaxConnsPerHost(13),
	})

	tr := newHTTP1Transport(opts)
	if tr.MaxIdleConns != 11 {
		t.Fatalf("MaxIdleConns = %d, want 11", tr.MaxIdleConns)
	}

	if tr.MaxIdleConnsPerHost != 12 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 12", tr.MaxIdleConnsPerHost)
	}

	if tr.MaxConnsPerHost != 13 {
		t.Fatalf("MaxConnsPerHost = %d, want 13", tr.MaxConnsPerHost)
	}

	if tr.IdleConnTimeout != 8*time.Second {
		t.Fatalf("IdleConnTimeout = %v, want 8s", tr.IdleConnTimeout)
	}

	if tr.TLSHandshakeTimeout != 6*time.Second {
		t.Fatalf("TLSHandshakeTimeout = %v, want 6s", tr.TLSHandshakeTimeout)
	}

	if tr.ResponseHeaderTimeout != 7*time.Second {
		t.Fatalf("ResponseHeaderTimeout = %v, want 7s", tr.ResponseHeaderTimeout)
	}
}

func TestNewHTTPClientAppliesTimeout(t *testing.T) {
	opts := applyClientOptions([]ClientOption{
		WithTransport(transportHTTP1),
		WithTimeout(2 * time.Second),
	})

	client, closer, err := newHTTPClientWithCloser(testExampleBaseURL, opts)
	if err != nil {
		t.Fatalf("newHTTPClientWithCloser() error = %v", err)
	}

	if closer != nil {
		t.Cleanup(func() { _ = closer.Close() })
	}

	if client == nil {
		t.Fatal("client = nil")
	}

	if client.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", client.Timeout)
	}
}
