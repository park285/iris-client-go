package transport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
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
	case transportHTTP1, "http", "http/1.1":
		return transportHTTP1
	default:
		return t
	}
}

func newHTTPClientWithCloser(baseURL string, opts clientOptions) (*http.Client, io.Closer, error) {
	rt, closer, err := selectTransport(baseURL, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("select transport: %w", err)
	}

	return cloneHTTPClientWithRedirectPolicy(&http.Client{
		Timeout:   opts.Timeout,
		Transport: rt,
	}), closer, nil
}

func cloneHTTPClientWithRedirectPolicy(source *http.Client) *http.Client {
	cloned := *source
	callerPolicy := source.CheckRedirect

	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 0 && hasIrisSigningHeaders(via[0].Header) {
			origin := via[0].URL

			return fmt.Errorf(
				"iris: refusing redirect of signed request from %q to %q",
				origin.Scheme+"://"+origin.Host,
				req.URL.Scheme+"://"+req.URL.Host,
			)
		}

		if callerPolicy != nil {
			return callerPolicy(req, via)
		}

		if len(via) >= 10 {
			return fmt.Errorf("iris: stopped after %d redirects", len(via))
		}

		return nil
	}

	return &cloned
}

func cloneHTTPClient(source *http.Client) *http.Client {
	cloned := *source
	return &cloned
}

func hasIrisSigningHeaders(header http.Header) bool {
	for _, name := range []string{
		HeaderIrisTimestamp,
		HeaderIrisNonce,
		HeaderIrisSignature,
		HeaderIrisBodySHA256,
	} {
		if header.Get(name) != "" {
			return true
		}
	}

	return false
}

func isSuccessfulHTTPStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode < http.StatusMultipleChoices
}

// selectTransport은 IRIS_TRANSPORT 또는 WithTransport로 클라이언트 transport를 고른다:
// h3는 https가 필요하며 closer를 가진 HTTP/3 transport를 반환한다. Http1은
// HTTP/1.1만 활성화한 net/http transport를 쓴다.
// 기본으로 해석되는 모드는 h3다.
func selectTransport(baseURL string, opts clientOptions) (http.RoundTripper, io.Closer, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, nil, fmt.Errorf("parse IRIS_BASE_URL: %w", err)
	}

	transport := resolveTransport(opts.Transport)
	switch transport {
	case transportHTTP1:
		return newHTTP1Transport(opts), nil, nil //nolint:nilnil // net/http transport는 닫을 closer가 없다.
	case transportH3:
		if parsed.Scheme != "https" {
			return nil, nil, fmt.Errorf("IRIS_TRANSPORT=h3 requires https IRIS_BASE_URL, got %s", parsed.Scheme)
		}

		caFile := resolveH3CACertFile(opts)
		if interval := resolveH3CAReloadInterval(opts); caFile != "" && interval > 0 {
			// CA 파일을 한 번만 읽어 초기 transport와 reloader의 기준 해시를 같은 바이트에서 만든다.
			// selectTransport의 초기 read와 reloader의 hash 시드 사이에 CA가 회전하면 swap이 누락되는 TOCTOU를 방지한다.
			// #nosec G304 -- CA 인증서 경로는 운영자 소유 설정이며 사용자 입력이 아니다.
			pemBytes, rerr := os.ReadFile(caFile)
			if rerr != nil {
				return nil, nil, fmt.Errorf("read IRIS_H3_CA_CERT_FILE: %w", rerr)
			}

			rt, err := newHTTP3TransportFromCA(opts, true, pemBytes)
			if err != nil {
				return nil, nil, fmt.Errorf("build h3 transport: %w", err)
			}

			reloader := newReloadingH3Transport(rt, opts, caFile, interval, pemBytes)

			return reloader, reloader, nil
		}

		rt, err := newHTTP3Transport(opts)
		if err != nil {
			return nil, nil, fmt.Errorf("build h3 transport: %w", err)
		}

		return rt, rt, nil
	case "":
		return nil, nil, errors.New("IRIS_TRANSPORT is required")
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

func newHTTP1Transport(opts clientOptions) *http.Transport {
	dialer := &net.Dialer{
		Timeout:   opts.DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		Protocols:             protocols,
		MaxIdleConns:          opts.MaxIdleConns,
		MaxIdleConnsPerHost:   opts.MaxIdleConnsPerHost,
		MaxConnsPerHost:       opts.MaxConnsPerHost,
		IdleConnTimeout:       opts.IdleConnTimeout,
		TLSHandshakeTimeout:   opts.TLSHandshakeTimeout,
		ResponseHeaderTimeout: opts.ResponseHeaderTimeout,
	}
}
