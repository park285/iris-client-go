package rebind

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/client/transport"
	"github.com/park285/iris-client-go/v2/internal/testsupport"
)

func TestRebindingInitErrorDoesNotDiscloseEndpoint(t *testing.T) {
	t.Parallel()

	// #nosec G101 -- .invalid 호스트의 합성 자격증명으로 오류·로그의 URL 비노출을 검증한다.
	for name, endpoint := range map[string]string{
		"userinfo":  "https://fixture-user:fixture-password@example.invalid/private-path",
		"query":     "https://example.invalid/private-path?key=fixture-query",
		"fragment":  "https://example.invalid/private-path#fixture-fragment",
		"malformed": "https://example.invalid/private-path/%invalid",
		"prefix":    "https://example.invalid/private-path",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			logger := slog.New(slog.NewTextHandler(&logs, nil))
			client := NewRebindingClient(RebindingClientConfig{
				ResolveBaseURL: func() (string, error) { return endpoint, nil },
				BotToken:       testBotToken,
				Logger:         logger,
				ClientOptions: []transport.ClientOption{
					transport.WithTransport("invalid-fixture-transport"),
					transport.WithLogger(logger),
				},
			})

			defer testsupport.CloseNow(t, "client", client.Close)

			err := client.SendMessage(t.Context(), "room", "message")
			if err == nil || !strings.Contains(err.Error(), "initialize") {
				t.Fatalf("expected a classified initialization error, got %v", err)
			}

			for _, private := range []string{endpoint, "fixture-user", "fixture-password", "private-path", "fixture-query", "fixture-fragment"} {
				if strings.Contains(err.Error(), private) || strings.Contains(logs.String(), private) {
					t.Fatal("initialization exposed private endpoint data")
				}
			}
		})
	}
}
