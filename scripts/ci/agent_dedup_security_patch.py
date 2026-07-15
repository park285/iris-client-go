from __future__ import annotations

from pathlib import Path

ROOT = Path.cwd()


def replace_once(path: str, old: str, new: str) -> None:
    target = ROOT / path
    text = target.read_text(encoding="utf-8")
    count = text.count(old)
    if count != 1:
        raise RuntimeError(f"{path}: expected one replacement target, found {count}")
    target.write_text(text.replace(old, new, 1), encoding="utf-8")


def write(path: str, content: str) -> None:
    target = ROOT / path
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(content, encoding="utf-8")


replace_once(
    "webhook/handler.go",
    """\tdedupedBeforeDecode, handled := h.handlePreDecodeDedup(w, r)
\tif handled {
\t\treturn
\t}

""",
    "",
)
replace_once(
    "webhook/handler.go",
    """\tif h.admitter == nil && (h.options.DedupMode == DedupModeAfterDecode || !dedupedBeforeDecode) {
""",
    """\tif h.admitter == nil {
""",
)
replace_once(
    "webhook/handler.go",
    """func WithDedupMode(mode DedupMode) HandlerOption {
\treturn func(h *Handler) {
\t\th.options.DedupMode = mode
\t}
}
""",
    """// WithDedupMode is retained for source compatibility. Deduplication always
// happens after authentication, body decoding, request validation, and message
// identity reconciliation. DedupModeBeforeDecode is no longer honored because a
// side-effecting backend could otherwise reserve an authenticated message ID for
// a request that is later rejected.
func WithDedupMode(mode DedupMode) HandlerOption {
\treturn func(h *Handler) {
\t\t_ = mode
\t\th.options.DedupMode = DedupModeAfterDecode
\t}
}
""",
)
replace_once(
    "webhook/handler.go",
    """\tif opts.DedupMode != DedupModeBeforeDecode && opts.DedupMode != DedupModeAfterDecode {
\t\topts.DedupMode = DedupModeAfterDecode
\t}
""",
    """\t// Pre-decode deduplication is intentionally disabled. A SET-NX style
\t// backend must not retain message identity until the body and identity have
\t// both passed validation.
\topts.DedupMode = DedupModeAfterDecode
""",
)
replace_once(
    "webhook/handler_test.go",
    r'''func TestServeHTTPEarlyDedupSkipsBodyDecode(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeBeforeDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := "{invalid-json"
	request := newV2IdentityRequest(t, body, "mid-early")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (dedup should short-circuit before decode)", recorder.Code, http.StatusOK)
	}

	assertMetricCounts(t, metrics, metricCounts{requests: 1, duplicate: 1})
}
''',
    r'''func TestServeHTTPBeforeDecodeModeRejectsMalformedWithoutDedup(t *testing.T) {
	t.Parallel()

	metrics := &mockMetrics{}
	dedup := &mockDeduplicator{duplicate: true}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}

	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithMetrics(metrics),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeBeforeDecode),
	)
	defer closeHandler(t, handler)

	recorder := httptest.NewRecorder()
	body := "{invalid-json"
	request := newV2IdentityRequest(t, body, "mid-early")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("dedup calls = %d, want 0 for rejected body", len(calls))
	}
	assertMetricCounts(t, metrics, metricCounts{requests: 1, badRequest: 1})
}
''',
)

write(
    "webhook/dedup_poisoning_security_test.go",
    r'''package webhook

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/park285/iris-client-go/internal/irishmac"
)

type retainingDeduplicator struct {
	mu    sync.Mutex
	seen  map[string]struct{}
	calls []string
}

func (d *retainingDeduplicator) IsDuplicate(_ context.Context, key string, _ time.Duration) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]struct{})
	}
	d.calls = append(d.calls, key)
	if _, ok := d.seen[key]; ok {
		return true, nil
	}
	d.seen[key] = struct{}{}
	return false, nil
}

func (d *retainingDeduplicator) snapshot() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.calls...)
}

func TestMalformedV2RequestCannotPoisonMessageIDDedup(t *testing.T) {
	t.Parallel()

	dedup := &retainingDeduplicator{}
	capture := &captureHandler{msgCh: make(chan *Message, 1)}
	handler := NewHandler(
		t.Context(),
		"token",
		capture,
		slog.Default(),
		WithDeduplicator(dedup),
		WithNonceCache(newMemoryNonceCache()),
		WithDedupMode(DedupModeBeforeDecode),
	)
	defer closeHandler(t, handler)

	const messageID = "mid-poison-regression"
	malformed := newV2DedupSecurityRequest(t, "{invalid-json", messageID, "poison-attempt")
	malformedRecorder := httptest.NewRecorder()
	handler.ServeHTTP(malformedRecorder, malformed)
	if malformedRecorder.Code != http.StatusBadRequest {
		t.Fatalf("malformed status = %d, want %d", malformedRecorder.Code, http.StatusBadRequest)
	}
	if calls := dedup.snapshot(); len(calls) != 0 {
		t.Fatalf("malformed request retained dedup identity: %#v", calls)
	}

	validBody := validJSONBodyWithMessageID(messageID)
	valid := newV2DedupSecurityRequest(t, validBody, messageID, "valid-delivery")
	validRecorder := httptest.NewRecorder()
	handler.ServeHTTP(validRecorder, valid)
	if validRecorder.Code != http.StatusOK {
		t.Fatalf("valid status = %d, want %d", validRecorder.Code, http.StatusOK)
	}
	select {
	case msg := <-capture.msgCh:
		if msg == nil || msg.JSON == nil || msg.JSON.MessageID != messageID {
			t.Fatalf("delivered message = %#v, want message ID %q", msg, messageID)
		}
	case <-time.After(time.Second):
		t.Fatal("valid delivery was not enqueued")
	}
	calls := dedup.snapshot()
	if len(calls) != 1 || calls[0] != fmt.Sprintf("iris:msg:%s", messageID) {
		t.Fatalf("dedup calls = %#v, want one validated message identity", calls)
	}
}

func newV2DedupSecurityRequest(t *testing.T, body, messageID, nonce string) *http.Request {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, PathWebhook, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(HeaderIrisMessageID, messageID)
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	bodySHA256 := irishmac.SHA256HexBytes([]byte(body))
	target, err := irishmac.CanonicalTarget(request.URL.RequestURI())
	if err != nil {
		t.Fatalf("CanonicalTarget() error = %v", err)
	}
	canonical := canonicalWebhookRequestV2(request.Method, target, timestamp, nonce, messageID, bodySHA256)
	request.Header.Set(HeaderIrisSignatureVersion, SignatureVersionV2)
	request.Header.Set(HeaderIrisTimestamp, timestamp)
	request.Header.Set(HeaderIrisNonce, nonce)
	request.Header.Set(HeaderIrisBodySHA256, bodySHA256)
	request.Header.Set(HeaderIrisSignature, irishmac.NewSigner("token").Sign(canonical))
	return request
}
''',
)

(ROOT / ".github/workflows/agent-dedup-security-patch.yml").unlink()
Path(__file__).unlink()
