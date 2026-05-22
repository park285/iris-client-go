# Changelog

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
