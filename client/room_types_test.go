package client

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
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
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(got.Rooms) != 2 {
		t.Fatalf("len(Rooms) = %d, want 2", len(got.Rooms))
	}

	r0 := got.Rooms[0]
	if r0.ChatID != 100 {
		t.Fatalf("Rooms[0].ChatID = %d, want 100", r0.ChatID)
	}
	if r0.Type == nil || *r0.Type != "open" {
		t.Fatalf("Rooms[0].Type = %v, want open", r0.Type)
	}
	if r0.LinkID == nil || *r0.LinkID != 200 {
		t.Fatalf("Rooms[0].LinkID = %v, want 200", r0.LinkID)
	}
	if r0.ActiveMembersCount == nil || *r0.ActiveMembersCount != 50 {
		t.Fatalf("Rooms[0].ActiveMembersCount = %v, want 50", r0.ActiveMembersCount)
	}
	if r0.LinkName == nil || *r0.LinkName != "test-room" {
		t.Fatalf("Rooms[0].LinkName = %v, want test-room", r0.LinkName)
	}
	if r0.LinkURL == nil || *r0.LinkURL != "https://open.kakao.com/o/test" {
		t.Fatalf("Rooms[0].LinkURL = %v, unexpected", r0.LinkURL)
	}
	if r0.MemberLimit == nil || *r0.MemberLimit != 300 {
		t.Fatalf("Rooms[0].MemberLimit = %v, want 300", r0.MemberLimit)
	}
	if r0.Searchable == nil || *r0.Searchable != 1 {
		t.Fatalf("Rooms[0].Searchable = %v, want 1", r0.Searchable)
	}
	if r0.BotRole == nil || *r0.BotRole != 2 {
		t.Fatalf("Rooms[0].BotRole = %v, want 2", r0.BotRole)
	}

	// Minimal room: only required chatId
	r1 := got.Rooms[1]
	if r1.ChatID != 101 {
		t.Fatalf("Rooms[1].ChatID = %d, want 101", r1.ChatID)
	}
	if r1.Type != nil {
		t.Fatalf("Rooms[1].Type = %v, want nil", r1.Type)
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
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
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
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
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
	if tm.MessageTypes["text"] != 150 || tm.MessageTypes["image"] != 50 {
		t.Fatalf("TopMembers[0].MessageTypes = %v, unexpected", tm.MessageTypes)
	}
}

func TestMemberActivityResponseJSON(t *testing.T) {
	raw := `{
		"userId": 1001,
		"nickname": "alice",
		"messageCount": 500,
		"firstMessageAt": 1711000000000,
		"lastMessageAt": 1711612800000,
		"activeHours": [9, 10, 14, 15, 21],
		"messageTypes": {"text": 400, "image": 100}
	}`

	var got MemberActivityResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.UserID != 1001 {
		t.Fatalf("UserID = %d, want 1001", got.UserID)
	}
	if got.Nickname == nil || *got.Nickname != "alice" {
		t.Fatalf("Nickname = %v, want alice", got.Nickname)
	}
	if got.MessageCount != 500 {
		t.Fatalf("MessageCount = %d, want 500", got.MessageCount)
	}
	if got.FirstMessageAt == nil || *got.FirstMessageAt != 1711000000000 {
		t.Fatalf("FirstMessageAt = %v, want 1711000000000", got.FirstMessageAt)
	}
	if got.LastMessageAt == nil || *got.LastMessageAt != 1711612800000 {
		t.Fatalf("LastMessageAt = %v, want 1711612800000", got.LastMessageAt)
	}
	if len(got.ActiveHours) != 5 {
		t.Fatalf("len(ActiveHours) = %d, want 5", len(got.ActiveHours))
	}
	if got.ActiveHours[0] != 9 || got.ActiveHours[4] != 21 {
		t.Fatalf("ActiveHours = %v, unexpected", got.ActiveHours)
	}
	if got.MessageTypes["text"] != 400 || got.MessageTypes["image"] != 100 {
		t.Fatalf("MessageTypes = %v, unexpected", got.MessageTypes)
	}
}

func TestRoomInfoResponseJSON(t *testing.T) {
	raw := `{
		"chatId": 100,
		"type": "open",
		"linkId": 200,
		"notices": [
			{"content": "Welcome!", "authorId": 1001, "updatedAt": 1711612800000}
		],
		"blindedMemberIds": [9001, 9002],
		"botCommands": [
			{"name": "!help", "botId": 42}
		],
		"openLink": {
			"name": "Test Room",
			"url": "https://open.kakao.com/o/test",
			"memberLimit": 300,
			"description": "A test room",
			"searchable": 1
		}
	}`

	var got RoomInfoResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", got.ChatID)
	}
	if got.Type == nil || *got.Type != "open" {
		t.Fatalf("Type = %v, want open", got.Type)
	}
	if got.LinkID == nil || *got.LinkID != 200 {
		t.Fatalf("LinkID = %v, want 200", got.LinkID)
	}

	// Notices
	if len(got.Notices) != 1 {
		t.Fatalf("len(Notices) = %d, want 1", len(got.Notices))
	}
	if got.Notices[0].Content != "Welcome!" || got.Notices[0].AuthorID != 1001 {
		t.Fatalf("Notices[0] = %+v, unexpected", got.Notices[0])
	}

	// BlindedMemberIDs
	if len(got.BlindedMemberIDs) != 2 || got.BlindedMemberIDs[0] != 9001 {
		t.Fatalf("BlindedMemberIDs = %v, unexpected", got.BlindedMemberIDs)
	}

	// BotCommands
	if len(got.BotCommands) != 1 || got.BotCommands[0].Name != "!help" || got.BotCommands[0].BotID != 42 {
		t.Fatalf("BotCommands = %+v, unexpected", got.BotCommands)
	}

	// OpenLink
	if got.OpenLink == nil {
		t.Fatal("OpenLink = nil, want non-nil")
	}
	if got.OpenLink.Name == nil || *got.OpenLink.Name != "Test Room" {
		t.Fatalf("OpenLink.Name = %v, want Test Room", got.OpenLink.Name)
	}
	if got.OpenLink.URL == nil || *got.OpenLink.URL != "https://open.kakao.com/o/test" {
		t.Fatalf("OpenLink.URL = %v, unexpected", got.OpenLink.URL)
	}
	if got.OpenLink.MemberLimit == nil || *got.OpenLink.MemberLimit != 300 {
		t.Fatalf("OpenLink.MemberLimit = %v, want 300", got.OpenLink.MemberLimit)
	}
}

func TestRoomInfoResponseNilOpenLinkJSON(t *testing.T) {
	raw := `{
		"chatId": 101,
		"notices": [],
		"blindedMemberIds": [],
		"botCommands": []
	}`

	var got RoomInfoResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.ChatID != 101 {
		t.Fatalf("ChatID = %d, want 101", got.ChatID)
	}
	if got.OpenLink != nil {
		t.Fatalf("OpenLink = %v, want nil", got.OpenLink)
	}
}
