package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
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
	client, _, err := newHTTPClientWithCloser(baseURL, opts)
	if err != nil {
		return &http.Client{
			Timeout:   opts.Timeout,
			Transport: errorRoundTripper{err: err},
		}
	}

	return client
}

func newHTTPClientWithCloser(baseURL string, opts clientOptions) (*http.Client, io.Closer, error) {
	rt, closer, err := selectTransport(baseURL, opts)
	if err != nil {
		return nil, nil, err
	}

	return &http.Client{
		Timeout:   opts.Timeout,
		Transport: rt,
	}, closer, nil
}

func selectTransport(baseURL string, opts clientOptions) (http.RoundTripper, io.Closer, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse IRIS_BASE_URL: %w", err)
	}

	transport := resolveTransport(opts.Transport)
	switch transport {
	case "http1", "http", "http/1.1":
		return newHTTP1Transport(opts, false), nil, nil
	case "h3", "http3":
		if parsed.Scheme != "https" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=h3 requires https IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		rt, err := newHTTP3Transport(opts)
		if err != nil {
			return nil, nil, err
		}

		return rt, rt, nil
	case "h2c", "http2":
		if parsed.Scheme != "http" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=h2c requires http IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		return newH2CTransport(opts), nil, nil
	case "":
		if strings.EqualFold(parsed.Scheme, "http") {
			return newH2CTransport(opts), nil, nil
		}

		return newHTTP1Transport(opts, true), nil, nil
	default:
		return nil, nil, fmt.Errorf("unsupported transport: %s", transport)
	}
}

type errorRoundTripper struct {
	err error
}

func (e errorRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, e.err
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
