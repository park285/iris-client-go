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
			name: "open talk room type does not fall back to chat log id",
			req: &WebhookRequest{
				ChatLogID: " 54321 ",
				RoomType:  "od",
			},
			want: "",
		},
		{
			name: "room link id does not fall back to chat log id",
			req: &WebhookRequest{
				ChatLogID:  "77777",
				RoomLinkID: "link-1",
			},
			want: "",
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
			got := resolveThreadID(tt.req)
			if got != tt.want {
				t.Fatalf("resolveThreadID() = %q, want %q", got, tt.want)
			}
		})
	}
}
