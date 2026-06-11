package client

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	"golang.org/x/net/http2"
)

func TestResolveTransport(t *testing.T) {
	t.Setenv("IRIS_TRANSPORT", "  H2C ")

	tests := []struct {
		name     string
		explicit string
		want     string
	}{
		{name: "explicit wins", explicit: "  HTTP1 ", want: "http1"},
		{name: "env fallback", explicit: "", want: "h2c"},
		{name: "h3 alias", explicit: " HTTP/3 ", want: "h3"},
		{name: "quic alias", explicit: " QUIC ", want: "h3"},
		{name: "h2 alias", explicit: " H2 ", want: "http2"},
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
	opts := applyClientOptions([]ClientOption{WithLogger(slog.Default())})

	tests := []struct {
		name      string
		baseURL   string
		transport string
		wantType  string
		wantErr   bool
	}{
		{name: "explicit http1", baseURL: "http://example.com", transport: "http1", wantType: "http1"},
		{name: "default h3 rejects http", baseURL: "http://example.com", transport: "", wantErr: true},
		{name: "explicit h2c for http", baseURL: "http://example.com", transport: "h2c", wantType: "h2c"},
		{name: "explicit h2 alias for https", baseURL: "https://example.com", transport: "h2", wantType: "http2"},
		{name: "explicit http2 for https", baseURL: "https://example.com", transport: "http2", wantType: "http2"},
		{name: "https defaults to h3", baseURL: "https://example.com", transport: "", wantType: "h3"},
		{name: "unknown transport errors", baseURL: "http://example.com", transport: "weird", wantErr: true},
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

			switch tt.wantType {
			case "http1":
				if _, ok := got.(*http.Transport); !ok {
					t.Fatalf("selectTransport() returned %T, want *http.Transport", got)
				}
			case "h2c":
				if _, ok := got.(*http2.Transport); !ok {
					t.Fatalf("selectTransport() returned %T, want *http2.Transport", got)
				}
			case "http2":
				tr, ok := got.(*http.Transport)
				if !ok {
					t.Fatalf("selectTransport() returned %T, want *http.Transport", got)
				}
				if !tr.ForceAttemptHTTP2 {
					t.Fatal("ForceAttemptHTTP2 = false, want true")
				}
			case "h3":
				if _, ok := got.(*http3.Transport); !ok {
					t.Fatalf("selectTransport() returned %T, want *http3.Transport", got)
				}
			default:
				t.Fatalf("unknown wantType %q", tt.wantType)
			}
		})
	}
}

func TestSelectTransportExplicitH3RequiresHTTPS(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3")})

	if _, _, err := selectTransport("https://example.com", opts); err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}
}

func TestSelectTransportExplicitH3RejectsHTTP(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3")})

	if _, _, err := selectTransport("http://example.com", opts); err == nil {
		t.Fatal("selectTransport() error = nil, want h3 to reject http URL")
	}
}

func TestSelectTransportExplicitH3ReturnsHTTP3Transport(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h3")})

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

func TestSelectTransportExplicitH3AliasesReturnHTTP3Transport(t *testing.T) {
	t.Parallel()

	for _, transport := range []string{"http3", "http/3", "quic"} {
		t.Run(transport, func(t *testing.T) {
			t.Parallel()

			opts := applyClientOptions([]ClientOption{WithTransport(transport)})

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

func TestExplicitH2CRequiresHTTP(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("h2c")})

	if _, _, err := selectTransport("https://example.com", opts); err == nil {
		t.Fatal("selectTransport() error = nil, want h2c to reject https URL")
	}
}

func TestExplicitHTTP2RequiresHTTPS(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("http2")})

	if _, _, err := selectTransport("http://example.com", opts); err == nil {
		t.Fatal("selectTransport() error = nil, want http2 to reject http URL")
	}
}

func TestDefaultTransportUsesH3ForHTTPS(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions(nil)

	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	if _, ok := rt.(*http3.Transport); !ok {
		t.Fatalf("selectTransport() returned %T, want *http3.Transport", rt)
	}

	if closer == nil {
		t.Fatalf("closer = nil, want HTTP/3 transport closer")
	}
}

func TestSelectTransport_HTTP1Mode_DoesNotForceHTTP2(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("http1")})
	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http.Transport", rt)
	}
	if tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = true, want false for explicit http1")
	}
}

func TestSelectTransport_HTTP2Mode_ForcesHTTP2(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("http2")})
	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatalf("selectTransport() returned %T, want *http.Transport", rt)
	}
	if !tr.ForceAttemptHTTP2 {
		t.Fatal("ForceAttemptHTTP2 = false, want true for explicit http2")
	}
}

func TestExplicitHTTP2AllowsHTTP2Negotiation(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("http2")})
	rt, _, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport() error = %v", err)
	}

	tr, ok := rt.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}

	if !tr.ForceAttemptHTTP2 {
		t.Fatal("explicit HTTP/2 transport should allow HTTP/2 negotiation")
	}
}

func TestMaxConnsPerHostApplied(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{
		WithTransport("http1"),
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

	opts := applyClientOptions([]ClientOption{WithTransport("http1")})
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

func TestMaxConnsPerHostDefaultsAppliedToHTTP2(t *testing.T) {
	t.Parallel()

	opts := applyClientOptions([]ClientOption{WithTransport("http2")})
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

	tr := newHTTP1Transport(opts, true)
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

func TestNewH2CTransportAppliesOptions(t *testing.T) {
	opts := applyClientOptions([]ClientOption{
		WithReadIdleTimeout(13 * time.Second),
		WithPingTimeout(14 * time.Second),
		WithWriteByteTimeout(15 * time.Second),
	})

	tr := newH2CTransport(opts)
	if !tr.AllowHTTP {
		t.Fatal("AllowHTTP = false, want true")
	}

	if tr.ReadIdleTimeout != 13*time.Second {
		t.Fatalf("ReadIdleTimeout = %v, want 13s", tr.ReadIdleTimeout)
	}

	if tr.PingTimeout != 14*time.Second {
		t.Fatalf("PingTimeout = %v, want 14s", tr.PingTimeout)
	}

	if tr.WriteByteTimeout != 15*time.Second {
		t.Fatalf("WriteByteTimeout = %v, want 15s", tr.WriteByteTimeout)
	}
}

func TestNewHTTPClientAppliesTimeout(t *testing.T) {
	opts := applyClientOptions([]ClientOption{
		WithTransport("http1"),
		WithTimeout(2 * time.Second),
	})

	client, closer, err := newHTTPClientWithCloser("http://example.com", opts)
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
