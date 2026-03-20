package client

import (
	"log/slog"
	"net/http"
	"testing"
	"time"

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

	if got := resolveTransport(""); got != "" {
		t.Fatalf("resolveTransport() = %q, want empty", got)
	}
}

func TestSelectTransport(t *testing.T) {
	opts := applyClientOptions([]ClientOption{WithLogger(slog.Default())})

	tests := []struct {
		name      string
		baseURL   string
		transport string
		wantType  string
	}{
		{name: "explicit http1", baseURL: "http://example.com", transport: "http1", wantType: "http1"},
		{name: "default h2c for http", baseURL: "http://example.com", transport: "", wantType: "h2c"},
		{name: "explicit h2c for http", baseURL: "http://example.com", transport: "h2c", wantType: "h2c"},
		{name: "https falls back to http1", baseURL: "https://example.com", transport: "", wantType: "http1"},
		{name: "unknown transport on http keeps h2c detection", baseURL: "http://example.com", transport: "weird", wantType: "h2c"},
		{name: "invalid url falls back to http1", baseURL: "://bad", transport: "", wantType: "http1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			localOpts := opts

			localOpts.Transport = tt.transport

			got := selectTransport(tt.baseURL, localOpts)

			switch tt.wantType {
			case "http1":
				if _, ok := got.(*http.Transport); !ok {
					t.Fatalf("selectTransport() returned %T, want *http.Transport", got)
				}
			case "h2c":
				if _, ok := got.(*http2.Transport); !ok {
					t.Fatalf("selectTransport() returned %T, want *http2.Transport", got)
				}
			default:
				t.Fatalf("unknown wantType %q", tt.wantType)
			}
		})
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
	})

	tr := newHTTP1Transport(opts)
	if tr.MaxIdleConns != 11 {
		t.Fatalf("MaxIdleConns = %d, want 11", tr.MaxIdleConns)
	}

	if tr.MaxIdleConnsPerHost != 12 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 12", tr.MaxIdleConnsPerHost)
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
	opts := applyClientOptions([]ClientOption{WithTimeout(2 * time.Second)})

	client := newHTTPClient("http://example.com", opts)
	if client.Timeout != 2*time.Second {
		t.Fatalf("Timeout = %v, want 2s", client.Timeout)
	}
}
