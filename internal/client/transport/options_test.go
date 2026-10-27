package transport

import (
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestApplySendOptions(t *testing.T) {
	threadID := "12345"
	clientRequestID := "chatbotgo:log-42:reply-v1"

	tests := []struct {
		name string
		opts []SendOption
		want sendOptions
	}{
		{
			name: "empty options",
			opts: nil,
			want: sendOptions{},
		},
		{
			name: "thread id only",
			opts: []SendOption{WithThreadID(threadID)},
			want: sendOptions{ThreadID: &threadID},
		},
		{
			name: "client request id only",
			opts: []SendOption{WithClientRequestID(clientRequestID)},
			want: sendOptions{ClientRequestID: &clientRequestID},
		},
		{
			name: "both options",
			opts: []SendOption{WithThreadID(threadID), WithClientRequestID(clientRequestID)},
			want: sendOptions{ThreadID: &threadID, ClientRequestID: &clientRequestID},
		},
		{
			name: "mentions",
			opts: []SendOption{
				WithMention(ReplyMention{UserID: 123456789, Nickname: testSenderName}),
				WithMention(ReplyMention{UserID: 987654321, At: []int{1}, Len: 3}),
			},
			want: sendOptions{
				Mentions: []ReplyMention{
					{UserID: 123456789, Nickname: testSenderName},
					{UserID: 987654321, At: []int{1}, Len: 3},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertSendOptionsEqual(t, applySendOptions(tt.opts), tt.want)
		})
	}
}

func TestValidateSendOptionsValidCases(t *testing.T) {
	threadID := "12345"
	clientRequestID := "chatbotgo:log-42:reply-v1"

	tests := []validateSendOptionsSuccessCase{
		{
			name:  "empty options",
			input: sendOptions{},
		},
		{
			name:  "valid thread id only",
			input: sendOptions{ThreadID: &threadID},
		},
		{
			name:  "valid client request id",
			input: sendOptions{ClientRequestID: &clientRequestID},
		},
		{
			name: "valid mention by nickname",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: 123456789, Nickname: testSenderName},
			}},
		},
		{
			name: "valid mention by text id",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: "talk-text-id", Nickname: testSenderName},
			}},
		},
		{
			name: "valid mention by explicit range",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: 123456789, At: []int{1}, Len: 3},
			}},
		},
	}

	runValidateSendOptionsSuccessTests(t, tests)
}

func TestValidateSendOptionsInvalidCases(t *testing.T) {
	threadID := "12a45"

	tests := []validateSendOptionsErrorCase{
		{
			name:    "reject non numeric thread id",
			input:   sendOptions{ThreadID: &threadID},
			wantErr: `iris: threadId must be numeric, got "12a45"`,
		},
		{
			name:    "reject short client request id",
			input:   sendOptions{ClientRequestID: new("short")},
			wantErr: "iris: clientRequestId must be 8..160 ASCII bytes using [A-Za-z0-9._:-]",
		},
		{
			name:    "reject client request id with space",
			input:   sendOptions{ClientRequestID: new("chatbotgo:bad space:reply-v1")},
			wantErr: "iris: clientRequestId must be 8..160 ASCII bytes using [A-Za-z0-9._:-]",
		},
		{
			name:    "reject non ascii client request id",
			input:   sendOptions{ClientRequestID: new("예티:reply-v1")},
			wantErr: "iris: clientRequestId must be 8..160 ASCII bytes using [A-Za-z0-9._:-]",
		},
		{
			name: "reject non positive mention user id",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: 0, Nickname: testSenderName},
			}},
			wantErr: "iris: mention userId must be positive, got 0",
		},
		{
			name: "reject blank text mention user id",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: " ", Nickname: testSenderName},
			}},
			wantErr: "iris: mention userId must not be blank",
		},
		{
			name: "reject mention without target",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: 123456789},
			}},
			wantErr: "iris: mention requires nickname or at/len",
		},
		{
			name: "reject mention invalid position",
			input: sendOptions{Mentions: []ReplyMention{
				{UserID: 123456789, At: []int{0}, Len: 3},
			}},
			wantErr: "iris: mention at positions must be positive, got 0",
		},
	}

	runValidateSendOptionsErrorTests(t, tests)
}

type validateSendOptionsSuccessCase struct {
	name  string
	input sendOptions
}

func runValidateSendOptionsSuccessTests(t *testing.T, tests []validateSendOptionsSuccessCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateSendOptions(tt.input); err != nil {
				t.Fatalf("validateSendOptions() error = %v, want nil", err)
			}
		})
	}
}

type validateSendOptionsErrorCase struct {
	name    string
	input   sendOptions
	wantErr string
}

func runValidateSendOptionsErrorTests(t *testing.T, tests []validateSendOptionsErrorCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSendOptions(tt.input)
			if err == nil {
				t.Fatalf("validateSendOptions() error = nil, want %q", tt.wantErr)
			}

			if err.Error() != tt.wantErr {
				t.Fatalf("validateSendOptions() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func assertSendOptionsEqual(t *testing.T, got, want sendOptions) {
	t.Helper()

	if !equalStringPtr(got.ThreadID, want.ThreadID) {
		t.Fatalf("ThreadID = %v, want %v", got.ThreadID, want.ThreadID)
	}

	if !equalStringPtr(got.ClientRequestID, want.ClientRequestID) {
		t.Fatalf("ClientRequestID = %v, want %v", got.ClientRequestID, want.ClientRequestID)
	}

	if !reflect.DeepEqual(got.Mentions, want.Mentions) {
		t.Fatalf("Mentions = %+v, want %+v", got.Mentions, want.Mentions)
	}
}

func TestApplyClientOptionsDefaults(t *testing.T) {
	got := applyClientOptions(nil)

	if got.Transport != "" || got.Timeout != 10*time.Second || got.ReplyRetryMax != 0 {
		t.Fatalf("defaults = Transport %q Timeout %v ReplyRetryMax %d, want empty/10s/0", got.Transport, got.Timeout, got.ReplyRetryMax)
	}
}

func TestApplyClientOptionsOverrides(t *testing.T) {
	guard := func(context.Context, net.IP) error { return nil }
	got := applyClientOptions([]ClientOption{
		WithTransport(TransportHTTP1),
		WithTimeout(2 * time.Second),
		WithH3DialGuard(guard),
		WithReplyRetry(3),
	})

	if got.Transport != TransportHTTP1 || got.Timeout != 2*time.Second || got.ReplyRetryMax != 3 {
		t.Fatalf("overrides = Transport %q Timeout %v ReplyRetryMax %d", got.Transport, got.Timeout, got.ReplyRetryMax)
	}

	if reflect.ValueOf(got.h3DialGuard).Pointer() != reflect.ValueOf(guard).Pointer() {
		t.Fatal("h3DialGuard was not applied")
	}
}

func TestApplyClientOptionsFallbackForNonPositiveValues(t *testing.T) {
	got := applyClientOptions([]ClientOption{WithTimeout(0), WithReplyRetry(-1)})

	if got.Timeout != 10*time.Second || got.ReplyRetryMax != 0 {
		t.Fatalf("fallback = Timeout %v ReplyRetryMax %d, want 10s/0", got.Timeout, got.ReplyRetryMax)
	}
}

func equalStringPtr(got, want *string) bool {
	if got == nil || want == nil {
		return got == want
	}

	return *got == *want
}

// 공개 helper는 전송 시점 검증과 같은 규칙이어야 한다. 갈라지면 소비자가 통과시킨 id를
// 전송이 거부한다.
func TestValidateClientRequestIDMatchesSendTimeValidation(t *testing.T) {
	t.Parallel()

	for _, id := range []string{
		"chatbotgo:room-1:reply-v1",
		"chatbotgo:room-1:reply-v1:r1",
		"short",
		"chatbotgo:bad space:reply-v1",
		"예티:reply-v1",
		"  chatbotgo:room-1:reply-v1  ",
		strings.Repeat("a", 161),
	} {
		publicErr := ValidateClientRequestID(id)
		sendErr := validateSendOptions(sendOptions{ClientRequestID: new(id)})

		if publicErr == nil {
			if sendErr != nil {
				t.Fatalf("ValidateClientRequestID(%q) = <nil>, send-time validation = %v", id, sendErr)
			}

			continue
		}

		if sendErr == nil {
			t.Fatalf("ValidateClientRequestID(%q) = %v, send-time validation = <nil>", id, publicErr)
		}

		if publicErr.Error() != sendErr.Error() {
			t.Fatalf("ValidateClientRequestID(%q) = %q, send-time validation = %q", id, publicErr, sendErr)
		}
	}
}
