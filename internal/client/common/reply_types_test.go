package common

import (
	jsonv2 "encoding/json/v2"
	"testing"
)

func TestReplyAcceptedResponseJSON(t *testing.T) {
	raw := `{
		"success": true,
		"delivery": "async",
		"requestId": "req-001",
		"room": "room-a",
		"type": "text"
	}`

	var got ReplyAcceptedResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Success {
		t.Fatal("Success = false, want true")
	}

	if got.Delivery != "async" {
		t.Fatalf("Delivery = %q, want async", got.Delivery)
	}

	if got.RequestID != "req-001" {
		t.Fatalf("RequestID = %q, want req-001", got.RequestID)
	}

	if got.Room != testRoomA {
		t.Fatalf("Room = %q, want room-a", got.Room)
	}

	if got.Type != testReplyTypeText {
		t.Fatalf("Type = %q, want text", got.Type)
	}
}

func TestReplyStatusSnapshotJSON(t *testing.T) {
	// Iris reply lifecycle wire 값(ReplyLifecycleState, snake_case)을 그대로 보존해야 한다.
	tests := []struct {
		name       string
		raw        string
		wantState  string
		wantDetail *string
	}{
		{
			name:       "outcome unknown with detail",
			raw:        `{"requestId":"req-001","state":"outcome_unknown","updatedAtEpochMs":1711612800000,"detail":"external send outcome is unknown; automatic replay disabled"}`,
			wantState:  "outcome_unknown",
			wantDetail: new("external send outcome is unknown; automatic replay disabled"),
		},
		{
			name:       "handoff completed without detail",
			raw:        `{"requestId":"req-002","state":"handoff_completed","updatedAtEpochMs":1711612800000}`,
			wantState:  "handoff_completed",
			wantDetail: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ReplyStatusSnapshot

			if err := jsonv2.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if got.RequestID == "" {
				t.Fatal("RequestID is empty")
			}

			if got.State != tt.wantState {
				t.Fatalf("State = %q, want %q", got.State, tt.wantState)
			}

			if got.UpdatedAtEpochMs == 0 {
				t.Fatal("UpdatedAtEpochMs = 0")
			}

			assertReplyStatusDetail(t, got.Detail, tt.wantDetail)
		})
	}
}

func assertReplyStatusDetail(t *testing.T, got, want *string) {
	t.Helper()

	if want == nil {
		if got != nil {
			t.Fatalf("Detail = %q, want nil", *got)
		}

		return
	}

	if got == nil {
		t.Fatal("Detail = nil, want non-nil")
	}

	if *got != *want {
		t.Fatalf("Detail = %q, want %q", *got, *want)
	}
}
