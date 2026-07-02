# Changelog

## [v0.24.0] - 2026-07-02

### Removed (Breaking)

- **BREAKING**: Removed the backward-compat `iris` facade re-exports of the public `webhook`
  package. `webhook` is already public and is now the canonical import path for the webhook
  message schema, handler options, and the raw handler. Consumers must import
  `github.com/park285/iris-client-go/webhook` directly and move the affected `iris.*` symbols to
  `webhook.*`.
  - Types: `iris.Message`, `iris.MessageJSON`, `iris.WebhookHandler`, `iris.MessageHandler`,
    `iris.HandlerOption`, `iris.HandlerOptions`, `iris.WebhookRequest`, `iris.WebhookMention`,
    `iris.Metrics`, `iris.NoopMetrics`, `iris.Deduplicator`, `iris.NoopDeduplicator`, `iris.TaskPool`,
    `iris.WebhookOrderingMode`, `iris.WebhookReceiveDiagnostics`, `iris.WebhookSDKConfig`,
    `iris.WebhookDedupMode` → `webhook.Message`, `webhook.MessageJSON`, `webhook.Handler`,
    `webhook.MessageHandler`, `webhook.HandlerOption`, `webhook.HandlerOptions`,
    `webhook.WebhookRequest`, `webhook.WebhookMention`, `webhook.Metrics`, `webhook.NoopMetrics`,
    `webhook.Deduplicator`, `webhook.NoopDeduplicator`, `webhook.TaskPool`, `webhook.OrderingMode`,
    `webhook.ReceiveDiagnostics`, `webhook.SDKConfig`, `webhook.DedupMode`.
  - Constants: `iris.PathWebhook`, `iris.HeaderIrisToken`, `iris.HeaderIrisMessageID`,
    `iris.HeaderIrisRoute`, `iris.DefaultDedupTTL`, `iris.WebhookOrderingModeKey/None`,
    `iris.WebhookDedupModeBeforeDecode/AfterDecode` → the matching `webhook.*` names
    (`webhook.OrderingModeKey/None`, `webhook.DedupModeBeforeDecode/AfterDecode`, ...).
  - Functions/vars: `iris.NewHandler`, `iris.WithWebhookOrderingMode`, `iris.WithDedupMode`,
    `iris.ResolveWebhookSDKConfig`, and the webhook option re-exports (`iris.WithWebhookToken`,
    `iris.WithWebhookLogger`, `iris.WithContext`, `iris.WithMetrics`, `iris.WithDeduplicator`,
    `iris.WithTaskPool`, `iris.WithWorkerCount`, `iris.WithQueueSize`, `iris.WithEnqueueTimeout`,
    `iris.WithHandlerTimeout`, `iris.WithRequireHTTP2`, `iris.WithDedupTTL`, `iris.WithDedupTimeout`,
    `iris.WithMaxBodyBytes`, `iris.WithAutoWorkerCount`, `iris.ResolveThreadID`, `iris.DedupKey`) →
    the matching `webhook.*` names (`webhook.WithOrderingMode`, `webhook.WithDedupMode`,
    `webhook.NewHandler`, `webhook.ResolveSDKConfig`, `webhook.WithWebhookToken`, ...).
- **BREAKING**: Removed the `KaringHololiveStream` type alias (`iris.KaringHololiveStream` and the
  internal `client.KaringHololiveStream`), which aliased `KaringContentItem`. Use
  `iris.KaringContentItem`; `KaringHololiveRequest.Stream`/`.Streams` are now `*KaringContentItem`
  / `[]KaringContentItem`.

### Notes

- The `iris` package stays the SDK entry point. `iris.NewClient`, `iris.NewWebhookHandler`
  (env-resolving webhook constructor that accepts `webhook.HandlerOption` values),
  `iris.WithValkeyDedup` / `iris.NewValkeyDeduplicator` / `iris.ValkeyDeduplicator`, and every
  `client`-backed re-export (types, error contracts, path/header/option symbols, runtime
  diagnostics) are retained. Those types live in the intentionally-internal `internal/client`
  package (compiler-enforced boundary; the HMAC signer stays unexported and file-scoped), so the
  `iris` aliases are their only public surface and are not backward-compat shims.

### Performance

- Reworked the SSE event-stream parser to operate on `[]byte` end to end: lines are consumed via
  `scanner.Bytes()`, data lines accumulate into a reusable buffer, each event allocates once via
  `bytes.Clone`, and event IDs parse through a zero-alloc `[]byte` parser equivalent to
  `strconv.ParseInt` (sign and overflow semantics included). Room-event hot path: 402→204 allocs/op,
  18,522→10,689 B/op, 32,387→17,659 ns/op per 100-event stream. An allocation-budget test and the
  `perf-smoke` benchmark gate guard the budget.
- Pooled per-secret HMAC signer states (`sync.Pool` of `hash.Hash`) so request signing no longer
  recomputes the key schedule per call, and added half-jitter (`[base/2, base]`) to fallback retry
  backoff; `Retry-After` still takes precedence.
- Raised the default `MaxConnsPerHost` to 32.

### Fixed

- The pooled HMAC hash is now always returned to the pool after signing, and the pool `Get`
  type assertion is checked — a foreign value falls back to a fresh HMAC state instead of
  panicking.

### Internal

- Moved the per-call signing helpers (`signIrisRequest`, `signIrisRequestWithBodySHA256`) into
  test-only code; production signing always goes through the prebuilt per-secret signer cache.
- Added retry-after delay bound tests for the lock path.

### CI

- Hardened workflows: concurrency groups with cancel-in-progress, job timeouts, and full-SHA
  action pins; adopted the stack-canonical `check-workflow-secrets` checker with profile
  auto-detection.

## [v0.17.0] - 2026-06-10

### Added

- Added `iris.BotClient`, the minimal bot-consumer interface (`Sender` + `Ping` + `GetConfig`).
- Added `iris.RebindingClient` / `iris.NewRebindingClient`: a base-URL hot-swapping client that
  resolves the target per call, reuses the cached client while the URL is unchanged, and closes
  the rotated-out client after `StaleCloseGrace`.

### Fixed

- Classified transport-init failures on the raw GET/POST request paths (config, rooms, diagnostics,
  cert-reload) as `TransportError{Op: "init"}` (non-retryable); previously they surfaced as
  `Op: "get"`/`Op: "post"` and matched `ErrRetryable`.
- Hardened the request signing path: canonical query strictly percent-decodes, preserves literal
  plus and flag params, and fails closed on malformed input so the signed and sent targets can no
  longer diverge; path segments are validated against a safe-token charset with a length cap;
  multipart image admission mirrors the runtime limit and boundary/nonce generation falls back
  deterministically when `crypto/rand` fails.
- Fixed a `webhook.Handler.Close()` hang when an externally injected `TaskPool` rejects work:
  `SubmitWait` returning false now releases the in-flight keys so the dispatcher can drain.

### Removed

- Removed internal dead code: the `wrapHTTPError` identity wrapper and the legacy `newHTTPClient`
  constructor; collapsed `PingError`'s dual `Err`/`err` fields into the exported `Err`. No public
  API surface changed.

### CI

- Wired the cross-cutting boundary checker into the CI fast gate (transport TLS / webhook worker
  recovery baselines).

## [v0.16.0] - 2026-06-08

### Added

- Added `ChatLogID` to `MemberNicknameUpdatedEvent`, matching the nullable `chatLogId` in the Iris nickname ledger payload.
- Added typed SSE bodies `SSERoomEventBody` (`room_event` frame) and `SSEStreamState` (`iris.stream_state` frame).
- Added contract constants: `EventTypeMemberNicknameUpdated`, `SSEEventRoomEvent`, `SSEEventStreamState`, `StreamCursorStatusCurrent/Stale/Future`, `StreamRecoveryQueryRecentMessages`.
- Added `webhook.HeaderIrisRoute` (`X-Iris-Route`) for the webhook delivery header Iris always sets.
- Added `iris.WithDedupMode` with `WebhookDedupModeAfterDecode` for consumers that must reject malformed webhook bodies before consuming a dedup key.

### Fixed

- Fixed `ConfigDiscoveredState.BotID` to decode the `botId` key Iris serializes; the previous `bot_id` tag always decoded 0.
- Fixed `KaringDryRunResponse` to decode the live 202 camelCase response (`receiverName`, `templateId`, `itemCount`, `streamCount`); previously those fields were silently dropped in live mode.
- Preserved `Retry-After` as `HTTPError.RetryAfter` and used it for bounded reply retry delays.
- Hardened SSE parsing for `field:value` frames, bounded scanner tokens to 1MiB, and surfaced scanner errors to the stream logger.
- Bounded error response body drain after the diagnostic snippet and removed avoidable HMAC/scheduler hot-path allocations.

### Removed

- Removed the retired room event struct aliases from the `iris` facade; `member_nickname_updated` is the only semantic event contract.
- Removed `RoomEventRecord.CreatedAt`; Iris serializes `createdAtMs` only.

### Docs

- Updated `docs/webhook-type-attachment.md` to the current Iris contract: attachment is an opt-in, allowlist-sanitized metadata JSON (no URL/path/raw blob), and retired event subtypes were removed.

## [v0.13.1] - 2026-05-23

### Changed

- Changed `SendImage` and `SendMultipleImages` multipart uploads to stream image payloads without buffering the full multipart body in memory while preserving retry-safe body reconstruction.

## [v0.13.0] - 2026-05-22

### Added

- Added exported sentinel errors: `ErrRetryable`, `ErrPermanent`, `ErrAuthFailed`, `ErrRateLimited`, and `ErrTransport`.
- Added typed errors: `HTTPError`, `TransportError`, and `PingError`.

### Changed

- Changed transport selection so `ForceAttemptHTTP2` is enabled only for explicit HTTP/2 mode and remains disabled for explicit HTTP/1.1 mode.
- Replaced internal error types with exported equivalents while keeping one-version aliases for compatibility.

### Notes

- This release explicitly overrides the Phase G "public API symbol 유지" policy to preserve the newly exported public API symbols.
- Multipart streaming (P2.1) split to follow-up Plan G. See /home/kapu/work/iris-stack/iris-client-go/docs/2026-05-22-plan-g-multipart-streaming.md (forthcoming).
