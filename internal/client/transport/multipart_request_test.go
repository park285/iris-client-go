package transport

import (
	jsonv2 "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendImageUsesKnownContentLengthWithoutChunkedTransfer(t *testing.T) {
	t.Parallel()

	var (
		gotContentLength    int64
		gotTransferEncoding []string
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentLength = r.ContentLength
		gotTransferEncoding = append([]string(nil), r.TransferEncoding...)

		if err := jsonv2.MarshalWrite(w, ReplyAcceptedResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "reply-image-1",
			Room:      testRoomA,
			Type:      msgTypeImage,
		}); err != nil {
			t.Fatalf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "bot-token", WithTransport(transportHTTP1))
	if _, err := client.SendImage(t.Context(), testRoomA, []byte("\x89PNG\r\n\x1a\npayload")); err != nil {
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

	client := NewAPIClient("http://127.0.0.1:1", "bot-token", WithTransport(transportHTTP1))
	if _, err := client.SendImage(t.Context(), testRoomA, nil); err == nil {
		t.Fatal("SendImage(nil) error = nil, want validation error")
	}
}
