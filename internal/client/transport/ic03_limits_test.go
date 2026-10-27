package transport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIC03RedirectDoesNotReplaySignedPostToDifferentHost_21233857(t *testing.T) {
	t.Parallel()

	var (
		attackerHits    int
		attackerBody    []byte
		attackerHeaders http.Header
	)

	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attackerHits++

		attackerHeaders = r.Header.Clone()

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read attacker body: %v", err)
		}

		attackerBody = body

		w.WriteHeader(http.StatusOK)
	}))

	defer attacker.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Redirect(w, &http.Request{}, attacker.URL+"/reply", http.StatusTemporaryRedirect)
	}))
	defer origin.Close()

	injected := origin.Client()
	if injected.CheckRedirect != nil {
		t.Fatal("test precondition: injected client must use the default redirect policy")
	}

	c := NewAPIClient(origin.URL, "token", WithHTTPClient(injected))
	if c.client == injected {
		t.Fatal("WithHTTPClient retained the caller-owned client pointer")
	}

	err := c.SendMessage(t.Context(), testRoom, "hello")
	if err == nil {
		t.Fatal("cross-host redirect of signed POST must fail, got nil error")
	}

	if attackerHits != 0 {
		t.Fatalf("signed POST body was replayed to attacker host %d times", attackerHits)
	}

	if len(attackerBody) != 0 {
		t.Fatalf("attacker received body %q", attackerBody)
	}

	for _, header := range []string{HeaderIrisTimestamp, HeaderIrisNonce, HeaderIrisSignature, HeaderIrisBodySHA256} {
		if attackerHeaders.Get(header) != "" {
			t.Fatalf("attacker received %s", header)
		}
	}

	if injected.CheckRedirect != nil {
		t.Fatal("WithHTTPClient mutated the caller-owned redirect policy")
	}
}

func TestIC03RedirectDoesNotReplaySignedPostAcrossHTTPSDowngrade_21233857(t *testing.T) {
	t.Parallel()

	var (
		redirectedRequests int
		redirectedBody     []byte
		redirectedHeaders  http.Header
	)

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Scheme == "https" {
			return &http.Response{
				StatusCode: http.StatusTemporaryRedirect,
				Header: http.Header{
					"Location": []string{testReplyURL},
				},
				Body: io.NopCloser(strings.NewReader("redirect")),
			}, nil
		}

		redirectedRequests++

		redirectedHeaders = req.Header.Clone()

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("read redirected body: %v", err)
		}

		redirectedBody = body

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	c := NewAPIClient("https://iris.test", "token", withRoundTripper(rt))

	err := c.SendMessage(t.Context(), testRoom, "hello")
	if err == nil {
		t.Fatal("HTTPS to HTTP redirect of signed POST must fail")
	}

	if redirectedRequests != 0 {
		t.Fatalf("downgrade target received %d requests, want 0", redirectedRequests)
	}

	if len(redirectedBody) != 0 {
		t.Fatalf("downgrade target received body %q", redirectedBody)
	}

	for _, header := range []string{HeaderIrisTimestamp, HeaderIrisNonce, HeaderIrisSignature, HeaderIrisBodySHA256} {
		if redirectedHeaders.Get(header) != "" {
			t.Fatalf("downgrade target received %s", header)
		}
	}
}

func TestIC03PingDrainBounded_0639c8cd(t *testing.T) {
	t.Parallel()

	huge := strings.Repeat("z", 16<<20)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathReady {
			w.WriteHeader(http.StatusNotFound)

			return
		}

		w.WriteHeader(http.StatusOK)

		// 상한까지만 drain한 클라이언트가 연결을 끊으면 이 Write는 reset으로 실패한다. 그게 통과 조건이다.
		if _, err := io.WriteString(w, huge); err != nil {
			return
		}
	}))

	defer srv.Close()

	c := NewAPIClient(srv.URL, "token", WithHTTPClient(srv.Client()))
	if !c.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true (200 ready must be alive even with large body)")
	}
}
