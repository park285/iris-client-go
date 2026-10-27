package iris_test

import (
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/park285/iris-client-go/v3/internal/testsupport"
	iris "github.com/park285/iris-client-go/v3/iris"
)

func TestNewClient_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")
	t.Setenv("IRIS_TRANSPORT", "http1")

	client, err := iris.NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	if err := os.Unsetenv("IRIS_BASE_URL"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BASE_URL) error = %v", err)
	}

	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}

func TestNewClient_MissingBotToken(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")

	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing bot token")
	}
}

func TestNewClient_WhitespaceBotTokenOption(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	t.Setenv("IRIS_TRANSPORT", "http1")

	if err := os.Unsetenv("IRIS_BOT_TOKEN"); err != nil {
		t.Fatalf("Unsetenv(IRIS_BOT_TOKEN) error = %v", err)
	}

	_, err := iris.NewClient(iris.WithBotToken("   "))
	if err == nil {
		t.Fatal("expected error for whitespace-only bot token")
	}

	if !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\" error, got %v", err)
	}
}

func TestNewClient_WhitespaceBotTokenEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	t.Setenv("IRIS_TRANSPORT", "http1")
	t.Setenv("IRIS_BOT_TOKEN", "   ")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for whitespace-only bot token")
	}

	if !strings.Contains(err.Error(), "is required") {
		t.Fatalf("expected \"is required\" error, got %v", err)
	}
}

func TestNewClient_OptionOverridesEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")
	t.Setenv("IRIS_TRANSPORT", "http1")

	client, err := iris.NewClient(iris.WithBaseURL("http://opt-host:4000"))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestFacadeReexportsClientSDKHelpers(t *testing.T) {
	t.Parallel()

	var (
		_ iris.ClientSDKConfig
		_ = iris.WithMention(iris.ReplyMention{UserID: 1, Nickname: "tester"})
		_ = iris.WithMention(iris.ReplyMention{UserID: "talk-text-id", Nickname: "tester"})
	)
}

func TestFacadeExposesH3DialGuard(t *testing.T) {
	t.Parallel()

	opt := iris.WithH3DialGuard(func(context.Context, net.IP) error {
		return nil
	})

	if opt == nil {
		t.Fatal("WithH3DialGuard() returned nil")
	}
}

func TestFacadeH3DialGuardDenialIsNotRetried(t *testing.T) {
	// CA 파일 없이 h3 client를 만들려고 system roots opt-in 환경 변수를 켠다(병렬 불가).
	t.Setenv("IRIS_H3_ALLOW_SYSTEM_ROOTS", "1")

	blocked := errors.New("blocked h3 egress")

	var attempts atomic.Int32

	client, err := iris.NewClient(
		iris.WithBaseURL("https://localhost:443"),
		iris.WithBotToken("token"),
		iris.WithTransport("h3"),
		iris.WithReplyRetry(2),
		iris.WithH3DialGuard(func(context.Context, net.IP) error {
			attempts.Add(1)

			return blocked
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	_, err = client.SendMessageAccepted(t.Context(), "room", "msg", iris.WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if err == nil {
		t.Fatal("SendMessageAccepted() error = nil, want H3 egress deny")
	}

	if errors.Is(err, iris.ErrRetryable) {
		t.Fatalf("SendMessageAccepted() error = %v, H3 egress deny must not be retryable", err)
	}

	if !errors.Is(err, blocked) {
		t.Fatalf("SendMessageAccepted() error = %v, want %v", err, blocked)
	}

	if attempts.Load() != 1 {
		t.Fatalf("guard attempts = %d, want 1", attempts.Load())
	}
}
