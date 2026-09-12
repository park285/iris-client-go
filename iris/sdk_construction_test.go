package iris_test

import (
	"testing"

	"github.com/park285/iris-client-go/v2/internal/testsupport"
	"github.com/park285/iris-client-go/v2/iris"
	"github.com/park285/iris-client-go/v2/webhook"
)

func TestSDKWebhookConstructorsApplyOptionsOnce(t *testing.T) {
	t.Setenv(iris.EnvWebhookToken, "")

	for _, test := range []struct {
		name      string
		construct func(...webhook.HandlerOption) (*webhook.Handler, error)
	}{
		{"message", func(opts ...webhook.HandlerOption) (*webhook.Handler, error) {
			return iris.NewWebhookHandler(stubHandler{}, opts...)
		}},
		{"durable", func(opts ...webhook.HandlerOption) (*webhook.Handler, error) {
			return iris.NewDurableWebhookHandler(stubAdmitter{}, opts...)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0

			handler, err := test.construct(
				webhook.WithWebhookToken("test-webhook-token"),
				webhook.WithNonceStore(testNonceStore{}),
				nil,
				func(*webhook.Handler) { calls++ },
			)
			if err != nil {
				t.Fatalf("construct handler: %v", err)
			}

			testsupport.CloseNow(t, "handler.Close", handler.Close)

			if calls != 1 {
				t.Fatalf("option calls = %d, want 1", calls)
			}
		})
	}
}
