package transport

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestH2CClientSendImageUsesKnownLengthMultipartRequest(t *testing.T) {
	t.Parallel()

	var sawKnownLength bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKnownLength = r.ContentLength > 0 && len(r.TransferEncoding) == 0

		metadata, images := readMultipartReplyRequest(t, r)
		if metadata.Type != "image" || metadata.Room != "room" {
			t.Fatalf("metadata = %+v, want image room", metadata)
		}
		if len(images) != 1 || !bytes.Equal(images[0], []byte{0x89, 'P', 'N', 'G'}) {
			t.Fatalf("images = %v, want one png payload", images)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "stream",
			Room:      "room",
			Type:      "image",
		}); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "", WithHTTPClient(server.Client()))
	if _, err := c.SendImage(t.Context(), "room", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if !sawKnownLength {
		t.Fatal("request did not arrive with known content length")
	}
}
