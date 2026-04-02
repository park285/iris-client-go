package webhook

import (
	"reflect"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestWebhookRequestJSONMarshalLegacyCompatibility(t *testing.T) {
	tt := webhookMarshalLegacyCase()
	assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "WebhookRequest")
}

func TestWebhookRequestJSONMarshalWithOptionalFields(t *testing.T) {
	tt := webhookMarshalOptionalFieldsCase()
	assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "WebhookRequest")
}

func TestWebhookRequestJSONUnmarshalLegacy(t *testing.T) {
	tests := []struct {
		name       string
		legacyJSON string
		want       WebhookRequest
	}{
		legacyWebhookUnmarshalCase("legacy payload without optional fields"),
		legacyWebhookUnmarshalCase("legacy payload remains compatible for newer shape"),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONUnmarshal(t, tt.legacyJSON, tt.want, "WebhookRequest")
		})
	}
}

func webhookMarshalLegacyCase() struct {
	name      string
	input     WebhookRequest
	wantJSON  string
	wantRound WebhookRequest
} {
	return struct {
		name      string
		input     WebhookRequest
		wantJSON  string
		wantRound WebhookRequest
	}{
		name: "omit optional fields for legacy compatibility",
		input: WebhookRequest{
			Text:   "hello",
			Room:   "room-a",
			Sender: "alice",
			UserID: "user-1",
		},
		wantJSON: `{"text":"hello","room":"room-a","sender":"alice","userId":"user-1"}`,
		wantRound: WebhookRequest{
			Text:   "hello",
			Room:   "room-a",
			Sender: "alice",
			UserID: "user-1",
		},
	}
}

func webhookMarshalOptionalFieldsCase() struct {
	name      string
	input     WebhookRequest
	wantJSON  string
	wantRound WebhookRequest
} {
	threadScope := 3

	return struct {
		name      string
		input     WebhookRequest
		wantJSON  string
		wantRound WebhookRequest
	}{
		name: "include new optional fields when set",
		input: WebhookRequest{
			Route:       "default",
			MessageID:   "msg-1",
			SourceLogID: 42,
			Text:        "hello",
			Room:        "room-a",
			Sender:      "alice",
			UserID:      "user-1",
			ChatLogID:   "chat-1",
			RoomType:    "OD",
			RoomLinkID:  "link-1",
			ThreadID:    "12345",
			ThreadScope: &threadScope,
			Type:        "1",
			Attachment:  "{\"url\":\"test\"}",
		},
		wantJSON: `{"route":"default","messageId":"msg-1","sourceLogId":42,"text":"hello","room":"room-a","sender":"alice","userId":"user-1","chatLogId":"chat-1","roomType":"OD","roomLinkId":"link-1","threadId":"12345","threadScope":3,"type":"1","attachment":"{\"url\":\"test\"}"}`,
		wantRound: WebhookRequest{
			Route:       "default",
			MessageID:   "msg-1",
			SourceLogID: 42,
			Text:        "hello",
			Room:        "room-a",
			Sender:      "alice",
			UserID:      "user-1",
			ChatLogID:   "chat-1",
			RoomType:    "OD",
			RoomLinkID:  "link-1",
			ThreadID:    "12345",
			ThreadScope: &threadScope,
			Type:        "1",
			Attachment:  "{\"url\":\"test\"}",
		},
	}
}

func legacyWebhookUnmarshalCase(name string) struct {
	name       string
	legacyJSON string
	want       WebhookRequest
} {
	return struct {
		name       string
		legacyJSON string
		want       WebhookRequest
	}{
		name:       name,
		legacyJSON: `{"text":"hello","room":"room-a","sender":"alice","userId":"user-1"}`,
		want: WebhookRequest{
			Text:   "hello",
			Room:   "room-a",
			Sender: "alice",
			UserID: "user-1",
		},
	}
}

func assertJSONRoundTrip[T any](t *testing.T, input T, wantJSON string, wantRound T, label string) {
	t.Helper()

	gotJSON, err := jsonx.Marshal(input)
	if err != nil {
		t.Fatalf("jsonx.Marshal() error = %v", err)
	}

	if string(gotJSON) != wantJSON {
		t.Fatalf("jsonx.Marshal() = %s, want %s", gotJSON, wantJSON)
	}

	var got T
	if err := jsonx.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("jsonx.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, wantRound, label)
}

func assertJSONUnmarshal[T any](t *testing.T, input string, want T, label string) {
	t.Helper()

	var got T
	if err := jsonx.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("jsonx.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, want, label)
}

func assertJSONEqual[T any](t *testing.T, got, want T, label string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}

func TestWebhookRequestIgnoresUnknownSenderRoleJSON(t *testing.T) {
	t.Run("absent field remains absent", func(t *testing.T) {
		input := `{"text":"hello","room":"room-a","sender":"alice","userId":"user-1"}`

		var got WebhookRequest
		if err := jsonx.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		out, err := jsonx.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		if strings.Contains(string(out), "senderRole") {
			t.Fatalf("marshalled output contains senderRole: %s", out)
		}
	})

	t.Run("unknown field is ignored", func(t *testing.T) {
		input := `{"text":"hello","room":"room-a","sender":"alice","userId":"user-1","senderRole":3}`

		var got WebhookRequest
		if err := jsonx.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		want := WebhookRequest{
			Text:   "hello",
			Room:   "room-a",
			Sender: "alice",
			UserID: "user-1",
		}
		assertJSONEqual(t, got, want, "WebhookRequest")

		out, err := jsonx.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		if strings.Contains(string(out), "senderRole") {
			t.Fatalf("marshalled output contains senderRole: %s", out)
		}
	})
}

func TestMessageJSONIgnoresUnknownSenderRoleJSON(t *testing.T) {
	t.Run("absent field remains absent", func(t *testing.T) {
		input := `{"user_id":"u1","message":"hi"}`

		var got MessageJSON
		if err := jsonx.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		out, err := jsonx.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		if strings.Contains(string(out), "sender_role") {
			t.Fatalf("marshalled output contains sender_role: %s", out)
		}
	})

	t.Run("unknown field is ignored", func(t *testing.T) {
		input := `{"user_id":"u1","message":"hi","sender_role":5}`

		var got MessageJSON
		if err := jsonx.Unmarshal([]byte(input), &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}

		want := MessageJSON{
			UserID:  "u1",
			Message: "hi",
		}
		assertJSONEqual(t, got, want, "MessageJSON")

		out, err := jsonx.Marshal(got)
		if err != nil {
			t.Fatalf("Marshal() error = %v", err)
		}

		if strings.Contains(string(out), "sender_role") {
			t.Fatalf("marshalled output contains sender_role: %s", out)
		}
	})
}
