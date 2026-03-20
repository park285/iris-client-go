package webhook

import (
	"encoding/json"
	"reflect"
	"testing"
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

	gotJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	if string(gotJSON) != wantJSON {
		t.Fatalf("json.Marshal() = %s, want %s", gotJSON, wantJSON)
	}

	var got T
	if err := json.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, wantRound, label)
}

func assertJSONUnmarshal[T any](t *testing.T, input string, want T, label string) {
	t.Helper()

	var got T
	if err := json.Unmarshal([]byte(input), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, want, label)
}

func assertJSONEqual[T any](t *testing.T, got, want T, label string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}
