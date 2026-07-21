package rebind

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/park285/iris-client-go/internal/client/transport"
)

func TestRebindingClientSendFileUsesCurrentClient(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	gotPayload := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reader, err := r.MultipartReader()
		if err != nil {
			t.Errorf("MultipartReader() error = %v", err)
			http.Error(w, "bad multipart", http.StatusBadRequest)
			return
		}
		_, _ = reader.NextPart()
		filePart, err := reader.NextPart()
		if err != nil {
			t.Errorf("NextPart(file) error = %v", err)
			http.Error(w, "missing file", http.StatusBadRequest)
			return
		}
		payloadBytes, err := io.ReadAll(filePart)
		if err != nil {
			t.Errorf("ReadAll(file) error = %v", err)
			http.Error(w, "bad file", http.StatusBadRequest)
			return
		}
		gotPayload <- payloadBytes
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "sent",
			RequestID: "rebound-file",
			Room:      "room",
			Type:      "file",
		}); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	client := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return server.URL, nil },
		BotToken:       "bot-token",
		ClientOptions:  []ClientOption{WithHTTPClient(server.Client()), WithTransport("http1")},
	})
	defer func() { _ = client.Close() }()

	accepted, err := client.SendFile(
		t.Context(),
		"room",
		transport.NewReplyFileBytes("payload.bin", "application/octet-stream", payload),
	)
	if err != nil {
		t.Fatalf("SendFile() error = %v", err)
	}
	if accepted == nil || accepted.Type != "file" {
		t.Fatalf("accepted = %+v, want file response", accepted)
	}
	receivedPayload := <-gotPayload
	if !bytes.Equal(receivedPayload, payload) {
		t.Fatalf("payload = %q, want %q", receivedPayload, payload)
	}
}
