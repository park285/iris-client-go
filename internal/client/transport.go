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
	if t := normalizeTransport(explicit); t != "" {
		return t
	}

	if t := normalizeTransport(os.Getenv("IRIS_TRANSPORT")); t != "" {
		return t
	}
	return transportH3
}

func normalizeTransport(value string) string {
	switch t := strings.ToLower(strings.TrimSpace(value)); t {
	case "h3", "http3", "http/3", "quic":
		return transportH3
	case transportH2C:
		return transportH2C
	case "h2", transportHTTP2:
		return transportHTTP2
	case transportHTTP1, "http", "http/1.1":
		return transportHTTP1
	default:
		return t
	}
}

func newHTTPClientWithCloser(baseURL string, opts clientOptions) (*http.Client, io.Closer, error) {
	rt, closer, err := selectTransport(baseURL, opts)
	if err != nil {
		return nil, nil, err
	}

	return &http.Client{
		Timeout:       opts.Timeout,
		Transport:     rt,
		CheckRedirect: rejectCrossHostRedirect,
	}, closer, nil
}

func rejectCrossHostRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return nil
	}
	if len(via) >= 10 {
		return fmt.Errorf("iris: stopped after %d redirects", len(via))
	}
	prev := via[len(via)-1]
	if !strings.EqualFold(req.URL.Host, prev.URL.Host) {
		return fmt.Errorf("iris: refusing cross-host redirect from %q to %q", prev.URL.Host, req.URL.Host)
	}
	return nil
}

// selectTransport은 IRIS_TRANSPORT 또는 WithTransport로 클라이언트 transport를 고른다:
// h3는 https가 필요하며 closer를 가진 HTTP/3 transport를 반환한다. h2c는 http가 필요하며
// cleartext HTTP/2 transport를 반환한다. http2는 https가 필요하며 net/http transport에서
// ForceAttemptHTTP2를 켠다. http1은 HTTP/2를 강제하지 않고 net/http transport를 쓴다.
// 기본으로 해석되는 모드는 h3다.
func selectTransport(baseURL string, opts clientOptions) (http.RoundTripper, io.Closer, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse IRIS_BASE_URL: %w", err)
	}

	transport := resolveTransport(opts.Transport)
	switch transport {
	case transportHTTP1:
		return newHTTP1Transport(opts, false), nil, nil
	case transportH3:
		if parsed.Scheme != "https" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=h3 requires https IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		caFile := resolveH3CACertFile(opts)
		if interval := resolveH3CAReloadInterval(opts); caFile != "" && interval > 0 {
			// CA 파일을 한 번만 읽어 초기 transport와 reloader의 기준 해시를 같은 바이트에서 만든다.
			// selectTransport의 초기 read와 reloader의 hash 시드 사이에 CA가 회전하면 swap이 누락되는 TOCTOU를 방지한다.
			pemBytes, rerr := os.ReadFile(caFile)
			if rerr != nil {
				return nil, nil, fmt.Errorf("read IRIS_H3_CA_CERT_FILE: %w", rerr)
			}
			rt, err := newHTTP3TransportFromCA(opts, true, pemBytes)
			if err != nil {
				return nil, nil, err
			}
			reloader := newReloadingH3Transport(rt, opts, caFile, interval, pemBytes)
			return reloader, reloader, nil
		}

		rt, err := newHTTP3Transport(opts)
		if err != nil {
			return nil, nil, err
		}
		return rt, rt, nil
	case transportH2C:
		if parsed.Scheme != "http" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=h2c requires http IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		return newH2CTransport(opts), nil, nil
	case transportHTTP2:
		if parsed.Scheme != "https" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=http2 requires https IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		return newHTTP1Transport(opts, true), nil, nil
	case "":
		return nil, nil, fmt.Errorf("IRIS_TRANSPORT is required")
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
