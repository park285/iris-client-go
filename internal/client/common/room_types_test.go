package common

import (
	jsonv2 "encoding/json/v2"
	"testing"
)

func TestRoomListResponseJSON(t *testing.T) {
	raw := `{
		"rooms": [
			{
				"chatId": 100,
				"type": "open",
				"linkId": 200,
				"activeMembersCount": 50,
				"linkName": "test-room",
				"linkUrl": "https://open.kakao.com/o/test",
				"memberLimit": 300,
				"searchable": 1,
				"botRole": 2
			},
			{
				"chatId": 101
			}
		]
	}`

	var got RoomListResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(got.Rooms) != 2 {
		t.Fatalf("len(Rooms) = %d, want 2", len(got.Rooms))
	}

	assertRoomSummaryIdentity(t, got.Rooms[0])
	assertRoomSummaryOpenLinkFields(t, got.Rooms[0])

	// 최소 room: 필수 chatId만 존재
	r1 := got.Rooms[1]
	if r1.ChatID != 101 {
		t.Fatalf("Rooms[1].ChatID = %d, want 101", r1.ChatID)
	}

	if r1.Type != nil {
		t.Fatalf("Rooms[1].Type = %v, want nil", r1.Type)
	}
}

func assertRoomSummaryIdentity(t *testing.T, room RoomSummary) {
	t.Helper()

	if room.ChatID != 100 {
		t.Fatalf("Rooms[0].ChatID = %d, want 100", room.ChatID)
	}

	if room.Type == nil || *room.Type != "open" {
		t.Fatalf("Rooms[0].Type = %v, want open", room.Type)
	}

	if room.LinkID == nil || *room.LinkID != 200 {
		t.Fatalf("Rooms[0].LinkID = %v, want 200", room.LinkID)
	}
}

func assertRoomSummaryOpenLinkFields(t *testing.T, room RoomSummary) {
	t.Helper()

	if room.ActiveMembersCount == nil || *room.ActiveMembersCount != 50 {
		t.Fatalf("Rooms[0].ActiveMembersCount = %v, want 50", room.ActiveMembersCount)
	}

	if room.LinkName == nil || *room.LinkName != "test-room" {
		t.Fatalf("Rooms[0].LinkName = %v, want test-room", room.LinkName)
	}

	if room.LinkURL == nil || *room.LinkURL != "https://open.kakao.com/o/test" {
		t.Fatalf("Rooms[0].LinkURL = %v, unexpected", room.LinkURL)
	}

	if room.MemberLimit == nil || *room.MemberLimit != 300 {
		t.Fatalf("Rooms[0].MemberLimit = %v, want 300", room.MemberLimit)
	}

	if room.Searchable == nil || *room.Searchable != 1 {
		t.Fatalf("Rooms[0].Searchable = %v, want 1", room.Searchable)
	}

	if room.BotRole == nil || *room.BotRole != 2 {
		t.Fatalf("Rooms[0].BotRole = %v, want 2", room.BotRole)
	}
}

func TestMemberListResponseJSON(t *testing.T) {
	raw := `{
		"chatId": 100,
		"linkId": 200,
		"members": [
			{
				"userId": 1001,
				"nickname": "alice",
				"role": "owner",
				"roleCode": 1,
				"profileImageUrl": "https://img.test/a.jpg",
				"messageCount": 500,
				"lastActiveAt": 1711612800000
			},
			{
				"userId": 1002,
				"role": "member",
				"roleCode": 4,
				"messageCount": 0
			}
		],
		"totalCount": 2
	}`

	var got MemberListResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", got.ChatID)
	}

	if got.LinkID == nil || *got.LinkID != 200 {
		t.Fatalf("LinkID = %v, want 200", got.LinkID)
	}

	if got.TotalCount != 2 {
		t.Fatalf("TotalCount = %d, want 2", got.TotalCount)
	}

	if len(got.Members) != 2 {
		t.Fatalf("len(Members) = %d, want 2", len(got.Members))
	}

	m0 := got.Members[0]
	if m0.UserID != 1001 {
		t.Fatalf("Members[0].UserID = %d, want 1001", m0.UserID)
	}

	if m0.Nickname == nil || *m0.Nickname != "alice" {
		t.Fatalf("Members[0].Nickname = %v, want alice", m0.Nickname)
	}

	if m0.Role != "owner" || m0.RoleCode != 1 {
		t.Fatalf("Members[0].Role/RoleCode = %s/%d, unexpected", m0.Role, m0.RoleCode)
	}

	if m0.ProfileImageURL == nil || *m0.ProfileImageURL != "https://img.test/a.jpg" {
		t.Fatalf("Members[0].ProfileImageURL = %v, unexpected", m0.ProfileImageURL)
	}

	m1 := got.Members[1]
	if m1.Nickname != nil {
		t.Fatalf("Members[1].Nickname = %v, want nil", m1.Nickname)
	}

	if m1.LastActiveAt != nil {
		t.Fatalf("Members[1].LastActiveAt = %v, want nil", m1.LastActiveAt)
	}
}

func TestStatsResponseJSON(t *testing.T) {
	raw := `{
		"chatId": 100,
		"period": {"from": 1711526400000, "to": 1711612800000},
		"totalMessages": 1234,
		"activeMembers": 42,
		"topMembers": [
			{
				"userId": 1001,
				"nickname": "alice",
				"messageCount": 200,
				"lastActiveAt": 1711612800000,
				"messageTypes": {"text": 150, "image": 50}
			}
		]
	}`

	var got StatsResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", got.ChatID)
	}

	if got.Period.From != 1711526400000 || got.Period.To != 1711612800000 {
		t.Fatalf("Period = %+v, unexpected", got.Period)
	}

	if got.TotalMessages != 1234 {
		t.Fatalf("TotalMessages = %d, want 1234", got.TotalMessages)
	}

	if got.ActiveMembers != 42 {
		t.Fatalf("ActiveMembers = %d, want 42", got.ActiveMembers)
	}

	if len(got.TopMembers) != 1 {
		t.Fatalf("len(TopMembers) = %d, want 1", len(got.TopMembers))
	}

	tm := got.TopMembers[0]
	if tm.UserID != 1001 {
		t.Fatalf("TopMembers[0].UserID = %d, want 1001", tm.UserID)
	}

	if tm.Nickname == nil || *tm.Nickname != "alice" {
		t.Fatalf("TopMembers[0].Nickname = %v, want alice", tm.Nickname)
	}

	if tm.MessageTypes[testReplyTypeText] != 150 || tm.MessageTypes[testReplyTypeImage] != 50 {
		t.Fatalf("TopMembers[0].MessageTypes = %v, unexpected", tm.MessageTypes)
	}
}
