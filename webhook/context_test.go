package webhook

import "testing"

func TestMessageContextNormalizesEnvelope(t *testing.T) {
	ctx := newNormalizedEnvelopeContext()

	assertNormalizedEnvelopeStrings(t, ctx)
	assertNormalizedEnvelopeOptionals(t, ctx)
	assertNormalizedEnvelopeMentions(t, ctx)

	if ctx.IsText() {
		t.Fatal("feed must not be text")
	}
}

func newNormalizedEnvelopeContext() MessageContext {
	sender := " Sender "
	threadID := " 77 "
	threadScope := 2
	sourceLogID := int64(99)
	rawSourceLogID := int64(41)
	sourceGenerationID := int64(2)
	isMine := false

	return NewMessageContext(&Message{
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
			EventPayload: []byte(`{"type":"kakao_feed","schemaVersion":1,"status":"recognized","kind":"user_joined"}`),
		},
	})
}

func assertNormalizedEnvelopeStrings(t *testing.T, ctx MessageContext) {
	t.Helper()

	for _, field := range []struct {
		name, got, want string
	}{
		{"RoomID", ctx.RoomID(), "42"},
		{"Text", ctx.Text(), "exact text"},
		{"Sender", ctx.Sender(), "Sender"},
		{"UserID", ctx.UserID(), "7"},
		{"Route", ctx.Route(), "events"},
		{"ThreadID", ctx.ThreadID(), "77"},
		{"EventType", ctx.EventType(), EventTypeKakaoFeed},
		{"EventKind", ctx.EventKind(), KakaoFeedKindUserJoined},
		{"EventStatus", ctx.EventStatus(), KakaoFeedStatusRecognized},
		{"StableMessageIdentity", ctx.StableMessageIdentity(), "message:msg"},
		{"RoomType", ctx.RoomType(), "OM"},
		{"RoomLinkID", ctx.RoomLinkID(), "55"},
		{"SourceAccountID", ctx.SourceAccountID(), "acct"},
		{"Origin", ctx.Origin(), "WRITE"},
		{"Attachment", ctx.Attachment(), `{"x":1}`},
	} {
		if field.got != field.want {
			t.Fatalf("%s=%q, want %q", field.name, field.got, field.want)
		}
	}
}

func assertNormalizedEnvelopeOptionals(t *testing.T, ctx MessageContext) {
	t.Helper()

	if got, ok := ctx.ThreadScope(); !ok || got != 2 {
		t.Fatalf("ThreadScope=%d,%v", got, ok)
	}

	if got, ok := ctx.EventSchemaVersion(); !ok || got != KakaoFeedSchemaVersion {
		t.Fatalf("EventSchemaVersion=%d,%v", got, ok)
	}

	if got, ok := ctx.RawSourceLogID(); !ok || got != 41 {
		t.Fatalf("RawSourceLogID=%d,%v", got, ok)
	}

	if got, ok := ctx.SourceGenerationID(); !ok || got != 2 {
		t.Fatalf("SourceGenerationID=%d,%v", got, ok)
	}

	if got, ok := ctx.IsMine(); !ok || got {
		t.Fatalf("IsMine=%v,%v", got, ok)
	}
}

func assertNormalizedEnvelopeMentions(t *testing.T, ctx MessageContext) {
	t.Helper()

	mentions := ctx.Mentions()
	if len(mentions) != 1 || mentions[0].UserID != "8" || mentions[0].Nickname != "N" {
		t.Fatalf("Mentions=%v", mentions)
	}

	mentions[0].At[0] = 9
	if got := ctx.Mentions()[0].At[0]; got != 1 {
		t.Fatalf("mention snapshot At=%d", got)
	}
}

func TestMessageContextFallsBackWithoutMutatingPayload(t *testing.T) {
	raw := []byte(`{"type":42}`)
	message := &Message{Msg: " raw ", Room: " room ", JSON: &MessageJSON{Type: " 1 ", EventPayload: raw}}
	ctx := NewMessageContext(message)

	if got := ctx.RoomID(); got != testRoom {
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

	if got := ctx.RoomID(); got != testRoom {
		t.Fatalf("snapshot RoomID=%q", got)
	}

	if string(message.JSON.EventPayload) != string(raw) {
		t.Fatal("EventPayload must return a copy")
	}
}

func TestMessageContextStableMessageIdentityPrecedence(t *testing.T) {
	sourceLogID := int64(3)
	generationID := int64(2)
	message := &Message{Room: testRoom, JSON: &MessageJSON{
		MessageID: "m", ChatLogID: "c", SourceLogID: &sourceLogID,
		SourceGenerationID: &generationID, SourceAccountID: "account",
	}}

	if got := NewMessageContext(message).StableMessageIdentity(); got != "message:m" {
		t.Fatal(got)
	}

	message.JSON.MessageID = ""
	if got := NewMessageContext(message).StableMessageIdentity(); got != "source:account:2:3" {
		t.Fatal(got)
	}

	message.JSON.SourceLogID = nil
	if got := NewMessageContext(message).StableMessageIdentity(); got != "chat-log:g2:room:c" {
		t.Fatal(got)
	}
}

func TestMessageContextStableMessageIdentityRequiresScopedFallback(t *testing.T) {
	sourceLogID := int64(3)
	message := &Message{JSON: &MessageJSON{SourceLogID: &sourceLogID, ChatLogID: "c"}}

	if got := NewMessageContext(message).StableMessageIdentity(); got != "" {
		t.Fatalf("unscoped identity=%q", got)
	}

	message.Room = testRoom
	if got := NewMessageContext(message).StableMessageIdentity(); got != "source-room:room:0:3" {
		t.Fatalf("room-scoped identity=%q", got)
	}
}
