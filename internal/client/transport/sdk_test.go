package transport

import (
	"errors"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/v4/internal/testsupport"
)

func TestNewClientAppliesEachOptionOnce(t *testing.T) {
	t.Parallel()

	calls := 0
	countingOption := func(*clientOptions) { calls++ }

	c, err := NewClient(WithBaseURL("http://127.0.0.1:1"), WithBotToken("token"), WithTransport(TransportHTTP1), countingOption)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", c.Close)

	if calls != 1 {
		t.Fatalf("option calls = %d, want 1", calls)
	}
}

// 설정 오류는 호출 때마다 늦게 드러나지 않고 생성 시점에 반환된다.
func TestNewClientRejectsMissingValuesAndConfigErrors(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		opts []ClientOption
		want string
	}{
		{name: "missing base URL", opts: []ClientOption{WithBotToken("token")}, want: "iris: base URL is required"},
		{name: "blank bot token", opts: []ClientOption{WithBaseURL("https://host.invalid"), WithBotToken("  ")}, want: "iris: bot token is required"},
		{name: "invalid base endpoint", opts: []ClientOption{WithBaseURL("https://user:secret@host.invalid"), WithBotToken("token")}, want: "iris: invalid base endpoint"},
		{name: "unsupported transport", opts: []ClientOption{WithBaseURL("https://host.invalid"), WithBotToken("token"), WithTransport("quic")}, want: "iris: unsupported transport"},
		{name: "missing h3 CA", opts: []ClientOption{WithBaseURL("https://host.invalid"), WithBotToken("token")}, want: "H3 CA cert file is required"},
		{name: "unreadable h3 CA", opts: []ClientOption{WithBaseURL("https://host.invalid"), WithBotToken("token"), WithH3CACertFile("/nonexistent/ca.pem")}, want: "read H3 CA cert file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			c, err := NewClient(test.opts...)
			if c != nil || err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewClient() = %v, %v; want nil client and %q", c, err, test.want)
			}
		})
	}
}

func TestNewClientMissingH3CAIsSentinel(t *testing.T) {
	t.Parallel()

	_, err := NewClient(WithBaseURL("https://host.invalid"), WithBotToken("token"))
	if !errors.Is(err, ErrMissingH3CACertFile) {
		t.Fatalf("NewClient() error = %v, want ErrMissingH3CACertFile", err)
	}
}
