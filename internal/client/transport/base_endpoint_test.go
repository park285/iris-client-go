package transport

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/signing"
)

func TestNewAPIClientRejectsInvalidBaseEndpointBeforeEveryTransportBranch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		branch string
	}{
		{name: "standard"},
		{name: "custom http client", branch: "client"},
		{name: "custom round tripper", branch: "round_tripper"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				options            []ClientOption
				roundTripperCalled atomic.Bool
			)

			switch test.branch {
			case "client":
				options = []ClientOption{WithHTTPClient(&http.Client{})}
			case "round_tripper":
				options = []ClientOption{WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
					roundTripperCalled.Store(true)

					return nil, errors.New("unexpected custom round tripper call")
				}))}
			}

			client := NewAPIClient("https://user:secret@iris.example?unsafe=1", "token", options...)
			if client.InitError() == nil {
				t.Fatal("InitError() = nil, want invalid base endpoint rejection")
			}

			if err := client.SendMessage(t.Context(), testRoomA, "hello"); err == nil {
				t.Fatal("SendMessage() error = nil, want initialization failure")
			}

			if roundTripperCalled.Load() {
				t.Fatal("custom round tripper called for invalid endpoint")
			}
		})
	}
}

func TestAPIClientPreservesDeploymentPrefixAndSignsOnlyRoute(t *testing.T) {
	t.Parallel()

	const secret = "base-prefix-signing-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if got, want := req.URL.Path, "/tenant/iris/reply"; got != want {
			t.Fatalf("request path = %q, want %q", got, want)
		}

		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		wantSignature, err := signing.SignIrisCanonicalWithSigner(
			signing.NewHMACSigner(secret),
			req.Method,
			PathReply,
			req.Header.Get(HeaderIrisTimestamp),
			req.Header.Get(HeaderIrisNonce),
			signing.SHA256HexBytes(body),
		)
		if err != nil {
			t.Fatalf("sign canonical route: %v", err)
		}

		if got := req.Header.Get(HeaderIrisSignature); got != wantSignature {
			t.Fatalf("signature = %q, want route-only signature %q", got, wantSignature)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewAPIClient(server.URL+"/tenant/iris///", "token",
		WithTransport(transportHTTP1),
		WithHMACSecret(secret),
	)
	if err := client.SendMessage(t.Context(), testRoomA, "hello"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
}
