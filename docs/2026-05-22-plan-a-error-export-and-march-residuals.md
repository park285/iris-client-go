# Plan A — iris-client-go Error Export + March Residuals

**Date:** 2026-05-22
**Status:** Proposed (replaces relevant scope of `2026-03-20-refactoring-recommendations.md` P2.1 잔여·P2.6, supersedes `Iris/docs/refactoring-2026-05-followup.md` Phase G's *"public API symbol 유지"* policy)
**Blocks:** Plan B (consumer error migration)
**Parallel with:** Plan C, Plan D

---

## Goal

iris-client-go가 호출자에게 분류 가능한 에러를 노출하도록 sentinel + typed error를 export하고, March 2026 refactoring recs의 잔여 항목(P2.1 multipart streaming, P2.6 transport 명확화)을 마무리한다.

## Architecture

- **단계 1: error contract.** 신규 `client/errors.go`에 sentinel 에러(`ErrRetryable`, `ErrPermanent`, `ErrRateLimited`, `ErrAuthFailed`, `ErrTransport`)와 metadata-carrying typed error(`HTTPError`, `TransportError`, `PingError`)를 export. 기존 unexported 에러(`retryableHTTPError`, `retryableTransportError`, `permanentPingError`)는 새 타입을 wrap하거나 alias로 단계적 전환. `Is`/`Unwrap` 메서드로 `errors.Is`/`errors.As` 모두 지원.
- **단계 2: multipart io.Pipe.** `client/client.go`의 `SendImage`/`SendMultipleImages` 경로에서 `bytes.Buffer`로 multipart body 전체를 메모리에 적재하던 부분을 `io.Pipe` 기반 streaming writer로 전환. 이미지 payload 크기만큼 추가 복사 제거. retry 시에는 idempotent body factory 패턴으로 재시도 가능.
- **단계 3: transport 명확화.** `client/transport.go:66`의 `ForceAttemptHTTP2: true`를 `selectTransport()` 결과와 정합시킴(http1 모드에서는 강제 활성화 의도 명시 또는 제거). transport 선택의 결정 트리를 doc comment로 명문화.

## Tech Stack

Go 1.22+, `net/http`, `mime/multipart`, `io`, `errors` (stdlib `errors.Is`/`errors.As`/`errors.Unwrap`).

## Execution

`subagent-driven-development` 사용. 각 task는 fresh worker subagent에 worktree로 dispatch, 종료 시 review subagent로 contract surface 검증.

## Policy override

이 plan은 사용자님이 직접 작성한 `Iris/docs/refactoring-2026-05-followup.md` Phase G의 *"Go SDK는 v0.x major 변경 없음, internal 정리만 허용"* 정책을 명시적으로 override한다. 사용자 결정(2026-05-22): "외부 contract 마이그레이션 OK". 새 export는 additive(기존 import 호환)이지만, 새 sentinel/typed error를 노출하는 surface change이므로 SDK semver minor bump(v0.x → v0.x+1) 필요.

---

## Success Criteria

1. `errors.Is(err, iris.ErrRetryable)` / `errors.Is(err, iris.ErrAuthFailed)` 등이 모든 client 메서드 반환 에러에 대해 동작.
2. `errors.As(err, &httpErr)` / `errors.As(err, &transportErr)`로 status code, URL 등 metadata 접근 가능.
3. `SendImage`/`SendMultipleImages` 경로에서 `bytes.Buffer.Bytes()` 또는 full-body alloc 패턴 0건(grep). io.Pipe writer 사용 확인.
4. `client/transport.go`에 `selectTransport()` 결정 트리와 `ForceAttemptHTTP2` 의도 doc comment 명시.
5. 기존 consumer(chat-bot-go-kakao, hololive-bot) **컴파일 통과** — 새 export는 additive.
6. `go test ./...` 전부 통과. `go vet`, `golangci-lint run` 깨끗.
7. CHANGELOG.md(없으면 신규 생성)에 sentinel 목록 + minor bump 사유 기록.

## File Map

- **Create:** `client/errors.go` — sentinel + typed errors export, `Is`/`Unwrap` 구현.
- **Create:** `client/errors_test.go` — sentinel 매칭, errors.As 추출, wrapping 동작 검증.
- **Create:** `client/multipart_writer.go` — `io.Pipe` 기반 multipart body factory.
- **Create:** `client/multipart_writer_test.go` — streaming behavior + retry-safe body re-creation 검증.
- **Modify:** `client/client.go:30-40` (existing retryableHTTPError/retryableTransportError 정의 부분) — 새 타입으로 wrap.
- **Modify:** `client/client.go:381` 부근(P2.1 bytes.Buffer 사용 지점) — multipart_writer 호출로 교체.
- **Modify:** `client/ping.go:23` (`permanentPingError`) — 새 `PingError` 타입 alias로 전환, `errors.Is(err, ErrPermanent)` 매칭 추가.
- **Modify:** `client/transport.go:66` — `ForceAttemptHTTP2` 결정 명시 + doc comment.
- **Modify:** `client/transport.go:108` (`errorRoundTripper`) — 그대로 유지(test util), but doc.
- **Create:** `CHANGELOG.md` — v0.x+1 sentinel surface change 기록.
- **Modify:** `iris/iris.go` — 새 sentinel/typed error를 facade에서 re-export.
- **Modify:** `iris/iris_test.go` — facade re-export 검증.

---

## Task 1 — Sentinel errors + HTTPError typed error 도입

**Files:**
- Create: `client/errors.go`
- Create: `client/errors_test.go`
- Modify: `iris/iris.go`

- [ ] **Step 1.1: failing test 작성 (`client/errors_test.go`)**

```go
package client

import (
	"errors"
	"testing"
)

func TestErrRetryable_MatchesWrappedHTTPError(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 503, URL: "http://iris.test/reply"}
	err := wrapHTTPError(httpErr)

	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected errors.Is(err, ErrRetryable) to be true, got false")
	}
	var got *HTTPError
	if !errors.As(err, &got) {
		t.Fatalf("expected errors.As to extract *HTTPError, failed")
	}
	if got.StatusCode != 503 {
		t.Fatalf("StatusCode=%d, want 503", got.StatusCode)
	}
}

func TestErrPermanent_DoesNotMatchRetryable(t *testing.T) {
	httpErr := &HTTPError{StatusCode: 400, URL: "http://iris.test/reply"}
	err := wrapHTTPError(httpErr)

	if errors.Is(err, ErrRetryable) {
		t.Fatalf("400 must not be retryable")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Fatalf("400 must match ErrPermanent")
	}
}

func TestErrAuthFailed_Matches401(t *testing.T) {
	err := wrapHTTPError(&HTTPError{StatusCode: 401, URL: "http://iris.test/reply"})
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("401 must match ErrAuthFailed")
	}
}

func TestErrRateLimited_Matches429(t *testing.T) {
	err := wrapHTTPError(&HTTPError{StatusCode: 429, URL: "http://iris.test/reply"})
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("429 must match ErrRateLimited")
	}
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("429 must also match ErrRetryable (retry with backoff)")
	}
}
```

- [ ] **Step 1.2: run, confirm RED**

Run: `cd /home/kapu/work/iris-stack/iris-client-go && go test ./client/ -run TestErr -v`
Expected: compile error (`ErrRetryable`, `ErrPermanent`, `ErrAuthFailed`, `ErrRateLimited`, `HTTPError`, `wrapHTTPError` 미정의).

- [ ] **Step 1.3: implement `client/errors.go`**

```go
package client

import (
	"errors"
	"fmt"
)

// Sentinel errors for category matching via errors.Is.
var (
	ErrRetryable   = errors.New("iris: retryable error")
	ErrPermanent   = errors.New("iris: permanent error")
	ErrAuthFailed  = errors.New("iris: authentication failed")
	ErrRateLimited = errors.New("iris: rate limited")
	ErrTransport   = errors.New("iris: transport error")
)

// HTTPError carries HTTP-level failure metadata. Use errors.As to extract.
type HTTPError struct {
	StatusCode int
	URL        string
	Body       string // truncated, may be empty
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("iris: HTTP %d from %s", e.StatusCode, e.URL)
}

func (e *HTTPError) Is(target error) bool {
	switch target {
	case ErrRetryable:
		return e.StatusCode >= 500 || e.StatusCode == 429
	case ErrPermanent:
		return e.StatusCode >= 400 && e.StatusCode < 500 && e.StatusCode != 429
	case ErrAuthFailed:
		return e.StatusCode == 401 || e.StatusCode == 403
	case ErrRateLimited:
		return e.StatusCode == 429
	}
	return false
}

// TransportError wraps a transport-layer failure (DNS, dial, TLS).
type TransportError struct {
	Op  string
	URL string
	Err error
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("iris transport %s %s: %v", e.Op, e.URL, e.Err)
}

func (e *TransportError) Unwrap() error { return e.Err }

func (e *TransportError) Is(target error) bool {
	return target == ErrTransport || target == ErrRetryable
}

func wrapHTTPError(e *HTTPError) error { return e }
```

- [ ] **Step 1.4: re-export from facade**

`iris/iris.go`에 추가:
```go
var (
	ErrRetryable   = client.ErrRetryable
	ErrPermanent   = client.ErrPermanent
	ErrAuthFailed  = client.ErrAuthFailed
	ErrRateLimited = client.ErrRateLimited
	ErrTransport   = client.ErrTransport
)

type (
	HTTPError      = client.HTTPError
	TransportError = client.TransportError
)
```

- [ ] **Step 1.5: run, confirm GREEN**

Run: `cd /home/kapu/work/iris-stack/iris-client-go && go test ./client/ -run TestErr -v && go test ./iris/ -v`
Expected: all PASS.

- [ ] **Step 1.6: golangci-lint + go vet**

Run: `cd /home/kapu/work/iris-stack/iris-client-go && golangci-lint run ./... && go vet ./...`
Expected: clean.

---

## Task 2 — 기존 internal 에러를 새 타입으로 wrap

**Files:**
- Modify: `client/client.go` (retryableHTTPError, retryableTransportError 정의 + 사용처)
- Modify: `client/ping.go` (permanentPingError → PingError로 alias)
- Test: 기존 단위 테스트가 동작 보존 검증

- [ ] **Step 2.1: failing test — 기존 client 호출이 새 sentinel과 매칭되는지**

`client/client_errors_integration_test.go` (또는 기존 client_test.go에 추가):
```go
func TestSend_5xxReturnsErrRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(503)
	}))
	defer srv.Close()
	c, _ := NewH2CClient(srv.URL, /* test options */)
	err := c.Send(context.Background(), "room", "msg")
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected ErrRetryable on 503, got %v", err)
	}
}
```

- [ ] **Step 2.2: run, confirm RED**

Expected: 기존 코드가 internal type만 반환하므로 `errors.Is(ErrRetryable)` false.

- [ ] **Step 2.3: client.go의 5xx/429 응답 경로에서 `*HTTPError` 생성 + wrapping**

기존:
```go
return &retryableHTTPError{statusCode: resp.StatusCode, url: req.URL.String()}
```
신규:
```go
return &HTTPError{StatusCode: resp.StatusCode, URL: req.URL.String(), Body: truncateBody(resp.Body)}
```

`retryableHTTPError` 정의는 제거 또는 `type retryableHTTPError = HTTPError` alias로 잠시 유지(같은 PR 안에서 제거 권장).

- [ ] **Step 2.3a: `truncateBody` secret-safe helper 정의 (risk-gate #1)**

`client/errors.go`에 추가:
```go
const httpErrorBodyMaxLen = 512

// truncateBody는 응답 body를 안전하게 잘라 *HTTPError.Body에 저장한다.
// (a) 길이를 512바이트로 cap.
// (b) Authorization, X-Iris-Signature, X-Iris-Secret 류 헤더 echo가 body에 포함될 경우 마스킹.
func truncateBody(r io.Reader) string {
	if r == nil { return "" }
	buf := make([]byte, httpErrorBodyMaxLen)
	n, _ := io.ReadFull(io.LimitReader(r, httpErrorBodyMaxLen), buf)
	body := string(buf[:n])
	return redactSensitiveTokens(body)
}

// redactSensitiveTokens는 token-like 패턴을 ***로 치환한다.
func redactSensitiveTokens(s string) string {
	for _, prefix := range []string{"Bearer ", "Signature=", "X-Iris-Secret:"} {
		s = redactPrefix(s, prefix)
	}
	return s
}
```

failing test 추가:
```go
func TestTruncateBody_RedactsBearerToken(t *testing.T) {
	in := strings.NewReader("error context Bearer abcdef1234567890 trailing")
	got := truncateBody(in)
	if strings.Contains(got, "abcdef1234567890") {
		t.Fatalf("token leaked: %q", got)
	}
}

func TestTruncateBody_Caps512Bytes(t *testing.T) {
	in := strings.NewReader(strings.Repeat("x", 2000))
	got := truncateBody(in)
	if len(got) > 512 {
		t.Fatalf("body length %d > 512", len(got))
	}
}
```

- [ ] **Step 2.4: transport-layer 에러도 `*TransportError`로 wrap**

`client.go`의 dial/connect 실패 경로에서 `&TransportError{Op: "dial", URL: ..., Err: err}` 반환.

- [ ] **Step 2.5: ping.go의 permanentPingError → PingError 타입화**

```go
type PingError struct {
	URL    string
	Reason string
}
func (e *PingError) Error() string { return fmt.Sprintf("iris ping %s: %s", e.URL, e.Reason) }
func (e *PingError) Is(target error) bool { return target == ErrPermanent }
```

기존 `permanentPingError` 호출처 전부 교체.

- [ ] **Step 2.6: run, confirm GREEN**

Run: `cd /home/kapu/work/iris-stack/iris-client-go && go test ./... -count=1`
Expected: 신규 + 기존 테스트 모두 PASS.

---

## Task 3 — 429 retry 경로가 ErrRateLimited를 통과

**Files:**
- Modify: `client/client.go` (P2.4 commit `73dba18` 후속)
- Test: 신규 retry 시나리오

- [ ] **Step 3.1: failing test — 429 응답 시 idempotency key 유지 + ErrRateLimited 분류**

```go
func TestSend_429RetriesAndExposesErrRateLimited(t *testing.T) {
	var attempts int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c, _ := NewH2CClient(srv.URL, /* options w/ maxRetries=2 */)
	err := c.Send(context.Background(), "room", "msg")

	if atomic.LoadInt32(&attempts) < 3 {
		t.Fatalf("expected >=3 attempts (1 + 2 retries), got %d", attempts)
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited after retries exhausted, got %v", err)
	}
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected ErrRateLimited to also be ErrRetryable")
	}
}
```

- [ ] **Step 3.2: run, confirm RED → fix → GREEN**

기존 429 retry 로직 검토, 필요 시 최종 에러에 `*HTTPError` 보존 확인.

---

## Task 4 — Multipart io.Pipe streaming (P2.1 잔여)

**Files:**
- Create: `client/multipart_writer.go`
- Create: `client/multipart_writer_test.go`
- Modify: `client/client.go:381` 부근 (`SendImage`, `SendMultipleImages`)

- [ ] **Step 4.1: failing test — streaming writer가 bytes.Buffer만큼 메모리를 잡지 않음**

```go
func TestMultipartStreaming_DoesNotBufferEntireBody(t *testing.T) {
	largeImage := make([]byte, 20*1024*1024) // 20MB
	// pipe-based writer should expose io.Reader without holding all 20MB in one alloc
	bodyFactory, contentType := newMultipartBodyFactory("image", largeImage, "image/png")

	r, err := bodyFactory()
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	defer r.Close()

	// Read in small chunks; verify reader returns data progressively
	buf := make([]byte, 4096)
	totalRead := 0
	for {
		n, err := r.Read(buf)
		totalRead += n
		if err == io.EOF { break }
		if err != nil { t.Fatalf("read: %v", err) }
	}

	if totalRead < len(largeImage) {
		t.Fatalf("read %d bytes, expected at least %d (image payload)", totalRead, len(largeImage))
	}
	if contentType == "" {
		t.Fatalf("contentType empty")
	}
}

func TestMultipartStreaming_RetrySafeBodyFactory(t *testing.T) {
	image := []byte("png-bytes")
	bodyFactory, _ := newMultipartBodyFactory("image", image, "image/png")

	r1, _ := bodyFactory()
	io.Copy(io.Discard, r1)
	r1.Close()

	r2, err := bodyFactory()
	if err != nil { t.Fatalf("second factory call: %v", err) }
	defer r2.Close()
	n, _ := io.Copy(io.Discard, r2)
	if n == 0 { t.Fatalf("second body returned 0 bytes; factory must be idempotent") }
}
```

- [ ] **Step 4.2: RED 확인**

- [ ] **Step 4.3: implement `client/multipart_writer.go`**

```go
package client

import (
	"io"
	"mime/multipart"
)

// newMultipartBodyFactory returns a factory that creates a fresh io.ReadCloser
// each time it is called, suitable for retry on transport errors.
func newMultipartBodyFactory(fieldName string, payload []byte, contentType string) (func() (io.ReadCloser, error), string) {
	mw := multipart.NewWriter(io.Discard)
	boundary := mw.Boundary()
	ct := mw.FormDataContentType()

	factory := func() (io.ReadCloser, error) {
		pr, pw := io.Pipe()
		writer := multipart.NewWriter(pw)
		_ = writer.SetBoundary(boundary)

		go func() {
			defer pw.Close()
			defer writer.Close()

			part, err := writer.CreateFormFile(fieldName, fieldName+"."+extFromContentType(contentType))
			if err != nil {
				_ = pw.CloseWithError(err)
				return
			}
			if _, err := part.Write(payload); err != nil {
				_ = pw.CloseWithError(err)
				return
			}
		}()
		return pr, nil
	}
	return factory, ct
}

func extFromContentType(ct string) string {
	switch ct {
	case "image/png":
		return "png"
	case "image/jpeg", "image/jpg":
		return "jpg"
	default:
		return "bin"
	}
}
```

- [ ] **Step 4.4: GREEN 확인 + client.go 사용처 교체**

`client.go:381` 부근의 `bytes.Buffer` + manual multipart 작성 코드를 `newMultipartBodyFactory` 호출로 교체. retry 경로에서 factory 재호출.

- [ ] **Step 4.5: integration test — SendImage 호출이 실제로 streaming하는지 검증**

`httptest.Server`로 받아서 `r.ContentLength`와 `r.Body` 직접 read; 동작 검증.

- [ ] **Step 4.6: benchmark — streaming vs buffered baseline (risk-gate #2)**

`client/multipart_writer_bench_test.go` 생성:
```go
func BenchmarkSendImage_BufferedBaseline(b *testing.B) {
	image := bytes.Repeat([]byte{0xFF}, 10*1024*1024) // 10MB
	srv := newTestSink(b)
	defer srv.Close()
	c, _ := NewH2CClient(srv.URL, /* baseline transport */)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = c.SendImage(context.Background(), "room", image)
	}
}

func BenchmarkSendImage_Streaming(b *testing.B) {
	// 동일하나 신규 streaming path 사용
}
```

목표: streaming이 alloc count + peak RSS에서 baseline 대비 50%+ 감소. 회귀 시 P2.1 부분 적용을 별도 PR로 split.

- [ ] **Step 4.7: error log 포맷 회귀 가드 (risk-gate #3)**

`client/log_format_test.go` 생성:
```go
func TestHTTPError_Error_HasMeaningfulMessage(t *testing.T) {
	err := &HTTPError{StatusCode: 503, URL: "http://iris/reply", Body: "upstream down"}
	got := err.Error()
	for _, want := range []string{"503", "iris/reply"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Error() %q missing %q", got, want)
		}
	}
}

func TestHTTPError_SlogValue_IncludesFields(t *testing.T) {
	// slog/json 출력이 StatusCode, URL, Body를 포함하는지 검증.
	// 기존 client.go의 logger.Error(..., slog.Any("error", err)) 호출이 정상 동작 보장.
}
```

---

## Task 5 — transport.go 명확화 (P2.6)

**Files:**
- Modify: `client/transport.go:66` (`ForceAttemptHTTP2: true` 결정 명시)
- Test: `client/transport_test.go` 보강

- [ ] **Step 5.1: failing test — selectTransport 결정 트리 명시**

```go
func TestSelectTransport_HTTP1Mode_DoesNotForceHTTP2(t *testing.T) {
	tr := selectTransport(transportOptions{Mode: "http1"})
	httpTr, ok := tr.(*http.Transport)
	if !ok { t.Fatalf("expected *http.Transport for http1 mode") }
	if httpTr.ForceAttemptHTTP2 {
		t.Fatalf("http1 mode must not set ForceAttemptHTTP2")
	}
}

func TestSelectTransport_HTTP2Mode_ForcesHTTP2(t *testing.T) {
	tr := selectTransport(transportOptions{Mode: "http2"})
	httpTr, ok := tr.(*http.Transport)
	if !ok { t.Fatalf("expected *http.Transport for http2 mode") }
	if !httpTr.ForceAttemptHTTP2 {
		t.Fatalf("http2 mode must set ForceAttemptHTTP2")
	}
}
```

- [ ] **Step 5.2: RED 확인 → transport.go의 `ForceAttemptHTTP2: true` 무조건 적용을 mode 분기로 변경 → GREEN**

- [ ] **Step 5.3: doc comment 추가 — selectTransport 결정 트리**

```go
// selectTransport chooses an http.RoundTripper based on options.Mode:
//   "h3"     → quic-based HTTP/3 transport (http3_transport.go)
//   "h2c"    → HTTP/2 cleartext over TCP (h2c.go)
//   "http2"  → HTTP/2 over TLS (http.Transport with ForceAttemptHTTP2=true)
//   "http1"  → HTTP/1.1 over TLS (http.Transport, ForceAttemptHTTP2=false)
//   default  → "http2"
//
// Callers may override entirely via WithRoundTripper.
func selectTransport(...) http.RoundTripper { ... }
```

---

## Task 6 — CHANGELOG + version bump 준비

**Files:**
- Create: `CHANGELOG.md` (없으면)

- [ ] **Step 6.1:** v0.13.0 entry 작성, sentinel 목록, breaking-but-additive 명시. (사용자 결정 2026-05-22: 현재 latest tag `v0.12.5`에서 minor bump)

```markdown
## [v0.13.0] - 2026-05-22
### Added
- Exported sentinel errors `ErrRetryable`, `ErrPermanent`, `ErrAuthFailed`, `ErrRateLimited`, `ErrTransport` for category matching via `errors.Is`.
- Exported typed errors `HTTPError`, `TransportError`, `PingError` with metadata accessors and `errors.As` support.
- `client/multipart_writer.go`: `io.Pipe`-based multipart body factory; reduces peak RSS during image sends.

### Changed
- `client/transport.go`: `ForceAttemptHTTP2` is now conditional on transport mode (was unconditional).
- Internal `retryableHTTPError`, `retryableTransportError`, `permanentPingError` replaced with exported equivalents. Old types remain as aliases for one minor version, then removed.

### Notes
- Supersedes `Iris/docs/refactoring-2026-05-followup.md` Phase G policy ("public API symbol 유지"). User decision 2026-05-22.
```

---

## Validation

```bash
cd /home/kapu/work/iris-stack/iris-client-go
go test ./... -count=1 -race
go vet ./...
golangci-lint run ./...
```

Expected: all PASS, no new warnings.

**Consumer compile smoke test** (Plan B 시작 전 사전 확인):
```bash
cd /home/kapu/work/iris-stack/chat-bot-go-kakao && go build ./...
cd /home/kapu/work/iris-stack/hololive-bot && go build ./...
```

Expected: 두 consumer 모두 컴파일 통과 (additive change이므로).

## Stop Rules

- **Consumer 컴파일 실패:** 새 export가 기존 alias와 충돌하면 즉시 중단. alias 명세 재조정.
- **기존 단위 테스트 회귀:** Task 2의 internal→external 전환에서 기존 `_test.go`가 깨지면 transition을 1단계(alias 유지) → 2단계(alias 제거)로 분리.
- **Multipart streaming에서 retry 동작 변경:** Task 4 후 retry-on-transport-error 동작이 변경되면(예: 두 번째 호출에서 body empty), factory pattern을 재검토.
- **Transport mode 변경이 production behavior에 영향:** Task 5의 `ForceAttemptHTTP2` 분기가 실제 deployment에서 사용되는 mode와 mismatch면 분기 조건 재확인.

## Risk Gates (Plan A 단독 영향)

| Gate | Trigger | Mitigation |
|---|---|---|
| **API contract / schema** | Sentinel + typed error export | additive, semver minor. Old internal types alias로 1버전 유지. |
| **Performance** | io.Pipe streaming 전환 | 단위 + integration test로 throughput/latency 비교. 회귀 시 P2.1 잔여만 별도 PR로 split. |
| **Transport behavior** | ForceAttemptHTTP2 분기 변경 | http1 모드 실사용 여부 사전 확인. 사용처 없으면 안전. |

Plan B(consumer migration)와 cross-cutting risk는 INDEX 문서에서 별도 정리.
