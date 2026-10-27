package webhook

import "testing"

func TestMessageContextNormalizesEnvelope(t *testing.T) {
	ctx := NewMessageContext(&Message{
		Msg:  "exact text",
		Room: " 42 ",
		JSON: &MessageJSON{
			UserID: " 7 ", Route: " events ", Type: " 0 ", MessageID: " msg ",
			EventPayload: []byte(`{"type":"kakao_feed","schemaVersion":1,"status":"recognized","kind":"user_joined"}`),
		},
	})

	for _, field := range []struct {
		name, got, want string
	}{
		{"RoomID", ctx.RoomID(), "42"},
		{"Route", ctx.Route(), "events"},
		{"EventType", ctx.EventType(), EventTypeKakaoFeed},
		{"EventKind", ctx.EventKind(), KakaoFeedKindUserJoined},
		{"EventStatus", ctx.EventStatus(), KakaoFeedStatusRecognized},
	} {
		if field.got != field.want {
			t.Fatalf("%s=%q, want %q", field.name, field.got, field.want)
		}
	}

	if got, ok := ctx.EventSchemaVersion(); !ok || got != KakaoFeedSchemaVersion {
		t.Fatalf("EventSchemaVersion=%d,%v", got, ok)
	}
}

func TestMessageContextFallsBackWithoutMutatingPayload(t *testing.T) {
	raw := []byte(`{"type":42}`)
	message := &Message{Msg: " raw ", Room: " room ", JSON: &MessageJSON{Type: " 1 ", EventPayload: raw}}
	ctx := NewMessageContext(message)

	if got := ctx.RoomID(); got != testRoom {
		t.Fatalf("RoomID=%q", got)
	}

	// payload header가 해석되지 않으면 정규화한 wire type으로 대체한다.
	if got := ctx.EventType(); got != "1" {
		t.Fatalf("EventType=%q", got)
	}

	if _, ok := ctx.EventSchemaVersion(); ok {
		t.Fatal("EventSchemaVersion must be absent for an undecodable header")
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
