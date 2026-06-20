package webhook

import (
	"strings"
	"testing"
)

func TestIC03WebhookWhitespacePayloadRejected_543d8949(t *testing.T) {
	t.Parallel()

	whitespace := strings.Repeat(" ", 1024)

	cases := []struct {
		name string
		req  *WebhookRequest
		want bool
	}{
		{
			name: "whitespace-only oversized optional field bypasses cap",
			req: &WebhookRequest{
				Text:   "hi",
				Room:   "room",
				UserID: "user",
				Route:  whitespace,
			},
			want: false,
		},
		{
			name: "whitespace-only oversized required field bypasses cap",
			req: &WebhookRequest{
				Text:   "hi",
				Room:   whitespace,
				UserID: "user",
			},
			want: false,
		},
		{
			name: "whitespace-only oversized text bypasses cap",
			req: &WebhookRequest{
				Text:   strings.Repeat(" ", 16001),
				Room:   "room",
				UserID: "user",
				Type:   "1",
			},
			want: false,
		},
		{
			name: "normal request stays valid",
			req: &WebhookRequest{
				Text:   " hello ",
				Room:   " room-1 ",
				UserID: " user-1 ",
				Route:  " default ",
			},
			want: true,
		},
		{
			name: "empty optional field stays valid",
			req: &WebhookRequest{
				Text:   "hi",
				Room:   "room",
				UserID: "user",
				Route:  "",
			},
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validWebhookRequest(tc.req); got != tc.want {
				t.Fatalf("validWebhookRequest = %v, want %v", got, tc.want)
			}
		})
	}
}
