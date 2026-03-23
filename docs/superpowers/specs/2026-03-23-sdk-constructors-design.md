# iris-client-go SDK Constructors

Date: 2026-03-23

## Problem

The `iris/preset` layer provides config structs and option builders, but every consumer still manually wires URL, token, transport, timeout, and connection pool options. Three consumers (settlement-go, chat-bot-go-kakao, hololive-bot) repeat the same boilerplate with slight variations.

## Solution

Add `NewClient()` and `NewWebhookHandler()` to the `iris/` facade. These read environment variables, apply sensible defaults, and accept the existing `With*` options for overrides. The `iris/preset/` package becomes deprecated.

## API

### New constructors

```go
func NewClient(opts ...ClientOption) (*H2CClient, error)
func NewWebhookHandler(handler MessageHandler, opts ...HandlerOption) (*WebhookHandler, error)
```

### New options

```go
// client (SDK-level, env override)
func WithBaseURL(url string) ClientOption
func WithBotToken(token string) ClientOption

// webhook (SDK-level, env override + defaults)
func WithWebhookToken(token string) HandlerOption
func WithWebhookLogger(logger *slog.Logger) HandlerOption
func WithContext(ctx context.Context) HandlerOption
```

### Resolution priority

```
Option value > Environment variable > Error
```

| Field | Option | Env var | Default |
|-------|--------|---------|---------|
| baseURL | `WithBaseURL` | `IRIS_BASE_URL` | required |
| botToken | `WithBotToken` | `IRIS_BOT_TOKEN` | required |
| webhookToken | `WithWebhookToken` | `IRIS_WEBHOOK_TOKEN` | required |
| logger (client) | `WithLogger` | - | `slog.Default()` |
| logger (webhook) | `WithWebhookLogger` | - | `slog.Default()` |
| ctx (webhook) | `WithContext` | - | `context.Background()` |

All other options (timeout, transport, worker count, etc.) retain their existing defaults from `applyClientOptions` / `applyHandlerOptions`.

## Implementation

### client/ package changes

Add SDK fields to `clientOptions`:

```go
type clientOptions struct {
    // ... existing fields unchanged ...
    baseURL  string  // SDK-level, used by iris.NewClient
    botToken string  // SDK-level, used by iris.NewClient
}
```

Add option functions and extraction helper:

```go
func WithBaseURL(url string) ClientOption
func WithBotToken(token string) ClientOption

type SDKConfig struct {
    BaseURL  string
    BotToken string
}

func ResolveSDKConfig(opts []ClientOption) SDKConfig
```

`NewH2CClient(url, token, opts...)` ignores `baseURL`/`botToken` fields — they are consumed only by `iris.NewClient`.

### webhook/ package changes

Add SDK fields to `handlerOptions`:

```go
type handlerOptions struct {
    // ... existing fields unchanged ...
    webhookToken string          // SDK-level
    sdkLogger    *slog.Logger    // SDK-level
    sdkCtx       context.Context // SDK-level
}
```

Add option functions and extraction helper:

```go
func WithWebhookToken(token string) HandlerOption
func WithWebhookLogger(logger *slog.Logger) HandlerOption
func WithContext(ctx context.Context) HandlerOption

type SDKConfig struct {
    Token  string
    Logger *slog.Logger
    Ctx    context.Context
}

func ResolveSDKConfig(opts []HandlerOption) SDKConfig
```

`NewHandler(ctx, token, handler, logger, opts...)` ignores SDK fields — they are consumed only by `iris.NewWebhookHandler`.

### iris/ package changes

Add `sdk.go`:

```go
func NewClient(opts ...ClientOption) (*H2CClient, error) {
    cfg := client.ResolveSDKConfig(opts)
    baseURL := firstNonEmpty(cfg.BaseURL, os.Getenv("IRIS_BASE_URL"))
    botToken := firstNonEmpty(cfg.BotToken, os.Getenv("IRIS_BOT_TOKEN"))

    if baseURL == "" {
        return nil, errors.New("iris: base URL is required (set IRIS_BASE_URL or use WithBaseURL)")
    }
    if botToken == "" {
        return nil, errors.New("iris: bot token is required (set IRIS_BOT_TOKEN or use WithBotToken)")
    }

    // WithLogger default: if no logger option provided, use slog.Default()
    opts = ensureLogger(opts)

    return NewH2CClient(baseURL, botToken, opts...), nil
}

func NewWebhookHandler(handler MessageHandler, opts ...HandlerOption) (*WebhookHandler, error) {
    if handler == nil {
        return nil, errors.New("iris: message handler is required")
    }

    cfg := webhook.ResolveSDKConfig(opts)
    token := firstNonEmpty(cfg.Token, os.Getenv("IRIS_WEBHOOK_TOKEN"))

    if token == "" {
        return nil, errors.New("iris: webhook token is required (set IRIS_WEBHOOK_TOKEN or use WithWebhookToken)")
    }

    logger := cfg.Logger
    if logger == nil {
        logger = slog.Default()
    }

    ctx := cfg.Ctx
    if ctx == nil {
        ctx = context.Background()
    }

    return NewHandler(ctx, token, handler, logger, opts...), nil
}
```

Re-export new options from client/ and webhook/:

```go
var (
    WithBaseURL      = client.WithBaseURL
    WithBotToken     = client.WithBotToken
    WithWebhookToken = webhook.WithWebhookToken
    WithWebhookLogger = webhook.WithWebhookLogger
    WithContext       = webhook.WithContext
)
```

### Valkey dedup

`preset.NewValkeyDeduplicator` and `preset.WebhookValkeyDedup` move to iris/ facade as re-exports from dedup/:

```go
var NewValkeyDeduplicator = dedup.NewValkeyDeduplicator

func WithValkeyDedup(valkeyClient valkey.Client) HandlerOption {
    return WithDeduplicator(NewValkeyDeduplicator(valkeyClient))
}
```

## Deprecated

The entire `iris/preset/` package:
- `ClientConfig`, `WebhookConfig` — replaced by env + `With*` options
- `ClientOptions()`, `WebhookOptions()` — replaced by `NewClient()`, `NewWebhookHandler()`
- `ClientDefaults()` — replaced by `NewClient(WithLogger(logger))`
- `NewValkeyDeduplicator()` — moved to iris/ facade
- `WebhookValkeyDedup()` — replaced by `WithValkeyDedup()`

Mark with `// Deprecated:` doc comments. Remove in next major version.

## Consumer migration

### settlement-go

```go
// Before
client := iris.NewH2CClient(cfg.BaseURL, cfg.BotToken, preset.ClientDefaults(logger)...)
handler := iris.NewHandler(ctx, token, msgHandler, logger, preset.WebhookOptions(preset.WebhookConfig{
    HandlerTimeout: settlementCommandTimeout,
})...)

// After
client, err := iris.NewClient(iris.WithLogger(logger))
handler, err := iris.NewWebhookHandler(msgHandler,
    iris.WithWebhookLogger(logger),
    iris.WithHandlerTimeout(settlementCommandTimeout),
)
```

### chat-bot-go-kakao

```go
// Before
opts := buildIrisClientOptions(cfg, logger)
client := iris.NewH2CClient(cfg.IrisBaseURL, cfg.IrisBotToken, opts...)

whOpts := buildIrisWebhookOptions(cfg, metrics, valkeyClient)
handler := webhook.NewHandler(ctx, cfg.IrisWebhookToken, msgHandler, logger, whOpts...)

// After
client, err := iris.NewClient(
    iris.WithLogger(logger),
    iris.WithTimeout(cfg.HTTPTimeout),
    // ... remaining overrides if any differ from defaults
)
handler, err := iris.NewWebhookHandler(msgHandler,
    iris.WithWebhookLogger(logger),
    iris.WithContext(ctx),
    iris.WithValkeyDedup(valkeyClient),
    iris.WithMetrics(metrics),
)
```

Delete `buildIrisClientOptions()` and `buildIrisWebhookOptions()`.

### hololive-bot

```go
// Before (uses client/ and webhook/ directly)
import iris "github.com/park285/iris-client-go/client"
import iriswebhook "github.com/park285/iris-client-go/webhook"

client := iris.NewH2CClient(cfg.BaseURL, cfg.BotToken, preset.ClientOptions(preset.ClientConfig{...})...)
handler := iriswebhook.NewHandler(ctx, token, msgHandler, logger, preset.WebhookOptions(...)...)

// After (unified to iris/ facade)
import "github.com/park285/iris-client-go/iris"

client, err := iris.NewClient(iris.WithLogger(logger))
handler, err := iris.NewWebhookHandler(msgHandler,
    iris.WithWebhookLogger(logger),
    iris.WithContext(ctx),
    iris.WithValkeyDedup(valkeyClient),
)
```

Delete custom iris interface definitions where they duplicate `iris.Client`.

## Testing

### iris-client-go

- `TestNewClient_ReadsEnv` — set env, call NewClient(), verify client is configured
- `TestNewClient_OptionOverridesEnv` — set env, call with WithBaseURL, verify option wins
- `TestNewClient_MissingRequiredFields` — no env, no option, verify error
- `TestNewClient_DefaultLogger` — no WithLogger, verify slog.Default() is used
- `TestNewWebhookHandler_ReadsEnv` — set env, call NewWebhookHandler(), verify handler
- `TestNewWebhookHandler_NilHandler` — verify error
- `TestWithValkeyDedup_SetsDeduplicator` — verify dedup is configured

### Consumer repos

- Existing tests adapted to use new constructors
- Remove preset-based test helpers

## Files changed

### iris-client-go
- `client/options.go` — add baseURL, botToken fields + WithBaseURL, WithBotToken
- `client/sdk.go` (new) — ResolveSDKConfig helper
- `webhook/options.go` — add webhookToken, sdkLogger, sdkCtx fields + options
- `webhook/sdk.go` (new) — ResolveSDKConfig helper
- `iris/sdk.go` (new) — NewClient, NewWebhookHandler, WithValkeyDedup, ensureLogger
- `iris/iris.go` — re-export new symbols
- `iris/preset/preset.go` — add Deprecated comments
- `README.md` — update examples to SDK constructors

### settlement-go
- `cmd/settlement/iris_adapter.go` — use iris.NewClient, iris.NewWebhookHandler
- `cmd/settlement/main.go` — remove manual env wiring for iris

### chat-bot-go-kakao
- `internal/app/app.go` — replace buildIris*Options with SDK constructors
- `internal/app/app_test.go` — adapt tests
- Remove preset import

### hololive-bot
- `hololive/hololive-shared/pkg/providers/infra_providers.go` — use iris.NewClient
- `hololive/hololive-kakao-bot-go/internal/app/bootstrap_bot_webhook_youtube.go` — use iris.NewWebhookHandler
- Unify imports from client/webhook/dedup to iris/
- Adapt tests
