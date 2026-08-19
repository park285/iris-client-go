package rebind

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
)

func TestRebindingClientFetchMediaChunkForwardsToCurrentClient(t *testing.T) {
	var got transport.MediaChunkRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != transport.PathMediaChunk {
			t.Fatalf("request = %s %s, want POST %s", r.Method, r.URL.Path, transport.PathMediaChunk)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if err := json.NewEncoder(w).Encode(transport.MediaChunkResponse{
			ChunkBase64: "AA==",
			TotalLength: 1,
			MIMEType:    "image/png",
			SHA256:      strings.Repeat("a", 64),
			EOF:         true,
			MediaCount:  1,
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewRebindingClient(RebindingClientConfig{
		ResolveBaseURL: func() (string, error) { return server.URL, nil },
		BotToken:       "bot-token",
		ClientOptions:  []transport.ClientOption{transport.WithHTTPClient(server.Client()), transport.WithTransport("http1")},
	})
	defer func() { _ = client.Close() }()

	want := transport.MediaChunkRequest{
		MessageID:          "message-1",
		SourceGenerationID: 1,
		RawSourceLogID:     2,
		SourceLogID:        3,
		ChatID:             "4",
		ChatLogID:          "5",
		Type:               "2",
		MediaIndex:         0,
		Offset:             0,
		Length:             1,
	}
	response, err := client.FetchMediaChunk(t.Context(), want)
	if err != nil {
		t.Fatalf("FetchMediaChunk() error = %v", err)
	}
	if got != want {
		t.Fatalf("request body = %+v, want %+v", got, want)
	}
	if response == nil || response.ChunkBase64 != "AA==" || !response.EOF {
		t.Fatalf("response = %+v", response)
	}
}
