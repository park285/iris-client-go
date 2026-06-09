# Changelog

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
