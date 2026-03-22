package preset

import (
	"log/slog"

	"github.com/park285/iris-client-go/dedup"
	iris "github.com/park285/iris-client-go/iris"
	iriswebhook "github.com/park285/iris-client-go/iris/webhook"
	"github.com/valkey-io/valkey-go"
)

// ValkeyDeduplicator는 Valkey dedup 구현 타입 별칭입니다.
type ValkeyDeduplicator = dedup.ValkeyDeduplicator

// ClientDefaults는 공통 client preset 목록을 반환합니다.
func ClientDefaults(logger *slog.Logger) []iris.ClientOption {
	if logger == nil {
		return nil
	}

	return []iris.ClientOption{iris.WithLogger(logger)}
}

// NewValkeyDeduplicator는 Valkey deduplicator를 생성합니다.
func NewValkeyDeduplicator(client valkey.Client) *ValkeyDeduplicator {
	return dedup.NewValkeyDeduplicator(client)
}

// WebhookValkeyDedup은 webhook handler용 dedup 옵션을 구성합니다.
func WebhookValkeyDedup(client valkey.Client) iriswebhook.HandlerOption {
	return iriswebhook.WithDeduplicator(NewValkeyDeduplicator(client))
}
