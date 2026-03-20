package webhook

import "testing"

func TestResolveThreadID(t *testing.T) {
	runResolveThreadIDTests(t, []resolveThreadIDTestCase{
		{
			name: "nil request",
			req:  nil,
			want: "",
		},
		{
			name: "direct thread id wins",
			req: &WebhookRequest{
				ThreadID:  " 12345 ",
				ChatLogID: "99999",
				RoomType:  "OD",
			},
			want: "12345",
		},
		{
			name: "open talk room type falls back to chat log id",
			req: &WebhookRequest{
				ChatLogID: " 54321 ",
				RoomType:  "od",
			},
			want: "54321",
		},
		{
			name: "room link id falls back to chat log id",
			req: &WebhookRequest{
				ChatLogID:  "77777",
				RoomLinkID: "link-1",
			},
			want: "77777",
		},
		{
			name: "non open talk does not fall back",
			req: &WebhookRequest{
				ChatLogID: "88888",
				RoomType:  "OM",
			},
			want: "",
		},
		{
			name: "empty chat log id does not fall back",
			req: &WebhookRequest{
				ChatLogID: " \t ",
				RoomType:  "OD",
			},
			want: "",
		},
	})
}

type resolveThreadIDTestCase struct {
	name string
	req  *WebhookRequest
	want string
}

func runResolveThreadIDTests(t *testing.T, tests []resolveThreadIDTestCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveThreadID(tt.req)
			if got != tt.want {
				t.Fatalf("ResolveThreadID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDedupKey(t *testing.T) {
	tests := []struct {
		name      string
		messageID string
		want      string
	}{
		{
			name:      "empty message id",
			messageID: "",
			want:      "",
		},
		{
			name:      "whitespace only message id",
			messageID: "  \n\t  ",
			want:      "",
		},
		{
			name:      "trimmed message id",
			messageID: "  abc123  ",
			want:      "iris:msg:{abc123}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DedupKey(tt.messageID)
			if got != tt.want {
				t.Fatalf("DedupKey() = %q, want %q", got, tt.want)
			}
		})
	}
}
