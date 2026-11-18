package transport

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

func ParseTransport(value string) (string, error) {
	switch t := strings.ToLower(strings.TrimSpace(value)); t {
	case TransportH3, TransportHTTP1:
		return t, nil
	default:
		return "", fmt.Errorf("iris: unsupported transport %q (want %s or %s)", value, TransportH3, TransportHTTP1)
	}
}

func resolveTransport(explicit string) (string, error) {
	if strings.TrimSpace(explicit) == "" {
		return TransportH3, nil
	}

	return ParseTransport(explicit)
}

func newHTTPClientWithCloser(baseURL *url.URL, transport string, opts clientOptions) (*http.Client, io.Closer, error) {
	rt, closer, err := selectTransport(baseURL, transport, opts)
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
		if len(via) > 0 && (hasIrisSigningHeaders(via[0].Header) || strings.HasSuffix(via[0].URL.Path, authEpochPath)) {
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

func hasIrisSigningHeaders(header http.Header) bool {
	for _, name := range []string{
		irishmac.HeaderIrisTimestamp,
		irishmac.HeaderIrisNonce,
		irishmac.HeaderIrisSignature,
		irishmac.HeaderIrisBodySHA256,
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

func selectTransport(baseURL *url.URL, transport string, opts clientOptions) (http.RoundTripper, io.Closer, error) {
	switch transport {
	case TransportHTTP1:
		if !isLoopbackHost(baseURL.Hostname()) {
			return nil, nil, errHTTP1ProbeOnly
		}

		return newHTTP1Transport(), nil, nil //nolint:nilnil // net/http transport는 닫을 closer가 없다.
	case TransportH3:
		if baseURL.Scheme != "https" {
			return nil, nil, fmt.Errorf("iris: h3 transport requires https base URL, got %s", baseURL.Scheme)
		}

		rt, err := newHTTP3Transport(opts)
		if err != nil {
			return nil, nil, fmt.Errorf("build h3 transport: %w", err)
		}

		return rt, rt, nil
	default:
		return nil, nil, fmt.Errorf("iris: unsupported transport %q", transport)
	}
}

var errHTTP1ProbeOnly = errors.New("iris: http1 transport is limited to loopback GET /health and /ready probes")

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}

func isHTTP1ProbeRoute(method, path string) bool {
	return method == http.MethodGet && (path == PathHealth || path == PathReady)
}

func newHTTP1Transport() *http.Transport {
	dialer := &net.Dialer{
		Timeout:   defaultHTTP1DialTimeout,
		KeepAlive: 30 * time.Second,
	}

	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)

	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		Protocols:             protocols,
		MaxIdleConns:          defaultHTTP1MaxIdleConns,
		MaxIdleConnsPerHost:   defaultHTTP1MaxIdleConnsPerHost,
		MaxConnsPerHost:       defaultMaxConnsPerHost,
		IdleConnTimeout:       defaultHTTP1IdleConnTimeout,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout,
		ResponseHeaderTimeout: defaultHTTP1HeaderTimeout,
	}
}
