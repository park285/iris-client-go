package client

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/http2"
)

func resolveTransport(explicit string) string {
	if t := strings.ToLower(strings.TrimSpace(explicit)); t != "" {
		return t
	}

	return strings.ToLower(strings.TrimSpace(os.Getenv("IRIS_TRANSPORT")))
}

func newHTTPClient(baseURL string, opts clientOptions) *http.Client {
	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: selectTransport(baseURL, opts),
	}
}

func selectTransport(baseURL string, opts clientOptions) http.RoundTripper {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		if opts.Logger != nil {
			opts.Logger.Warn("iris_base_url_invalid", "base_url", baseURL, "error", err)
		}

		return newHTTP1Transport(opts, true)
	}

	transport := resolveTransport(opts.Transport)
	switch transport {
	case "http1", "http", "http/1.1":
		return newHTTP1Transport(opts, false)
	case "", "h2c", "http2":
		// proceed to h2c detection
	default:
		if opts.Logger != nil {
			opts.Logger.Warn("iris_transport_unknown", "transport", transport)
		}
	}

	if strings.EqualFold(parsed.Scheme, "http") {
		return newH2CTransport(opts)
	}

	return newHTTP1Transport(opts, true)
}

func newHTTP1Transport(opts clientOptions, forceHTTP2 bool) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   opts.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     forceHTTP2,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       opts.IdleConnTimeout,
		TLSHandshakeTimeout:   opts.TLSHandshakeTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
	}
}

func newH2CTransport(opts clientOptions) *http2.Transport {
	dialer := &net.Dialer{
		Timeout:   opts.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	return &http2.Transport{
		AllowHTTP:        true,
		IdleConnTimeout:  opts.IdleConnTimeout,
		ReadIdleTimeout:  opts.ReadIdleTimeout,
		PingTimeout:      opts.PingTimeout,
		WriteByteTimeout: opts.WriteByteTimeout,
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			return dialer.DialContext(ctx, network, addr)
		},
	}
}
