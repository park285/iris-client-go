package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendImageUsesKnownContentLengthWithoutChunkedTransfer(t *testing.T) {
	t.Parallel()

	var gotContentLength int64
	var gotTransferEncoding []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength
		gotTransferEncoding = append([]string(nil), r.TransferEncoding...)

		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "reply-image-1",
			Room:      "room-a",
			Type:      msgTypeImage,
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "bot-token", WithTransport("http1"))
	if _, err := client.SendImage(t.Context(), "room-a", []byte("\x89PNG\r\n\x1a\npayload")); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if gotContentLength <= 0 {
		t.Fatalf("ContentLength = %d, want known positive multipart length", gotContentLength)
	}
	if len(gotTransferEncoding) != 0 {
		t.Fatalf("TransferEncoding = %v, want no chunked transfer when ContentLength is known", gotTransferEncoding)
	}
}

func TestSendImageRejectsEmptyPayloadBeforeNetwork(t *testing.T) {
	t.Parallel()

	client := NewH2CClient("http://127.0.0.1:1", "bot-token", WithTransport("http1"))
	if _, err := client.SendImage(t.Context(), "room-a", nil); err == nil {
		t.Fatal("SendImage(nil) error = nil, want validation error")
	}
}
