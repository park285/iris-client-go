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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertJSONRoundTrip(t, tt.input, tt.wantJSON, tt.wantRound, "ReplyRequest")
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
