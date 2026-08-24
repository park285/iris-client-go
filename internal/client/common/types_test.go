package common

import (
	jsonv2 "encoding/json/v2"
	"reflect"
	"testing"
)

func TestReplyRequestJSON(t *testing.T) {
	threadID := "12345"
	threadScope := 2
	clientRequestID := "chatbotgo:log-42:reply-v1"

	tests := []struct {
		name      string
		input     ReplyRequest
		wantJSON  string
		wantRound ReplyRequest
	}{
		{
			name: "omit empty optional fields",
			input: ReplyRequest{
				Type: testReplyTypeText,
				Room: testRoomA,
				Data: testHelloText,
			},
			wantJSON: `{"type":"text","room":"room-a","data":"hello"}`,
			wantRound: ReplyRequest{
				Type: testReplyTypeText,
				Room: testRoomA,
				Data: testHelloText,
			},
		},
		{
			name: "include client request id",
			input: ReplyRequest{
				ClientRequestID: &clientRequestID,
				Type:            testReplyTypeText,
				Room:            testRoomA,
				Data:            testHelloText,
			},
			wantJSON: `{"clientRequestId":"chatbotgo:log-42:reply-v1","type":"text","room":"room-a","data":"hello"}`,
			wantRound: ReplyRequest{
				ClientRequestID: &clientRequestID,
				Type:            testReplyTypeText,
				Room:            testRoomA,
				Data:            testHelloText,
			},
		},
		{
			name: "include optional thread fields",
			input: ReplyRequest{
				Type:        testReplyTypeText,
				Room:        testRoomA,
				Data:        testHelloText,
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
			},
			wantJSON: `{"type":"text","room":"room-a","data":"hello","threadId":"12345","threadScope":2}`,
			wantRound: ReplyRequest{
				Type:        testReplyTypeText,
				Room:        testRoomA,
				Data:        testHelloText,
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyRequest")
		})
	}
}

func TestReplyRequestMentionsJSON(t *testing.T) {
	tests := []struct {
		name      string
		input     ReplyRequest
		wantJSON  string
		wantRound ReplyRequest
	}{
		{
			name: "include mentions",
			input: ReplyRequest{
				Type: testReplyTypeText,
				Room: testRoomA,
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: 123456789, Nickname: "홍길동"},
					{UserID: 987654321, At: []int{1}, Len: 3},
				},
			},
			wantJSON: `{"type":"text","room":"room-a","data":"@홍길동 hello","mentions":[{"userId":123456789,"nickname":"홍길동"},{"userId":987654321,"at":[1],"len":3}]}`,
			wantRound: ReplyRequest{
				Type: testReplyTypeText,
				Room: testRoomA,
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
				Type: testReplyTypeText,
				Room: testRoomA,
				Data: "@홍길동 hello",
				Mentions: []ReplyMention{
					{UserID: "talk-text-id", Nickname: "홍길동"},
				},
			},
			wantJSON: `{"type":"text","room":"room-a","data":"@홍길동 hello","mentions":[{"userId":"talk-text-id","nickname":"홍길동"}]}`,
			wantRound: ReplyRequest{
				Type: testReplyTypeText,
				Room: testRoomA,
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
	clientRequestID := "chatbotgo:log-42:image-v1"

	tests := []struct {
		name      string
		input     ReplyImageMetadata
		wantJSON  string
		wantRound ReplyImageMetadata
	}{
		{
			name: "minimal with empty images",
			input: ReplyImageMetadata{
				Type:   testReplyTypeImage,
				Room:   testRoomA,
				Images: []ImagePartSpec{},
			},
			wantJSON: `{"type":"image","room":"room-a","images":[]}`,
			wantRound: ReplyImageMetadata{
				Type:   testReplyTypeImage,
				Room:   testRoomA,
				Images: []ImagePartSpec{},
			},
		},
		{
			name: "include client request id",
			input: ReplyImageMetadata{
				ClientRequestID: &clientRequestID,
				Type:            testReplyTypeImage,
				Room:            testRoomA,
				Images:          []ImagePartSpec{},
			},
			wantJSON: `{"clientRequestId":"chatbotgo:log-42:image-v1","type":"image","room":"room-a","images":[]}`,
			wantRound: ReplyImageMetadata{
				ClientRequestID: &clientRequestID,
				Type:            testReplyTypeImage,
				Room:            testRoomA,
				Images:          []ImagePartSpec{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyImageMetadata")
		})
	}
}

func TestReplyImageMetadataImagesJSON(t *testing.T) {
	threadID := "12345"
	threadScope := 1

	tests := []struct {
		name      string
		input     ReplyImageMetadata
		wantJSON  string
		wantRound ReplyImageMetadata
	}{
		{
			name: "with images manifest",
			input: ReplyImageMetadata{
				Type: testReplyTypeImage,
				Room: testRoomA,
				Images: []ImagePartSpec{
					{Index: 0, SHA256Hex: "abcd1234", ByteLength: 1024, ContentType: "image/png"},
				},
			},
			wantJSON: `{"type":"image","room":"room-a","images":[{"index":0,"sha256Hex":"abcd1234","byteLength":1024,"contentType":"image/png"}]}`,
			wantRound: ReplyImageMetadata{
				Type: testReplyTypeImage,
				Room: testRoomA,
				Images: []ImagePartSpec{
					{Index: 0, SHA256Hex: "abcd1234", ByteLength: 1024, ContentType: "image/png"},
				},
			},
		},
		{
			name: "include optional thread fields and multiple images",
			input: ReplyImageMetadata{
				Type:        "image_multiple",
				Room:        testRoomA,
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
				Images: []ImagePartSpec{
					{Index: 0, SHA256Hex: "aaa", ByteLength: 100, ContentType: "image/jpeg"},
					{Index: 1, SHA256Hex: "bbb", ByteLength: 200, ContentType: "image/png"},
				},
			},
			wantJSON: `{"type":"image_multiple","room":"room-a","threadId":"12345","threadScope":1,"images":[{"index":0,"sha256Hex":"aaa","byteLength":100,"contentType":"image/jpeg"},{"index":1,"sha256Hex":"bbb","byteLength":200,"contentType":"image/png"}]}`,
			wantRound: ReplyImageMetadata{
				Type:        "image_multiple",
				Room:        testRoomA,
				ThreadID:    &threadID,
				ThreadScope: &threadScope,
				Images: []ImagePartSpec{
					{Index: 0, SHA256Hex: "aaa", ByteLength: 100, ContentType: "image/jpeg"},
					{Index: 1, SHA256Hex: "bbb", ByteLength: 200, ContentType: "image/png"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyImageMetadata")
		})
	}
}

func assertJSONRoundTrip[T any](t *testing.T, input T, wantJSON string, wantRound T, label string) {
	t.Helper()

	gotJSON, err := jsonv2.Marshal(input)
	if err != nil {
		t.Fatalf("jsonv2.Marshal() error = %v", err)
	}

	if string(gotJSON) != wantJSON {
		t.Fatalf("jsonv2.Marshal() = %s, want %s", gotJSON, wantJSON)
	}

	var got T

	if err := jsonv2.Unmarshal(gotJSON, &got); err != nil {
		t.Fatalf("jsonv2.Unmarshal() error = %v", err)
	}

	assertJSONEqual(t, got, wantRound, label)
}

func assertJSONEqual[T any](t *testing.T, got, want T, label string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", label, got, want)
	}
}
