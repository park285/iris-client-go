package client

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestMemberEventJSON(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		wantLinkID *int64
	}{
		{
			name:       "with linkId",
			raw:        `{"type":"member","event":"join","chatId":100,"linkId":200,"userId":1001,"nickname":"alice","estimated":false,"timestamp":1711612800000}`,
			wantLinkID: int64Ptr(200),
		},
		{
			name:       "null linkId",
			raw:        `{"type":"member","event":"leave","chatId":100,"userId":1001,"estimated":true,"timestamp":1711612800000}`,
			wantLinkID: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got MemberEvent
			if err := jsonx.Unmarshal([]byte(tt.raw), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if got.Type != "member" {
				t.Fatalf("Type = %q, want member", got.Type)
			}
			if got.ChatID != 100 {
				t.Fatalf("ChatID = %d, want 100", got.ChatID)
			}
			if got.UserID != 1001 {
				t.Fatalf("UserID = %d, want 1001", got.UserID)
			}
			if got.Timestamp != 1711612800000 {
				t.Fatalf("Timestamp = %d, want 1711612800000", got.Timestamp)
			}

			if tt.wantLinkID == nil {
				if got.LinkID != nil {
					t.Fatalf("LinkID = %v, want nil", got.LinkID)
				}
			} else {
				if got.LinkID == nil || *got.LinkID != *tt.wantLinkID {
					t.Fatalf("LinkID = %v, want %d", got.LinkID, *tt.wantLinkID)
				}
			}
		})
	}
}

func TestNicknameChangeEventJSON(t *testing.T) {
	raw := `{
		"type": "nickname_change",
		"chatId": 100,
		"linkId": 200,
		"userId": 1001,
		"oldNickname": "alice",
		"newNickname": "alice2",
		"timestamp": 1711612800000
	}`

	var got NicknameChangeEvent
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != "nickname_change" {
		t.Fatalf("Type = %q, want nickname_change", got.Type)
	}
	if got.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", got.ChatID)
	}
	if got.OldNickname == nil || *got.OldNickname != "alice" {
		t.Fatalf("OldNickname = %v, want alice", got.OldNickname)
	}
	if got.NewNickname == nil || *got.NewNickname != "alice2" {
		t.Fatalf("NewNickname = %v, want alice2", got.NewNickname)
	}
}

func TestNicknameChangeEventNullNicknamesJSON(t *testing.T) {
	raw := `{
		"type": "nickname_change",
		"chatId": 100,
		"userId": 1001,
		"timestamp": 1711612800000
	}`

	var got NicknameChangeEvent
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.OldNickname != nil {
		t.Fatalf("OldNickname = %v, want nil", got.OldNickname)
	}
	if got.NewNickname != nil {
		t.Fatalf("NewNickname = %v, want nil", got.NewNickname)
	}
	if got.LinkID != nil {
		t.Fatalf("LinkID = %v, want nil", got.LinkID)
	}
}

func TestRoleChangeEventJSON(t *testing.T) {
	raw := `{
		"type": "role_change",
		"chatId": 100,
		"userId": 1001,
		"oldRole": "member",
		"newRole": "owner",
		"timestamp": 1711612800000
	}`

	var got RoleChangeEvent
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != "role_change" {
		t.Fatalf("Type = %q, want role_change", got.Type)
	}
	if got.OldRole != "member" {
		t.Fatalf("OldRole = %q, want member", got.OldRole)
	}
	if got.NewRole != "owner" {
		t.Fatalf("NewRole = %q, want owner", got.NewRole)
	}
	if got.LinkID != nil {
		t.Fatalf("LinkID = %v, want nil", got.LinkID)
	}
}

func TestProfileChangeEventJSON(t *testing.T) {
	raw := `{
		"type": "profile_change",
		"chatId": 100,
		"linkId": 200,
		"userId": 1001,
		"timestamp": 1711612800000,
		"nickname": "alice",
		"oldProfileImageUrl": "profile-a",
		"newProfileImageUrl": "profile-b"
	}`

	var got ProfileChangeEvent
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Type != "profile_change" {
		t.Fatalf("Type = %q, want profile_change", got.Type)
	}
	if got.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", got.ChatID)
	}
	if got.LinkID == nil || *got.LinkID != 200 {
		t.Fatalf("LinkID = %v, want 200", got.LinkID)
	}
	if got.UserID != 1001 {
		t.Fatalf("UserID = %d, want 1001", got.UserID)
	}
	if got.Timestamp != 1711612800000 {
		t.Fatalf("Timestamp = %d, want 1711612800000", got.Timestamp)
	}
	if got.Nickname == nil || *got.Nickname != "alice" {
		t.Fatalf("Nickname = %v, want alice", got.Nickname)
	}
	if got.OldProfileImageURL == nil || *got.OldProfileImageURL != "profile-a" {
		t.Fatalf("OldProfileImageURL = %v, want profile-a", got.OldProfileImageURL)
	}
	if got.NewProfileImageURL == nil || *got.NewProfileImageURL != "profile-b" {
		t.Fatalf("NewProfileImageURL = %v, want profile-b", got.NewProfileImageURL)
	}
}

func TestProfileChangeEventOptionalFieldsJSON(t *testing.T) {
	raw := `{
		"type": "profile_change",
		"chatId": 100,
		"userId": 1001,
		"timestamp": 1711612800000
	}`

	var got ProfileChangeEvent
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.LinkID != nil {
		t.Fatalf("LinkID = %v, want nil", got.LinkID)
	}
	if got.Nickname != nil {
		t.Fatalf("Nickname = %v, want nil", got.Nickname)
	}
	if got.OldProfileImageURL != nil {
		t.Fatalf("OldProfileImageURL = %v, want nil", got.OldProfileImageURL)
	}
	if got.NewProfileImageURL != nil {
		t.Fatalf("NewProfileImageURL = %v, want nil", got.NewProfileImageURL)
	}
}

func int64Ptr(v int64) *int64 { return &v }
