# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Go client library for Iris (KakaoTalk message bridge). Provides H2C-based message sending (`client/`), webhook receiving (`webhook/`), and Valkey deduplication (`dedup/`).

## Commands

```bash
# All tests
go test ./...

# Single package
go test ./client/
go test ./webhook/
go test ./dedup/

# Specific test function
go test ./webhook/ -run TestHandlerServeHTTP

# Race detection
go test -race ./...

# Vet
go vet ./...
```

## Architecture

### Packages

```
client/  <- stdlib + x/net/http2
webhook/ <- stdlib only
dedup/   <- webhook.Deduplicator interface only
```

- **Module path**: `github.com/park285/iris-client-go`
- **`client/`**: `H2CClient` implements both `Sender` and `AdminClient` interfaces. Types (`ReplyRequest`, `Config`, `DecryptRequest`, `DecryptResponse`), constants (`Path*`, `HeaderBotToken`), and `SendOption`/`ClientOption` functional options also live here. Internal `normalize.go` sanitizes threadID/threadScope before sending. Consumers depend on the minimal interface they need (e.g. `client.Sender` only)
- **`webhook/`**: `Handler` implements `http.Handler`. `NewHandler` requires a `MessageHandler` interface (the consumer's message processing callback). Types (`WebhookRequest`, `Message`, `MessageJSON`), constants (`PathWebhook`, `HeaderIris*`, `DefaultDedupTTL`), and `ResolveThreadID`/`DedupKey` helpers also live here. Async processing uses a stripe worker pool. Accepts injected `Metrics` and `Deduplicator` interfaces (defaults: `NoopMetrics{}`, `NoopDeduplicator{}`)
- **`dedup/`**: `ValkeyDeduplicator` — Valkey (SET NX) implementation of `webhook.Deduplicator`

### External dependencies

- `golang.org/x/net/http2` — H2C transport (used in `client/` only)
- `github.com/valkey-io/valkey-go` — Valkey client (used in `dedup/` only)

### Key patterns

- **Functional options**: `client.SendOption`, `client.ClientOption`, `webhook.HandlerOption` all follow the same pattern. `webhook.HandlerOptions` struct holds resolved configuration
- **Transport selection**: `client.WithTransport()` > `IRIS_TRANSPORT` env var > auto-detect from URL scheme (`http://` = H2C, `https://` = HTTP/1.1)
- **3-stage Ping probe**: `/ready` -> `/health` -> `OPTIONS /reply` with fallback, exponential backoff retry (50ms, 100ms, max 3 attempts hardcoded)
- **Reply retry**: `client.WithReplyRetry(n)` enables 429 retry with configurable max attempts (disabled by default)
- **Dedup defaults**: `webhook.DefaultDedupTTL` is the default TTL used by the webhook handler for duplicate detection
- **Stripe worker pool**: Webhook messages hashed by `room:threadId` to stripes, preserving message ordering within a thread
- **Constant-time token comparison**: Webhook auth uses `crypto/subtle.ConstantTimeCompare`
