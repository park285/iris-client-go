package sse

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
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
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
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

func TestSSERoomEventBodyJSON(t *testing.T) {
	raw := `{
		"roomEventId": 165595,
		"chatId": 18479861808840308,
		"eventType": "member_nickname_updated",
		"userId": 8691114094424718810,
		"payload": {"type": "member_nickname_updated", "sourceLogId": 165595}
	}`

	var got SSERoomEventBody
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.RoomEventID != 165595 {
		t.Fatalf("RoomEventID = %d, want 165595", got.RoomEventID)
	}
	if got.ChatID != 18479861808840308 {
		t.Fatalf("ChatID = %d, want 18479861808840308", got.ChatID)
	}
	if got.EventType != EventTypeMemberNicknameUpdated {
		t.Fatalf("EventType = %q, want member_nickname_updated", got.EventType)
	}
	if got.UserID != 8691114094424718810 {
		t.Fatalf("UserID = %d, want 8691114094424718810", got.UserID)
	}

	var payload MemberNicknameUpdatedEvent
	if err := jsonx.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if payload.SourceLogID != 165595 {
		t.Fatalf("payload.SourceLogID = %d, want 165595", payload.SourceLogID)
	}
}

func TestSSEStreamStateJSON(t *testing.T) {
	raw := `{
		"cursorStatus": "stale",
		"lastEventId": 12,
		"oldestAvailableId": 40,
		"latestAvailableId": 90,
		"recommendedRecovery": "query_recent_messages"
	}`

	var got SSEStreamState
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.CursorStatus != StreamCursorStatusStale {
		t.Fatalf("CursorStatus = %q, want stale", got.CursorStatus)
	}
	if got.LastEventID != 12 {
		t.Fatalf("LastEventID = %d, want 12", got.LastEventID)
	}
	if got.OldestAvailableID == nil || *got.OldestAvailableID != 40 {
		t.Fatalf("OldestAvailableID = %v, want 40", got.OldestAvailableID)
	}
	if got.LatestAvailableID == nil || *got.LatestAvailableID != 90 {
		t.Fatalf("LatestAvailableID = %v, want 90", got.LatestAvailableID)
	}
	if got.RecommendedRecovery != StreamRecoveryQueryRecentMessages {
		t.Fatalf("RecommendedRecovery = %q, want query_recent_messages", got.RecommendedRecovery)
	}
}

func TestSSEStreamStateNullAvailableIDsJSON(t *testing.T) {
	raw := `{
		"cursorStatus": "future",
		"lastEventId": 99,
		"oldestAvailableId": null,
		"latestAvailableId": null,
		"recommendedRecovery": "query_recent_messages"
	}`

	var got SSEStreamState
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.CursorStatus != StreamCursorStatusFuture {
		t.Fatalf("CursorStatus = %q, want future", got.CursorStatus)
	}
	if got.OldestAvailableID != nil || got.LatestAvailableID != nil {
		t.Fatalf("available ids = %v %v, want nil nil", got.OldestAvailableID, got.LatestAvailableID)
	}
}
