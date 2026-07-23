package webhook

import (
	"encoding/json"
	"testing"
)

func TestMessageContextNormalizesEnvelope(t *testing.T) {
	sender := " Sender "
	threadID := " 77 "
	threadScope := 2
	sourceLogID := int64(99)
	rawSourceLogID := int64(41)
	sourceGenerationID := int64(2)
	isMine := false
	ctx := NewMessageContext(&Message{
		Msg:    "fallback",
		Room:   " room-fallback ",
		Sender: &sender,
		JSON: &MessageJSON{
			ChatID: " 42 ", Message: "exact text", UserID: " 7 ", Route: " events ",
			Type: " 0 ", ThreadID: &threadID, ThreadScope: &threadScope,
			MessageID: " msg ", ChatLogID: " log ", RoomType: " OM ", RoomLinkID: " 55 ",
			SourceLogID: &sourceLogID, RawSourceLogID: &rawSourceLogID,
			SourceGenerationID: &sourceGenerationID, SourceAccountID: " acct ", IsMine: &isMine,
			Origin: " WRITE ", Attachment: "{\"x\":1}",
			Mentions:     []WebhookMention{{UserID: " 8 ", Nickname: " N ", At: []int{1}, Len: 1}},
			EventPayload: json.RawMessage(`{"type":"kakao_feed","status":"recognized","kind":"user_joined"}`),
		},
	})
	if got := ctx.RoomID(); got != "42" {
		t.Fatalf("RoomID=%q", got)
	}
	if got := ctx.Text(); got != "exact text" {
		t.Fatalf("Text=%q", got)
	}
	if got := ctx.Sender(); got != "Sender" {
		t.Fatalf("Sender=%q", got)
	}
	if got := ctx.UserID(); got != "7" {
		t.Fatalf("UserID=%q", got)
	}
	if got := ctx.Route(); got != "events" {
		t.Fatalf("Route=%q", got)
	}
	if got := ctx.ThreadID(); got != "77" {
		t.Fatalf("ThreadID=%q", got)
	}
	if got, ok := ctx.ThreadScope(); !ok || got != 2 {
		t.Fatalf("ThreadScope=%d,%v", got, ok)
	}
	if got := ctx.EventType(); got != EventTypeKakaoFeed {
		t.Fatalf("EventType=%q", got)
	}
	if got := ctx.EventKind(); got != KakaoFeedKindUserJoined {
		t.Fatalf("EventKind=%q", got)
	}
	if got := ctx.EventStatus(); got != KakaoFeedStatusRecognized {
		t.Fatalf("EventStatus=%q", got)
	}
	if got := ctx.StableMessageIdentity(); got != "message:msg" {
		t.Fatalf("StableMessageIdentity=%q", got)
	}
	if got := ctx.RoomType(); got != "OM" {
		t.Fatalf("RoomType=%q", got)
	}
	if got := ctx.RoomLinkID(); got != "55" {
		t.Fatalf("RoomLinkID=%q", got)
	}
	if got, ok := ctx.RawSourceLogID(); !ok || got != 41 {
		t.Fatalf("RawSourceLogID=%d,%v", got, ok)
	}
	if got, ok := ctx.SourceGenerationID(); !ok || got != 2 {
		t.Fatalf("SourceGenerationID=%d,%v", got, ok)
	}
	if got := ctx.SourceAccountID(); got != "acct" {
		t.Fatalf("SourceAccountID=%q", got)
	}
	if got, ok := ctx.IsMine(); !ok || got {
		t.Fatalf("IsMine=%v,%v", got, ok)
	}
	if got := ctx.Origin(); got != "WRITE" {
		t.Fatalf("Origin=%q", got)
	}
	if got := ctx.Attachment(); got != `{"x":1}` {
		t.Fatalf("Attachment=%q", got)
	}
	mentions := ctx.Mentions()
	if len(mentions) != 1 || mentions[0].UserID != "8" || mentions[0].Nickname != "N" {
		t.Fatalf("Mentions=%v", mentions)
	}
	mentions[0].At[0] = 9
	if got := ctx.Mentions()[0].At[0]; got != 1 {
		t.Fatalf("mention snapshot At=%d", got)
	}
	if ctx.IsText() {
		t.Fatal("feed must not be text")
	}
}

func TestMessageContextFallsBackWithoutMutatingPayload(t *testing.T) {
	raw := json.RawMessage(`{"type":42}`)
	message := &Message{Msg: " raw ", Room: " room ", JSON: &MessageJSON{Type: " 1 ", EventPayload: raw}}
	ctx := NewMessageContext(message)
	if got := ctx.RoomID(); got != "room" {
		t.Fatalf("RoomID=%q", got)
	}
	if got := ctx.Text(); got != " raw " {
		t.Fatalf("Text=%q", got)
	}
	if got := ctx.EventType(); got != MessageTypeText {
		t.Fatalf("EventType=%q", got)
	}
	if !ctx.IsText() {
		t.Fatal("blank/1 type must be text")
	}
	copyPayload := ctx.EventPayload()
	copyPayload[0] = '['
	message.JSON.ChatID = "changed"
	if got := ctx.RoomID(); got != "room" {
		t.Fatalf("snapshot RoomID=%q", got)
	}
	if string(message.JSON.EventPayload) != string(raw) {
		t.Fatal("EventPayload must return a copy")
	}
}

func TestMessageContextStableMessageIdentityPrecedence(t *testing.T) {
	sourceLogID := int64(3)
	message := &Message{JSON: &MessageJSON{MessageID: "m", ChatLogID: "c", SourceLogID: &sourceLogID}}
	if got := NewMessageContext(message).StableMessageIdentity(); got != "message:m" {
		t.Fatal(got)
	}
	message.JSON.MessageID = ""
	if got := NewMessageContext(message).StableMessageIdentity(); got != "chat-log:c" {
		t.Fatal(got)
	}
	message.JSON.ChatLogID = ""
	if got := NewMessageContext(message).StableMessageIdentity(); got != "source-log:3" {
		t.Fatal(got)
	}
}
