package transport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDoSignedRejectsNon2xxStatus(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	c := mustNewAPIClient(server.URL, "token", WithHTTPClient(server.Client()))

	resp, err := c.doSigned(t.Context(), http.MethodGet, PathRooms)
	if err == nil {
		if resp != nil {
			_ = resp.Body.Close()
		}

		t.Fatal("doSigned() status 304 error = nil, want non-2xx failure")
	}
}
