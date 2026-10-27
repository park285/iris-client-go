package query

import (
	jsonv2 "encoding/json/v2"
	"testing"
)

func TestMemberNicknameUpdatedEventJSON(t *testing.T) {
	raw := `{
		"type": "member_nickname_updated",
		"sourceLogId": 1000000000001,
		"rawSourceLogId": 1,
		"sourceGenerationId": 1,
		"sourceAccountId": "123456789",
		"chatLogId": "165595",
		"chatId": 18479861808840308,
		"userId": 8691114094424718810,
		"previousDisplayName": "카푸치노",
		"currentDisplayName": "카푸카푸",
		"createdAtMs": 1778226335000
	}`

	var got MemberNicknameUpdatedEvent

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != EventTypeMemberNicknameUpdated {
		t.Fatalf("Type = %q, want member_nickname_updated", got.Type)
	}

	if got.ChatLogID == nil || *got.ChatLogID != "165595" {
		t.Fatalf("ChatLogID = %v, want 165595", got.ChatLogID)
	}

	if got.SourceLogID != 1000000000001 {
		t.Fatalf("SourceLogID = %d, want 1000000000001", got.SourceLogID)
	}

	if got.RawSourceLogID == nil || *got.RawSourceLogID != 1 {
		t.Fatalf("RawSourceLogID = %v, want 1", got.RawSourceLogID)
	}

	if got.SourceGenerationID == nil || *got.SourceGenerationID != 1 {
		t.Fatalf("SourceGenerationID = %v, want 1", got.SourceGenerationID)
	}

	if got.SourceAccountID != "123456789" {
		t.Fatalf("SourceAccountID = %q, want 123456789", got.SourceAccountID)
	}

	if got.ChatID != 18479861808840308 {
		t.Fatalf("ChatID = %d, want 18479861808840308", got.ChatID)
	}

	if got.UserID != 8691114094424718810 {
		t.Fatalf("UserID = %d, want 8691114094424718810", got.UserID)
	}

	if got.PreviousDisplayName != "카푸치노" {
		t.Fatalf("PreviousDisplayName = %q, want 카푸치노", got.PreviousDisplayName)
	}

	if got.CurrentDisplayName != "카푸카푸" {
		t.Fatalf("CurrentDisplayName = %q, want 카푸카푸", got.CurrentDisplayName)
	}

	if got.CreatedAtMs != 1778226335000 {
		t.Fatalf("CreatedAtMs = %d, want 1778226335000", got.CreatedAtMs)
	}
}
