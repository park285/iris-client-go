package rebind

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
	"github.com/park285/iris-client-go/v2/internal/testsupport"
)

func TestRebindingClientSendFileUsesCurrentClient(t *testing.T) {
	t.Parallel()

	payload := []byte("payload")
	gotPayload := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payloadBytes, ok := readMultipartFilePayload(t, w, r)
		if !ok {
			return
		}

		gotPayload <- payloadBytes

		w.Header().Set("Content-Type", "application/json")

		testsupport.WriteJSON(t, w, transport.ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "sent",
			RequestID: "rebound-file",
			Room:      "room",
			Type:      "file",
		})
	}))

	defer server.Close()

	client := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return server.URL, nil },
		BotToken:       testBotToken,
		ClientOptions:  []transport.ClientOption{transport.WithHTTPClient(server.Client()), transport.WithTransport("http1")},
	})

	defer testsupport.CloseNow(t, "client.Close", client.Close)

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

func readMultipartFilePayload(t *testing.T, w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	t.Helper()

	reader, err := r.MultipartReader()
	if err != nil {
		t.Errorf("MultipartReader() error = %v", err)
		http.Error(w, "bad multipart", http.StatusBadRequest)

		return nil, false
	}

	if _, metadataErr := reader.NextPart(); metadataErr != nil {
		t.Errorf("NextPart(metadata) error = %v", metadataErr)
		http.Error(w, "missing metadata", http.StatusBadRequest)

		return nil, false
	}

	filePart, err := reader.NextPart()
	if err != nil {
		t.Errorf("NextPart(file) error = %v", err)
		http.Error(w, "missing file", http.StatusBadRequest)

		return nil, false
	}

	payloadBytes, err := io.ReadAll(filePart)
	if err != nil {
		t.Errorf("ReadAll(file) error = %v", err)
		http.Error(w, "bad file", http.StatusBadRequest)

		return nil, false
	}

	return payloadBytes, true
}
