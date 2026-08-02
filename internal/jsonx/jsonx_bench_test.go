package jsonx

import (
	"bytes"
	"strings"
	"testing"
)

type benchReplyRequest struct {
	ClientRequestID *string `json:"clientRequestId,omitempty"`
	Type            string  `json:"type"`
	Room            string  `json:"room"`
	Data            string  `json:"data"`
	ThreadID        *string `json:"threadId,omitempty"`
}

type benchWebhookRequest struct {
	Msg      string `json:"msg"`
	Room     string `json:"room"`
	Sender   string `json:"sender"`
	JSON     string `json:"json"`
	LogID    int64  `json:"logId"`
	ChatID   int64  `json:"chatId"`
	UserID   int64  `json:"userId"`
	CreatedA int64  `json:"createdAt"`
}

func benchReplyPayload() benchReplyRequest {
	id := "bench:room-42:2026-08-02"
	thread := "998877"
	return benchReplyRequest{
		ClientRequestID: &id,
		Type:            "text",
		Room:            "room-42",
		Data:            "안녕하세요 " + strings.Repeat("hotpath ", 16),
		ThreadID:        &thread,
	}
}

func benchWebhookBody() []byte {
	body, err := Marshal(benchWebhookRequest{
		Msg:      "안녕하세요 " + strings.Repeat("payload ", 16),
		Room:     "room-42",
		Sender:   "사용자",
		JSON:     `{"attachment":"","enc":31}`,
		LogID:    123456789012345,
		ChatID:   987654321,
		UserID:   1122334455,
		CreatedA: 1785000000000,
	})
	if err != nil {
		panic(err)
	}
	return body
}

func BenchmarkMarshalReplyRequest(b *testing.B) {
	payload := benchReplyPayload()

	b.ReportAllocs()
	for b.Loop() {
		if _, err := Marshal(payload); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalWebhookRequest(b *testing.B) {
	body := benchWebhookBody()

	b.ReportAllocs()
	for b.Loop() {
		var got benchWebhookRequest
		if err := Unmarshal(body, &got); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkDecoderWebhookRequest(b *testing.B) {
	body := benchWebhookBody()
	reader := bytes.NewReader(body)

	b.ReportAllocs()
	for b.Loop() {
		reader.Reset(body)
		var got benchWebhookRequest
		if err := NewDecoder(reader).Decode(&got); err != nil {
			b.Fatal(err)
		}
	}
}
