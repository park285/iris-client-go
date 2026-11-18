package transport

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestReplyStatusCannotConfirmADifferentRequest(t *testing.T) {
	for _, returnedID := range []string{"another-reply", "", " wanted-reply "} {
		t.Run(returnedID, func(t *testing.T) {
			var requests atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)

				if r.URL.Path != "/reply-status/wanted-reply" {
					t.Errorf("request path=%q, want the queried reply", r.URL.Path)
				}

				if _, err := fmt.Fprintf(w, `{"requestId":%q,"state":"handoff_completed","updatedAtEpochMs":1711600000000}`, returnedID); err != nil {
					t.Errorf("write reply status: %v", err)
				}
			}))
			defer server.Close()

			client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))
			snapshot, err := client.GetReplyStatus(t.Context(), " wanted-reply ")

			if snapshot != nil || !errors.Is(err, ErrTransport) {
				t.Fatalf("GetReplyStatus()=(%+v, %v), want an unusable observation", snapshot, err)
			}

			if errors.Is(err, ErrRetryable) {
				t.Fatalf("GetReplyStatus() error=%v, want no automatic retry of a contradictory response", err)
			}

			if !errors.Is(err, ErrPermanent) || errors.Is(err, ErrNotSent) {
				t.Fatalf("GetReplyStatus() error=%v, want a permanent observation error without a not-sent verdict", err)
			}

			if transport, ok := errors.AsType[*TransportError](err); !ok || !transport.Idempotent {
				t.Fatalf("GetReplyStatus() error=%v, want the idempotent query transport error", err)
			}

			if requests.Load() != 1 {
				t.Fatalf("status requests=%d, want one", requests.Load())
			}
		})
	}
}

func TestReplyStatusPreservesUnrecognizedStateForTheQueriedRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"requestId":"wanted-reply","state":"future_state","updatedAtEpochMs":1711600000000}`)); err != nil {
			t.Errorf("write reply status: %v", err)
		}
	}))
	defer server.Close()

	client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))
	snapshot, err := client.GetReplyStatus(t.Context(), " wanted-reply ")

	if err != nil || snapshot == nil || snapshot.RequestID != "wanted-reply" || snapshot.State != "future_state" {
		t.Fatalf("GetReplyStatus()=(%+v, %v), want the queried request with the original state", snapshot, err)
	}
}

func TestReplyStatusHTTPErrorsPreserveClassification(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusServiceUnavailable} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
			}))
			defer server.Close()

			client := mustNewAPIClient(server.URL, "", WithTransport(TransportHTTP1))
			snapshot, err := client.GetReplyStatus(t.Context(), "wanted-reply")

			if snapshot != nil || err == nil || errors.Is(err, ErrTransport) || errors.Is(err, ErrNotSent) {
				t.Fatalf("GetReplyStatus()=(%+v, %v), want an HTTP rejection without an observation mismatch", snapshot, err)
			}

			if errors.Is(err, ErrRetryable) != (status == http.StatusServiceUnavailable) || errors.Is(err, ErrPermanent) != (status < http.StatusInternalServerError) {
				t.Fatalf("GetReplyStatus() error=%v, want the original HTTP retry classification", err)
			}
		})
	}
}
