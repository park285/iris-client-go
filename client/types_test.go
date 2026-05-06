package client

import (
	"reflect"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestReplyRequestJSON(t *testing.T) {
	threadID := "12345"
	threadScope := 2

	tests := []struct {
		name      string
		input     ReplyRequest
		wantJSON  string
		wantRound ReplyRequest
	}{
		{
			name: "omit empty optional fields",
			input: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "hello",
			},
			wantJSON: `{"type":"text","room":"room-a","data":"hello"}`,
			wantRound: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "hello",
			},
		},
		{
			name: "include optional thread fields",
			input: ReplyRequest{
				Type:        "text",
				Room:        "room-a",
				Data:        "hello",
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
			},
			wantJSON: `{"type":"text","room":"room-a","data":"hello","threadId":"12345","threadScope":2}`,
			wantRound: ReplyRequest{
				Type:        "text",
				Room:        "room-a",
				Data:        "hello",
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
			},
		},
		{
			name: "include mentions",
			input: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: 123456789, Nickname: "홍길동"},
					{UserID: 987654321, At: []int{1}, Len: 3},
				},
			},
			wantJSON: `{"type":"text","room":"room-a","data":"@홍길동 hello","mentions":[{"userId":123456789,"nickname":"홍길동"},{"userId":987654321,"at":[1],"len":3}]}`,
			wantRound: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: int64(123456789), Nickname: "홍길동"},
					{UserID: int64(987654321), At: []int{1}, Len: 3},
				},
			},
		},
		{
			name: "include text id mention",
			input: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: "talk-text-id", Nickname: "홍길동"},
				},
			},
			wantJSON: `{"type":"text","room":"room-a","data":"@홍길동 hello","mentions":[{"userId":"talk-text-id","nickname":"홍길동"}]}`,
			wantRound: ReplyRequest{
				Type: "text",
				Room: "room-a",
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: "talk-text-id", Nickname: "홍길동"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyRequest")
		})
	}
}

func TestReplyImageMetadataJSON(t *testing.T) {
	threadID := "12345"
	threadScope := 1

	tests := []struct {
		name      string
		input     replyImageMetadata
		wantJSON  string
		wantRound replyImageMetadata
	}{
		{
			name: "minimal with empty images",
			input: replyImageMetadata{
				Type:   "image",
				Room:   "room-a",
				Images: []imagePartSpec{},
			},
			wantJSON: `{"type":"image","room":"room-a","images":[]}`,
			wantRound: replyImageMetadata{
				Type:   "image",
				Room:   "room-a",
				Images: []imagePartSpec{},
			},
		},
		{
			name: "with images manifest",
			input: replyImageMetadata{
				Type: "image",
				Room: "room-a",
				Images: []imagePartSpec{
					{Index: 0, SHA256Hex: "abcd1234", ByteLength: 1024, ContentType: "image/png"},
				},
			},
			wantJSON: `{"type":"image","room":"room-a","images":[{"index":0,"sha256Hex":"abcd1234","byteLength":1024,"contentType":"image/png"}]}`,
			wantRound: replyImageMetadata{
				Type: "image",
				Room: "room-a",
				Images: []imagePartSpec{
					{Index: 0, SHA256Hex: "abcd1234", ByteLength: 1024, ContentType: "image/png"},
				},
			},
		},
		{
			name: "include optional thread fields and multiple images",
			input: replyImageMetadata{
				Type:        "image_multiple",
				Room:        "room-a",
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
				Images: []imagePartSpec{
					{Index: 0, SHA256Hex: "aaa", ByteLength: 100, ContentType: "image/jpeg"},
					{Index: 1, SHA256Hex: "bbb", ByteLength: 200, ContentType: "image/png"},
				},
			},
			wantJSON: `{"type":"image_multiple","room":"room-a","threadId":"12345","threadScope":1,"images":[{"index":0,"sha256Hex":"aaa","byteLength":100,"contentType":"image/jpeg"},{"index":1,"sha256Hex":"bbb","byteLength":200,"contentType":"image/png"}]}`,
			wantRound: replyImageMetadata{
				Type:        "image_multiple",
				Room:        "room-a",
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
				Images: []imagePartSpec{
					{Index: 0, SHA256Hex: "aaa", ByteLength: 100, ContentType: "image/jpeg"},
					{Index: 1, SHA256Hex: "bbb", ByteLength: 200, ContentType: "image/png"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "replyImageMetadata")
		})
	}
}

func assertJSONRoundTrip[T any](t *testing.T, input T, wantJSON string, wantRound T, label string) {
	t.Helper()

	gotJSON, err := jsonx.Marshal(input)
	if err != nil {
		t.Fatalf("jsonx.Marshal() error = %v", err)
	}

	if string(gotJSON) != wantJSON {
		t.Fatalf("jsonx.Marshal() = %s, want %s", gotJSON, wantJSON)
	}

	var got T
	if err := jsonx.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("jsonx.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, wantRound, label)
}

func assertJSONEqual[T any](t *testing.T, got, want T, label string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}
