package preset

import (
	"log/slog"
	"time"

	legacyclient "github.com/park285/iris-client-go/client"
	"github.com/park285/iris-client-go/dedup"
	legacywebhook "github.com/park285/iris-client-go/webhook"
	"github.com/valkey-io/valkey-go"
)

// ValkeyDeduplicator는 Valkey dedup 구현 타입 별칭입니다.
type ValkeyDeduplicator = dedup.ValkeyDeduplicator

// ClientConfig는 공통 Iris client preset 구성을 담습니다.
type ClientConfig struct {
	Logger                *slog.Logger
	Transport             string
	Timeout               time.Duration
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
}

// WebhookConfig는 공통 Iris webhook preset 구성을 담습니다.
type WebhookConfig struct {
	Metrics        legacywebhook.Metrics
	Deduplicator   legacywebhook.Deduplicator
	WorkerCount    int
	QueueSize      int
	EnqueueTimeout time.Duration
	HandlerTimeout time.Duration
	RequireHTTP2   bool
	DedupTTL       time.Duration
	DedupTimeout   time.Duration
	MaxBodyBytes   int64
}

// ClientDefaults는 공통 client preset 목록을 반환합니다.
func ClientDefaults(logger *slog.Logger) []legacyclient.ClientOption {
	return ClientOptions(ClientConfig{Logger: logger})
}

// ClientOptions는 재사용 가능한 client option 조합을 반환합니다.
func ClientOptions(cfg ClientConfig) []legacyclient.ClientOption {
	opts := make([]legacyclient.ClientOption, 0, 8)

	if cfg.Logger != nil {
		opts = append(opts, legacyclient.WithLogger(cfg.Logger))
	}
	if cfg.Transport != "" {
		opts = append(opts, legacyclient.WithTransport(cfg.Transport))
	}
	if cfg.Timeout > 0 {
		opts = append(opts, legacyclient.WithTimeout(cfg.Timeout))
	}
	if cfg.DialTimeout > 0 {
		opts = append(opts, legacyclient.WithDialTimeout(cfg.DialTimeout))
	}
	if cfg.ResponseHeaderTimeout > 0 {
		opts = append(opts, legacyclient.WithResponseHeaderTimeout(cfg.ResponseHeaderTimeout))
	}
	if cfg.IdleConnTimeout > 0 {
		opts = append(opts, legacyclient.WithIdleConnTimeout(cfg.IdleConnTimeout))
	}
	if cfg.MaxIdleConns > 0 {
		opts = append(opts, legacyclient.WithMaxIdleConns(cfg.MaxIdleConns))
	}
	if cfg.MaxIdleConnsPerHost > 0 {
		opts = append(opts, legacyclient.WithMaxIdleConnsPerHost(cfg.MaxIdleConnsPerHost))
	}

	return opts
}

// WebhookOptions는 재사용 가능한 webhook option 조합을 반환합니다.
func WebhookOptions(cfg WebhookConfig) []legacywebhook.HandlerOption {
	opts := make([]legacywebhook.HandlerOption, 0, 10)

	if cfg.Metrics != nil {
		opts = append(opts, legacywebhook.WithMetrics(cfg.Metrics))
	}
	if cfg.Deduplicator != nil {
		opts = append(opts, legacywebhook.WithDeduplicator(cfg.Deduplicator))
	}
	if cfg.WorkerCount > 0 {
		opts = append(opts, legacywebhook.WithWorkerCount(cfg.WorkerCount))
	}
	if cfg.QueueSize > 0 {
		opts = append(opts, legacywebhook.WithQueueSize(cfg.QueueSize))
	}
	if cfg.EnqueueTimeout > 0 {
		opts = append(opts, legacywebhook.WithEnqueueTimeout(cfg.EnqueueTimeout))
	}
	if cfg.HandlerTimeout > 0 {
		opts = append(opts, legacywebhook.WithHandlerTimeout(cfg.HandlerTimeout))
	}
	if cfg.RequireHTTP2 {
		opts = append(opts, legacywebhook.WithRequireHTTP2(true))
	}
	if cfg.DedupTTL > 0 {
		opts = append(opts, legacywebhook.WithDedupTTL(cfg.DedupTTL))
	}
	if cfg.DedupTimeout > 0 {
		opts = append(opts, legacywebhook.WithDedupTimeout(cfg.DedupTimeout))
	}
	if cfg.MaxBodyBytes > 0 {
		opts = append(opts, legacywebhook.WithMaxBodyBytes(cfg.MaxBodyBytes))
	}

	return opts
}

// NewValkeyDeduplicator는 Valkey deduplicator를 생성합니다.
func NewValkeyDeduplicator(client valkey.Client) *ValkeyDeduplicator {
	return dedup.NewValkeyDeduplicator(client)
}

// WebhookValkeyDedup은 webhook handler용 dedup 옵션을 구성합니다.
func WebhookValkeyDedup(client valkey.Client) legacywebhook.HandlerOption {
	return legacywebhook.WithDeduplicator(NewValkeyDeduplicator(client))
}
