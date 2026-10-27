package transport

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// Ping은 서명 없는 GET /ready를 SDK 소유 http1 loopback transport로 한 번 보낸다.
func TestPingSendsSingleUnsignedReadyProbe(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)

		if r.Method != http.MethodGet || r.URL.Path != PathReady {
			t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, PathReady)
		}

		for _, header := range []string{HeaderIrisSignature, HeaderIrisNonce, HeaderIrisTimestamp, HeaderIrisBodySHA256} {
			if r.Header.Get(header) != "" {
				t.Errorf("header %s present, want unsigned probe", header)
			}
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(WithBaseURL(server.URL), WithBotToken("bot-token"), WithTransport(TransportHTTP1))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

// 실패 원인은 error로 돌려주고 다시 시도하지 않는다. 4xx는 설정 오류(ErrPermanent), 5xx는 일시 장애
// (ErrRetryable)로 호출자가 구분한다.
func TestPingReturnsStatusErrorWithoutRetry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        int
		wantPermanent bool
		wantRetryable bool
	}{
		{name: "not found", status: http.StatusNotFound, wantPermanent: true},
		{name: "unauthorized", status: http.StatusUnauthorized, wantPermanent: true},
		{name: "unavailable", status: http.StatusServiceUnavailable, wantRetryable: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))

			err := client.Ping(t.Context())

			httpErr, ok := errors.AsType[*HTTPError](err)
			if !ok || httpErr.StatusCode != tt.status {
				t.Fatalf("Ping() error = %v, want *HTTPError status %d", err, tt.status)
			}

			if errors.Is(err, ErrPermanent) != tt.wantPermanent || errors.Is(err, ErrRetryable) != tt.wantRetryable {
				t.Fatalf("Ping() error = %v, permanent/retryable = %v/%v, want %v/%v", err,
					errors.Is(err, ErrPermanent), errors.Is(err, ErrRetryable), tt.wantPermanent, tt.wantRetryable)
			}

			if got := calls.Load(); got != 1 {
				t.Fatalf("calls = %d, want 1", got)
			}
		})
	}
}

func TestPingTransportFailureIsTransportError(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)

		return nil, errors.New("connection refused")
	})
	client := mustNewAPIClient("http://localhost", "", withRoundTripper(rt))

	err := client.Ping(t.Context())

	got, ok := errors.AsType[*TransportError](err)
	if !ok || got.Op != "ping" || got.URL != "http://localhost"+PathReady {
		t.Fatalf("Ping() error = %v, want TransportError{Op:ping, URL:http://localhost%s}", err, PathReady)
	}

	if !errors.Is(err, ErrTransport) || !errors.Is(err, ErrRetryable) {
		t.Fatalf("Ping() error = %v, want ErrTransport and ErrRetryable (GET is idempotent)", err)
	}

	if calls.Load() != 1 {
		t.Fatalf("calls = %d, want 1", calls.Load())
	}
}
