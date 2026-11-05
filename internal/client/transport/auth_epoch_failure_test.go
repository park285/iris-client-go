package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"testing/iotest"
)

func TestAuthEpochRefreshFailureStopsOuterReplyRetry(t *testing.T) {
	for _, tc := range []struct {
		name    string
		status  int
		body    string
		failure error
	}{
		{name: "server error", status: http.StatusServiceUnavailable},
		{name: "generic unauthorized", status: http.StatusUnauthorized},
		{name: "malformed epoch", status: http.StatusOK, body: `{}`},
		{name: "transport failure", failure: io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var discoveries, protected int
			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == authEpochPath {
					discoveries++
					if discoveries == 2 {
						if tc.failure != nil {
							return nil, tc.failure
						}
						return epochResponse(tc.status, tc.body), nil
					}
					return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
				}
				protected++
				readEpochRequestBody(t, req)
				if protected == 1 {
					return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
				}
				return epochResponse(http.StatusOK, `{}`), nil
			}), WithReplyRetry(3))

			_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("refresh-failure"))
			if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) {
				t.Fatalf("refresh failure must remain not-sent: %v", err)
			}
			if discoveries != 2 || protected != 1 {
				t.Fatalf("failed refresh must end this logical call: discoveries=%d protected=%d", discoveries, protected)
			}
		})
	}
}

func TestAuthEpochBootstrapPreservesCallerRetryClassification(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		body        string
		failure     error
		bodyFailure bool
		cancelled   bool
		retryable   bool
	}{
		{name: "server error", status: http.StatusServiceUnavailable, retryable: true},
		{name: "rate limited", status: http.StatusTooManyRequests, retryable: true},
		{name: "transport failure", failure: io.ErrUnexpectedEOF, retryable: true},
		{name: "guard DNS failure", failure: fmt.Errorf("%w: %w", ErrNotSent, &net.DNSError{Err: "temporary resolver failure", Name: "iris.test", IsTemporary: true}), retryable: true},
		{name: "body read failure", status: http.StatusOK, bodyFailure: true, retryable: true},
		{name: "deadline", failure: context.DeadlineExceeded, retryable: true},
		{name: "cancelled before discovery", cancelled: true, retryable: true},
		{name: "egress denied", failure: ErrH3EgressDenied},
		{name: "probe rejected", failure: fmt.Errorf("%w: %w", ErrNotSent, errHTTP1ProbeOnly)},
		{name: "unauthorized", status: http.StatusUnauthorized},
		{name: "unsupported server", status: http.StatusNotFound},
		{name: "malformed epoch", status: http.StatusOK, body: `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var discoveries, protected int
			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != authEpochPath {
					protected++
				}
				discoveries++
				if tc.failure != nil {
					return nil, tc.failure
				}
				response := epochResponse(tc.status, tc.body)
				if tc.bodyFailure {
					response.Body = io.NopCloser(iotest.ErrReader(io.ErrUnexpectedEOF))
				}
				return response, nil
			}), WithReplyRetry(3))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			_, err := client.SendMessageAccepted(ctx, testRoom, "hello", WithClientRequestID("bootstrap-classification"))
			if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) || errors.Is(err, ErrRetryable) != tc.retryable {
				t.Fatalf("bootstrap classification: notSent=%t unknown=%t retryable=%t error=%v", errors.Is(err, ErrNotSent), errors.Is(err, ErrTransport), errors.Is(err, ErrRetryable), err)
			}
			wantDiscoveries := 1
			if tc.cancelled {
				wantDiscoveries = 0
			}
			if protected != 0 || discoveries != wantDiscoveries {
				t.Fatalf("bootstrap failure must not send or retry: discoveries=%d protected=%d", discoveries, protected)
			}
		})
	}
}

func TestAuthEpochRepeatedMismatchPreservesCallerRetryClassification(t *testing.T) {
	var discoveries, protected int
	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			discoveries++
			return epochResponse(http.StatusOK, epochDocument(fmt.Sprintf("%032x", discoveries))), nil
		}
		protected++
		readEpochRequestBody(t, req)
		// 각 조회 직후 인증 창이 다시 교체되는 두 차례 재시작을 모사한다.
		return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
	}), WithReplyRetry(3))
	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("repeated-epoch-change"))
	if discoveries != 2 || protected != 2 {
		t.Fatalf("epoch refresh budget exceeded: discoveries=%d protected=%d", discoveries, protected)
	}
	if !errors.Is(err, ErrNotSent) || !errors.Is(err, ErrRetryable) || errors.Is(err, ErrTransport) || errors.Is(err, ErrPermanent) || errors.Is(err, ErrAuthFailed) {
		t.Fatalf("repeated epoch change must remain retryable without an unknown outcome: %v", err)
	}
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != http.StatusUnauthorized || HTTPErrorCode(err) != authEpochMismatchCode {
		t.Fatalf("epoch rejection lost its HTTP status or verified code: %v", err)
	}
}

func TestAuthEpochDiscoveryCannotResolvePriorReplyOutcome(t *testing.T) {
	var discoveries, protected int
	network := errors.New("synthetic lost admission response")
	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			discoveries++
			if discoveries == 2 {
				return epochResponse(http.StatusConflict, `{"code":"CLIENT_REQUEST_ID_FAILED"}`), nil
			}
			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}
		protected++
		readEpochRequestBody(t, req)
		if protected == 1 {
			return nil, network
		}
		return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
	}), WithReplyRetry(3))
	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("bootstrap-cannot-resolve"))
	assertUnknownReplyOutcome(t, err, network)
	if errors.Is(err, ErrNotSent) || errors.Is(err, ErrPermanent) || HTTPErrorCode(err) != "" {
		t.Fatalf("public discovery cannot establish prior reply outcome: %v", err)
	}
	if discoveries != 2 || protected != 2 {
		t.Fatalf("failed discovery must stop this logical call: discoveries=%d protected=%d", discoveries, protected)
	}
}
