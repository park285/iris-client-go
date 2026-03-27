package client

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
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
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
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
	if got.Room != "room-a" {
		t.Fatalf("Room = %q, want room-a", got.Room)
	}
	if got.Type != "text" {
		t.Fatalf("Type = %q, want text", got.Type)
	}
}

func TestReplyStatusSnapshotJSON(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantDetail *string
	}{
		{
			name:       "with detail",
			raw:        `{"requestId":"req-001","state":"delivered","updatedAtEpochMs":1711612800000,"detail":"sent ok"}`,
			wantDetail: strPtr("sent ok"),
		},
		{
			name:       "nil detail",
			raw:        `{"requestId":"req-002","state":"pending","updatedAtEpochMs":1711612800000}`,
			wantDetail: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ReplyStatusSnapshot
			if err := jsonx.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if got.RequestID == "" {
				t.Fatal("RequestID is empty")
			}
			if got.State == "" {
				t.Fatal("State is empty")
			}
			if got.UpdatedAtEpochMs == 0 {
				t.Fatal("UpdatedAtEpochMs = 0")
			}

			if tt.wantDetail == nil {
				if got.Detail != nil {
					t.Fatalf("Detail = %q, want nil", *got.Detail)
				}
			} else {
				if got.Detail == nil {
					t.Fatal("Detail = nil, want non-nil")
				}
				if *got.Detail != *tt.wantDetail {
					t.Fatalf("Detail = %q, want %q", *got.Detail, *tt.wantDetail)
				}
			}
		})
	}
}

func strPtr(s string) *string { return &s }
