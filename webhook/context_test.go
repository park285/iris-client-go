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
		Msg:    "exact text",
		Room:   " 42 ",
		Sender: &sender,
		JSON: &MessageJSON{
			UserID: " 7 ", Route: " events ",
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
		{"MessageID", ctx.MessageID(), "msg"},
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
	if len(mentions) != 1 || mentions[0].UserID != "8" || mentions[0].Nickname != "N" || len(mentions[0].At) != 1 {
		t.Fatalf("Mentions=%v", mentions)
	}

	mentions[0].At[0] = 9

	snapshot := ctx.Mentions()
	if len(snapshot) != 1 || len(snapshot[0].At) != 1 {
		t.Fatalf("mention snapshot=%v", snapshot)
	}

	if got := snapshot[0].At[0]; got != 1 {
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
	message.Room = "changed"

	if got := ctx.RoomID(); got != testRoom {
		t.Fatalf("snapshot RoomID=%q", got)
	}

	if string(message.JSON.EventPayload) != string(raw) {
		t.Fatal("EventPayload must return a copy")
	}
}
