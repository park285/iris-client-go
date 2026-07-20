package webhook

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
	if len(calls) != 1 || calls[0] != fmt.Sprintf("iris:msg:{%s}", messageID) {
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
