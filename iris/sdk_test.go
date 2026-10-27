package iris_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v4/internal/testsupport"
	iris "github.com/park285/iris-client-go/v4/iris"
)

// writeTestCA는 h3 client 생성에 필요한 self-signed CA PEM 파일을 만든다.
func writeTestCA(t *testing.T) string {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "iris-test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write CA: %v", err)
	}

	return path
}

// SDK는 IRIS_* 환경 변수를 읽지 않는다. 값이 있어도 옵션만으로 설정이 정해지고, 시스템 루트 opt-in도
// 없다. 이 테스트는 t.Setenv를 쓰므로 병렬로 돌리지 않는다.
func TestNewClientIgnoresIrisEnvironment(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "http://127.0.0.1:3000")
	t.Setenv("IRIS_BOT_TOKEN", "env-token")
	t.Setenv("IRIS_TRANSPORT", "http1")
	t.Setenv("IRIS_H3_CA_CERT_FILE", writeTestCA(t))
	t.Setenv("IRIS_H3_SERVER_NAME", "env-server")
	t.Setenv("IRIS_H3_ALLOW_SYSTEM_ROOTS", "1")

	if _, err := iris.NewClient(); err == nil || !strings.Contains(err.Error(), "base URL is required") {
		t.Fatalf("NewClient() error = %v, want base URL required despite IRIS_BASE_URL", err)
	}

	if _, err := iris.NewClient(iris.WithBaseURL("https://iris.invalid")); err == nil || !strings.Contains(err.Error(), "bot token is required") {
		t.Fatalf("NewClient() error = %v, want bot token required despite IRIS_BOT_TOKEN", err)
	}

	// IRIS_TRANSPORT=http1이 적용되지 않아 h3 기본값이 남고, env CA·시스템 루트로 넘어가지 않는다.
	_, err := iris.NewClient(iris.WithBaseURL("https://iris.invalid"), iris.WithBotToken("token"))
	if err == nil || !strings.Contains(err.Error(), "H3 CA cert file is required") {
		t.Fatalf("NewClient() error = %v, want h3 CA required despite IRIS_TRANSPORT/IRIS_H3_*", err)
	}
}

func TestNewClientRejectsWhitespaceBotToken(t *testing.T) {
	t.Parallel()

	_, err := iris.NewClient(iris.WithBaseURL("https://iris.invalid"), iris.WithBotToken("   "))
	if err == nil || !strings.Contains(err.Error(), "is required") {
		t.Fatalf("NewClient() error = %v, want bot token required", err)
	}
}

func TestParseTransportFacade(t *testing.T) {
	t.Parallel()

	if got, err := iris.ParseTransport(" H3 "); err != nil || got != iris.TransportH3 {
		t.Fatalf("ParseTransport(H3) = (%q, %v), want h3", got, err)
	}

	if _, err := iris.ParseTransport("quic"); err == nil {
		t.Fatal("ParseTransport(quic) error = nil, want rejection")
	}
}

func TestFacadeExposesMentionOption(t *testing.T) {
	t.Parallel()

	var (
		_ = iris.WithMention(iris.ReplyMention{UserID: 1, Nickname: "tester"})
		_ = iris.WithMention(iris.ReplyMention{UserID: "talk-text-id", Nickname: "tester"})
	)
}

// guard 거부는 요청이 나가기 전에 확정된 실패다. 결과 불명(ErrTransport)도 재시도 대상(ErrRetryable)도
// 아니며 ErrNotSent로 드러난다.
func TestFacadeH3DialGuardDenialIsNotSentAndNotRetried(t *testing.T) {
	t.Parallel()

	blocked := errors.New("blocked h3 egress")

	var attempts atomic.Int32

	client, err := iris.NewClient(
		iris.WithBaseURL("https://localhost:443"),
		iris.WithBotToken("token"),
		iris.WithTransport(iris.TransportH3),
		iris.WithH3CACertFile(writeTestCA(t)),
		iris.WithReplyRetry(2),
		iris.WithH3DialGuard(func(context.Context, net.IP) error {
			attempts.Add(1)

			return blocked
		}),
	)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	_, err = client.SendMessageAccepted(t.Context(), "room", "msg", iris.WithClientRequestID("chatbotgo:log-42:reply-v1"))
	if !errors.Is(err, iris.ErrNotSent) || !errors.Is(err, blocked) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrNotSent wrapping %v", err, blocked)
	}

	if errors.Is(err, iris.ErrTransport) || errors.Is(err, iris.ErrRetryable) {
		t.Fatalf("SendMessageAccepted() error = %v, egress deny must be neither ErrTransport nor ErrRetryable", err)
	}

	if attempts.Load() != 1 {
		t.Fatalf("guard attempts = %d, want 1", attempts.Load())
	}
}
