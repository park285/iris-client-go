# SDK Constructors Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `NewClient()` and `NewWebhookHandler()` to the `iris/` facade with env auto-reading and sensible defaults, deprecate `iris/preset/`.

**Architecture:** SDK fields are added to `clientOptions` (client/) and `Handler` (webhook/) structs. `ResolveSDKConfig()` helpers extract these before delegating to existing constructors. The `iris/` facade adds `sdk.go` with the new constructors and re-exports.

**Tech Stack:** Go 1.26, iris-client-go (client/, webhook/, dedup/, iris/), valkey-go

**Spec:** `docs/superpowers/specs/2026-03-23-sdk-constructors-design.md`

**Worktree:** `/home/kapu/.config/superpowers/worktrees/iris-client-go/iris-standardization-excl-game-bot`

**Cross-repo verification workspace:** `/home/kapu/.config/superpowers/worktrees/gemini/iris-standardization-excl-game-bot/go.work`

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `client/options.go` | Modify | Add `baseURL`, `botToken` fields + `WithBaseURL`, `WithBotToken` |
| `client/sdk.go` | Create | `SDKConfig` type + `ResolveSDKConfig()` helper |
| `webhook/handler.go` | Modify | Add `sdkToken`, `sdkLogger`, `sdkCtx` fields to `Handler` |
| `webhook/sdk.go` | Create | `WithWebhookToken`, `WithWebhookLogger`, `WithContext` options + `SDKConfig` + `ResolveSDKConfig()` |
| `iris/sdk.go` | Create | `NewClient()`, `NewWebhookHandler()`, `WithValkeyDedup()`, `firstNonEmpty()` |
| `iris/iris.go` | Modify | Re-export new symbols |
| `iris/preset/preset.go` | Modify | Add `// Deprecated:` comments |
| `README.md` | Modify | Update examples to SDK constructors |

---

### Task 1: client/ SDK fields and helpers

**Files:**
- Modify: `client/options.go`
- Create: `client/sdk.go`

**Key context:**
- `ClientOption` is `func(*clientOptions)` (line 80 of options.go)
- `clientOptions` is unexported struct (line 59)
- `applyClientOptions` applies options and fills defaults (line 204)
- `NewH2CClient` takes `baseURL, botToken` positionally — these new fields are ignored by it

- [ ] **Step 1: Write failing test for WithBaseURL/WithBotToken**

Create `client/sdk_test.go`:
```go
package client

import "testing"

func TestResolveSDKConfig(t *testing.T) {
	t.Parallel()

	t.Run("extracts base URL and bot token from options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]ClientOption{
			WithBaseURL("http://test:3000"),
			WithBotToken("test-token"),
		})
		if cfg.BaseURL != "http://test:3000" {
			t.Fatalf("BaseURL = %q, want %q", cfg.BaseURL, "http://test:3000")
		}
		if cfg.BotToken != "test-token" {
			t.Fatalf("BotToken = %q, want %q", cfg.BotToken, "test-token")
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]ClientOption{WithTimeout(5)})
		if cfg.BaseURL != "" || cfg.BotToken != "" {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		cfg := ResolveSDKConfig(nil)
		if cfg.BaseURL != "" || cfg.BotToken != "" {
			t.Fatal("expected zero SDK config")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/kapu/.config/superpowers/worktrees/iris-client-go/iris-standardization-excl-game-bot && GOWORK=off go test ./client -run TestResolveSDKConfig -v`
Expected: FAIL — `WithBaseURL`, `WithBotToken`, `ResolveSDKConfig` undefined

- [ ] **Step 3: Add SDK fields to clientOptions**

In `client/options.go`, add to `clientOptions` struct (after `ReplyRetryMax` field, around line 78):
```go
	baseURL  string // SDK-level: used by iris.NewClient, ignored by NewH2CClient
	botToken string // SDK-level: used by iris.NewClient, ignored by NewH2CClient
```

Add option functions (after `WithReplyRetry`, around line 202):
```go
// WithBaseURL sets the Iris server URL. Used by iris.NewClient.
func WithBaseURL(url string) ClientOption {
	return func(o *clientOptions) {
		o.baseURL = url
	}
}

// WithBotToken sets the bot authentication token. Used by iris.NewClient.
func WithBotToken(token string) ClientOption {
	return func(o *clientOptions) {
		o.botToken = token
	}
}
```

- [ ] **Step 4: Create client/sdk.go**

```go
package client

// SDKConfig holds SDK-level settings extracted from ClientOption.
// Used by iris.NewClient to resolve base URL and bot token.
type SDKConfig struct {
	BaseURL  string
	BotToken string
}

// ResolveSDKConfig applies options and extracts SDK-level config.
func ResolveSDKConfig(opts []ClientOption) SDKConfig {
	var o clientOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return SDKConfig{BaseURL: o.baseURL, BotToken: o.botToken}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/kapu/.config/superpowers/worktrees/iris-client-go/iris-standardization-excl-game-bot && GOWORK=off go test ./client -run TestResolveSDKConfig -v`
Expected: PASS

- [ ] **Step 6: Run full client test suite**

Run: `GOWORK=off go test ./client/... -count=1`
Expected: All existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add client/options.go client/sdk.go client/sdk_test.go
git commit -m "feat(client): add SDK config fields and ResolveSDKConfig helper"
```

---

### Task 2: webhook/ SDK fields and helpers

**Files:**
- Modify: `webhook/handler.go`
- Create: `webhook/sdk.go`

**Key context:**
- `HandlerOption` is `func(*Handler)` (line 75 of handler.go) — differs from client
- SDK fields go on `Handler` struct directly (not a sub-options struct)
- `NewHandler` takes `ctx, token, handler, logger` as positional args

- [ ] **Step 1: Write failing test for webhook SDK config**

Create `webhook/sdk_test.go`:
```go
package webhook

import (
	"context"
	"log/slog"
	"testing"
)

func TestResolveSDKConfig(t *testing.T) {
	t.Parallel()

	t.Run("extracts token logger and context from options", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		ctx := context.Background()

		cfg := ResolveSDKConfig([]HandlerOption{
			WithWebhookToken("wh-token"),
			WithWebhookLogger(logger),
			WithContext(ctx),
		})
		if cfg.Token != "wh-token" {
			t.Fatalf("Token = %q, want %q", cfg.Token, "wh-token")
		}
		if cfg.Logger != logger {
			t.Fatal("Logger mismatch")
		}
		if cfg.Ctx != ctx {
			t.Fatal("Ctx mismatch")
		}
	})

	t.Run("returns zero config when no SDK options", func(t *testing.T) {
		cfg := ResolveSDKConfig([]HandlerOption{WithWorkerCount(8)})
		if cfg.Token != "" || cfg.Logger != nil || cfg.Ctx != nil {
			t.Fatal("expected zero SDK config")
		}
	})

	t.Run("returns zero config for nil slice", func(t *testing.T) {
		cfg := ResolveSDKConfig(nil)
		if cfg.Token != "" {
			t.Fatal("expected zero SDK config")
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./webhook -run TestResolveSDKConfig -v`
Expected: FAIL — undefined symbols

- [ ] **Step 3: Add SDK fields to Handler struct**

In `webhook/handler.go`, add to `Handler` struct (after `baseCtxFn` field, line 61):
```go
	// SDK-level fields: used by iris.NewWebhookHandler only, ignored by NewHandler.
	sdkToken  string
	sdkLogger *slog.Logger
	sdkCtx    context.Context
```

- [ ] **Step 4: Create webhook/sdk.go**

```go
package webhook

import (
	"context"
	"log/slog"
)

// WithWebhookToken sets the webhook authentication token. Used by iris.NewWebhookHandler.
func WithWebhookToken(token string) HandlerOption {
	return func(h *Handler) {
		h.sdkToken = token
	}
}

// WithWebhookLogger sets the logger. Used by iris.NewWebhookHandler.
func WithWebhookLogger(logger *slog.Logger) HandlerOption {
	return func(h *Handler) {
		h.sdkLogger = logger
	}
}

// WithContext sets the base context. Used by iris.NewWebhookHandler.
func WithContext(ctx context.Context) HandlerOption {
	return func(h *Handler) {
		h.sdkCtx = ctx
	}
}

// SDKConfig holds SDK-level settings extracted from HandlerOption.
type SDKConfig struct {
	Token  string
	Logger *slog.Logger
	Ctx    context.Context
}

// ResolveSDKConfig applies options to a zero Handler and extracts SDK fields.
func ResolveSDKConfig(opts []HandlerOption) SDKConfig {
	var h Handler
	for _, opt := range opts {
		if opt != nil {
			opt(&h)
		}
	}
	return SDKConfig{Token: h.sdkToken, Logger: h.sdkLogger, Ctx: h.sdkCtx}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `GOWORK=off go test ./webhook -run TestResolveSDKConfig -v`
Expected: PASS

- [ ] **Step 6: Run full webhook test suite**

Run: `GOWORK=off go test ./webhook/... -count=1`
Expected: All existing tests still pass

- [ ] **Step 7: Commit**

```bash
git add webhook/handler.go webhook/sdk.go webhook/sdk_test.go
git commit -m "feat(webhook): add SDK config fields and ResolveSDKConfig helper"
```

---

### Task 3: iris/ SDK constructors

**Files:**
- Create: `iris/sdk.go`
- Create: `iris/sdk_test.go`
- Modify: `iris/iris.go`

**Key context:**
- `iris/iris.go` re-exports all types and functions from client/ and webhook/
- `NewH2CClient(url, token, opts...)` and `NewHandler(ctx, token, handler, logger, opts...)` already exist as facade wrappers
- `dedup.NewValkeyDeduplicator(valkey.Client)` needs re-export for `WithValkeyDedup`
- `WithLogger` already exists as `= client.WithLogger` but applies to client only
- `NewH2CClient` already defaults nil logger to `slog.Default()`, so no `ensureLogger` needed

- [ ] **Step 1: Write failing tests for NewClient**

Create `iris/sdk_test.go`:
```go
package iris_test

import (
	"os"
	"reflect"
	"testing"

	iris "github.com/park285/iris-client-go/iris"
)

func TestNewClient_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")

	client, err := iris.NewClient()
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil")
	}
}

func TestNewClient_OptionOverridesEnv(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://env-host:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")

	client, err := iris.NewClient(iris.WithBaseURL("http://opt-host:4000"))
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// baseURL option should override env
	value := reflect.ValueOf(client).Elem()
	baseURL := value.FieldByName("baseURL").String()
	if baseURL != "http://opt-host:4000" {
		t.Fatalf("baseURL = %q, want %q", baseURL, "http://opt-host:4000")
	}
}

func TestNewClient_MissingBaseURL(t *testing.T) {
	os.Unsetenv("IRIS_BASE_URL")
	os.Unsetenv("IRIS_BOT_TOKEN")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing base URL")
	}
}

func TestNewClient_MissingBotToken(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://host:3000")
	os.Unsetenv("IRIS_BOT_TOKEN")

	_, err := iris.NewClient()
	if err == nil {
		t.Fatal("expected error for missing bot token")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GOWORK=off go test ./iris -run TestNewClient -v`
Expected: FAIL — `NewClient`, `WithBaseURL` undefined

- [ ] **Step 3: Write failing tests for NewWebhookHandler**

Append to `iris/sdk_test.go`:
```go
type stubHandler struct{}

func (stubHandler) HandleMessage(_ context.Context, _ *iris.Message) {}

func TestNewWebhookHandler_ReadsEnv(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	handler, err := iris.NewWebhookHandler(stubHandler{})
	if err != nil {
		t.Fatalf("NewWebhookHandler() error = %v", err)
	}
	if handler == nil {
		t.Fatal("NewWebhookHandler() returned nil")
	}
	_ = handler.Close()
}

func TestNewWebhookHandler_MissingToken(t *testing.T) {
	os.Unsetenv("IRIS_WEBHOOK_TOKEN")

	_, err := iris.NewWebhookHandler(stubHandler{})
	if err == nil {
		t.Fatal("expected error for missing webhook token")
	}
}

func TestNewWebhookHandler_NilHandler(t *testing.T) {
	t.Setenv("IRIS_WEBHOOK_TOKEN", "wh-token")

	_, err := iris.NewWebhookHandler(nil)
	if err == nil {
		t.Fatal("expected error for nil handler")
	}
}
```

- [ ] **Step 4: Create iris/sdk.go**

```go
package iris

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/park285/iris-client-go/client"
	"github.com/park285/iris-client-go/dedup"
	basewebhook "github.com/park285/iris-client-go/webhook"
	"github.com/valkey-io/valkey-go"
)

const (
	// EnvBaseURL is the environment variable for the Iris server URL.
	EnvBaseURL = "IRIS_BASE_URL"
	// EnvBotToken is the environment variable for the bot authentication token.
	EnvBotToken = "IRIS_BOT_TOKEN"
	// EnvWebhookToken is the environment variable for the webhook authentication token.
	EnvWebhookToken = "IRIS_WEBHOOK_TOKEN"
)

// NewClient creates an Iris client.
// Base URL and bot token are read from IRIS_BASE_URL and IRIS_BOT_TOKEN
// environment variables. Use WithBaseURL/WithBotToken to override.
func NewClient(opts ...ClientOption) (*H2CClient, error) {
	cfg := client.ResolveSDKConfig(opts)

	baseURL := firstNonEmpty(cfg.BaseURL, os.Getenv(EnvBaseURL))
	if baseURL == "" {
		return nil, errors.New("iris: base URL is required (set IRIS_BASE_URL or use WithBaseURL)")
	}

	botToken := firstNonEmpty(cfg.BotToken, os.Getenv(EnvBotToken))
	if botToken == "" {
		return nil, errors.New("iris: bot token is required (set IRIS_BOT_TOKEN or use WithBotToken)")
	}

	return NewH2CClient(baseURL, botToken, opts...), nil
}

// NewWebhookHandler creates an Iris webhook handler.
// Webhook token is read from IRIS_WEBHOOK_TOKEN. Use WithWebhookToken to override.
// Logger defaults to slog.Default(). Context defaults to context.Background().
func NewWebhookHandler(handler MessageHandler, opts ...HandlerOption) (*WebhookHandler, error) {
	if handler == nil {
		return nil, errors.New("iris: message handler is required")
	}

	cfg := basewebhook.ResolveSDKConfig(opts)

	token := firstNonEmpty(cfg.Token, os.Getenv(EnvWebhookToken))
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

// NewValkeyDeduplicator creates a Valkey-backed deduplicator.
var NewValkeyDeduplicator = dedup.NewValkeyDeduplicator

// ValkeyDeduplicator is the Valkey dedup implementation type.
type ValkeyDeduplicator = dedup.ValkeyDeduplicator

// WithValkeyDedup configures the webhook handler to use Valkey deduplication.
func WithValkeyDedup(valkeyClient valkey.Client) HandlerOption {
	return WithDeduplicator(NewValkeyDeduplicator(valkeyClient))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

- [ ] **Step 5: Update iris/iris.go to re-export new symbols**

Add to the `var` block in `iris/iris.go`:
```go
	WithBaseURL       = client.WithBaseURL
	WithBotToken      = client.WithBotToken
	WithWebhookToken  = basewebhook.WithWebhookToken
	WithWebhookLogger = basewebhook.WithWebhookLogger
	WithContext        = basewebhook.WithContext
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `GOWORK=off go test ./iris/... -count=1 -v`
Expected: All tests pass

- [ ] **Step 7: Run full repo test suite**

Run: `GOWORK=off go test ./... -count=1`
Expected: All tests pass

- [ ] **Step 8: Commit**

```bash
git add iris/sdk.go iris/sdk_test.go iris/iris.go
git commit -m "feat(iris): add NewClient and NewWebhookHandler SDK constructors"
```

---

### Task 4: Deprecate iris/preset/

**Files:**
- Modify: `iris/preset/preset.go`

- [ ] **Step 1: Add Deprecated comments to all exported symbols**

Add `// Deprecated:` doc comments to:
- `ClientConfig` — "Use iris.NewClient with WithBaseURL, WithBotToken, etc."
- `WebhookConfig` — "Use iris.NewWebhookHandler with WithWebhookToken, etc."
- `ClientDefaults` — "Use iris.NewClient(iris.WithLogger(logger))."
- `ClientOptions` — "Use iris.NewClient with individual With* options."
- `WebhookOptions` — "Use iris.NewWebhookHandler with individual With* options."
- `NewValkeyDeduplicator` — "Use iris.NewValkeyDeduplicator."
- `WebhookValkeyDedup` — "Use iris.WithValkeyDedup."

- [ ] **Step 2: Verify preset tests still pass**

Run: `GOWORK=off go test ./iris/preset/... -count=1`
Expected: PASS (deprecation comments don't break anything)

- [ ] **Step 3: Commit**

```bash
git add iris/preset/preset.go
git commit -m "chore(preset): mark all exports as deprecated in favor of iris.NewClient/NewWebhookHandler"
```

---

### Task 5: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Update examples to use SDK constructors**

Replace the "message sending" example:
```go
// Before
clientOpts := preset.ClientOptions(preset.ClientConfig{...})
c := client.NewH2CClient("http://iris-host:3000", "bot-token", clientOpts...)

// After
c, err := iris.NewClient()
// or with overrides:
c, err := iris.NewClient(iris.WithTimeout(5 * time.Second))
```

Replace the "webhook" example:
```go
// Before
handlerOpts := preset.WebhookOptions(preset.WebhookConfig{...})
handler := webhook.NewHandler(ctx, "iris-webhook-token", myMessageHandler, logger, handlerOpts...)

// After
handler, err := iris.NewWebhookHandler(myMessageHandler)
// or with overrides:
handler, err := iris.NewWebhookHandler(myMessageHandler,
    iris.WithValkeyDedup(valkeyClient),
    iris.WithWorkerCount(32),
)
```

Replace the "Valkey dedup" example:
```go
// Before
preset.WebhookValkeyDedup(valkeyClient)

// After
iris.WithValkeyDedup(valkeyClient)
```

Add env var documentation table.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: update README to SDK constructor API"
```

---

### Task 6: Tag and publish iris-client-go

**Important:** This task must complete before consumer repos can update.

- [ ] **Step 1: Verify all tests pass**

Run: `GOWORK=off go test ./... -count=1`

- [ ] **Step 2: Confirm with user before tagging**

Ask: "iris-client-go를 v0.5.0으로 태그하고 push해도 됩니까?"

- [ ] **Step 3: Tag and push**

```bash
git tag v0.5.0
git push origin feat/iris-standardization-excl-game-bot --tags
```

---

### Task 7: Migrate settlement-go

**Files:**
- Modify: `cmd/settlement/iris_adapter.go`
- Modify: `cmd/settlement/main.go`
- Modify: `cmd/settlement/handler_test.go`
- Modify: `go.mod` / `go.sum`

**Worktree:** `/home/kapu/gemini/settlement-go/.worktrees/iris-standardization-excl-game-bot`

- [ ] **Step 1: Update go.mod to new iris-client-go version**

Run: `go get github.com/park285/iris-client-go@v0.5.0`

- [ ] **Step 2: Replace client construction in main.go**

Replace manual `iris.NewH2CClient(url, token, preset.ClientDefaults(logger)...)` with `iris.NewClient(iris.WithLogger(logger))`.

- [ ] **Step 3: Replace webhook handler construction in iris_adapter.go**

Replace `iris.NewHandler(ctx, token, handler, logger, preset.WebhookOptions(...)...)` with:
```go
iris.NewWebhookHandler(handler,
    iris.WithWebhookLogger(logger),
    iris.WithHandlerTimeout(settlementCommandTimeout),
)
```

Remove `preset` import.

- [ ] **Step 4: Update tests**

Adapt `handler_test.go` — tests that construct webhook handlers should use new API.

- [ ] **Step 5: Run tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor(iris): migrate to SDK constructors, remove preset dependency"
```

---

### Task 8: Migrate chat-bot-go-kakao

**Files:**
- Modify: `internal/app/app.go`
- Modify: `internal/app/app_test.go`
- Modify: `go.mod` / `go.sum`

**Worktree:** `/home/kapu/.config/superpowers/worktrees/chat-bot-go-kakao/iris-standardization-excl-game-bot`

- [ ] **Step 1: Update go.mod to new iris-client-go version**

- [ ] **Step 2: Replace client/webhook construction in app.go**

Delete `buildIrisClientOptions()` and `buildIrisWebhookOptions()`.
Replace with `iris.NewClient(...)` and `iris.NewWebhookHandler(...)`.
Unify imports to `iris/` facade only (remove `webhook/` direct import).

- [ ] **Step 3: Update tests**

Adapt `app_test.go` reflection tests to match new construction path.

- [ ] **Step 4: Run tests**

Run: `go test ./... -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git commit -am "refactor(iris): migrate to SDK constructors, remove preset/buildHelper boilerplate"
```

---

### Task 9: Migrate hololive-bot

**Files:**
- Modify: `hololive/hololive-shared/pkg/providers/infra_providers.go`
- Modify: `hololive/hololive-kakao-bot-go/internal/app/bootstrap_bot_webhook_youtube.go`
- Modify: various test files
- Modify: `go.mod` / `go.sum` in affected modules

**Worktree:** `/home/kapu/gemini/hololive-bot/.worktrees/iris-standardization-excl-game-bot`

- [ ] **Step 1: Update go.mod to new iris-client-go version**

- [ ] **Step 2: Unify imports to iris/ facade**

Replace all:
```go
import iris "github.com/park285/iris-client-go/client"
import iriswebhook "github.com/park285/iris-client-go/webhook"
import irisdedup "github.com/park285/iris-client-go/dedup"
```
with:
```go
import "github.com/park285/iris-client-go/iris"
```

This affects files across hololive-kakao-bot-go and hololive-shared.

- [ ] **Step 3: Replace client construction in infra_providers.go**

Replace preset-based construction with `iris.NewClient(iris.WithLogger(logger))`.

- [ ] **Step 4: Replace webhook construction in bootstrap_bot_webhook_youtube.go**

Replace preset-based construction with `iris.NewWebhookHandler(handler, ...)`.

- [ ] **Step 5: Remove custom iris.Client interface if it duplicates iris.Client**

Check if `irisClient interface` in hololive-kakao-bot-go matches `iris.Client`. If so, replace with `iris.Client`.

- [ ] **Step 6: Update tests**

Adapt reflection-based tests in `bootstrap_bot_dependency_views_test.go` and `bootstrap_guard_additional_test.go`.

- [ ] **Step 7: Run tests**

Run: `go test ./hololive/hololive-kakao-bot-go/... -count=1`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git commit -am "refactor(iris): unify to iris/ facade, migrate to SDK constructors"
```

---

### Task 10: Clean up temporary go.work and verify cross-repo

- [ ] **Step 1: Delete temporary verification go.work**

```bash
rm /home/kapu/.config/superpowers/worktrees/gemini/iris-standardization-excl-game-bot/go.work
rm /home/kapu/.config/superpowers/worktrees/gemini/iris-standardization-excl-game-bot/go.work.sum
```

- [ ] **Step 2: Verify each consumer builds independently**

Each consumer should now resolve iris-client-go from the published tag, not from a local go.work replace.

```bash
cd <settlement-go worktree> && go build ./...
cd <chat-bot-go-kakao worktree> && go build ./...
cd <hololive-bot worktree> && go build ./...
```
