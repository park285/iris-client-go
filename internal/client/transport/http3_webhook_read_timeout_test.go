package transport

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"

	"github.com/park285/iris-client-go/v3/internal/testsupport"
	"github.com/park285/iris-client-go/v3/webhook"
	"github.com/park285/iris-client-go/v3/webhooksign"
)

type h3WebhookNonceStore struct {
	nonces sync.Map
}

func (s *h3WebhookNonceStore) IsDuplicate(_ context.Context, key string, _ time.Duration) (bool, error) {
	_, duplicate := s.nonces.LoadOrStore(key, struct{}{})

	return duplicate, nil
}

func (*h3WebhookNonceStore) SetOnceNonce() {}

type h3WebhookAdmitter struct {
	calls atomic.Int32
}

func (a *h3WebhookAdmitter) AdmitMessage(context.Context, *webhook.Message) error {
	a.calls.Add(1)

	return nil
}

func TestHTTP3WebhookBodyReadTimeoutAllowsFreshSignedDelivery(t *testing.T) {
	t.Parallel()

	secret := rand.Text()
	admitter := &h3WebhookAdmitter{}

	handler, err := webhook.NewDurableHandler(t.Context(), secret, admitter, slog.New(slog.DiscardHandler), webhook.WithNonceStore(&h3WebhookNonceStore{}))
	if err != nil {
		t.Fatal(err)
	}

	testsupport.CloseOnCleanup(t, "handler.Close", handler.Close)

	var requests atomic.Int32

	client, endpoint := startReadTimeoutWebhookHTTP3Server(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 3 {
			t.Errorf("request protocol = %s, want HTTP/3", r.Proto)
		}

		if requests.Add(1) == 1 {
			// An expired read budget models ingress delay before the SDK reads an otherwise valid body.
			if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
				t.Errorf("set HTTP/3 read deadline: %v", err)
			}
		}

		handler.ServeHTTP(w, r)
	}))

	if status := deliverSignedWebhookHTTP3(t, client, endpoint, secret); status != http.StatusRequestTimeout {
		t.Fatalf("first status = %d, want retryable %d", status, http.StatusRequestTimeout)
	}

	if calls := admitter.calls.Load(); calls != 0 {
		t.Fatalf("timed-out admissions = %d, want 0", calls)
	}

	if status := deliverSignedWebhookHTTP3(t, client, endpoint, secret); status != http.StatusOK {
		t.Fatalf("fresh signed delivery status = %d, want %d", status, http.StatusOK)
	}

	if calls := admitter.calls.Load(); calls != 1 {
		t.Fatalf("admissions after fresh signed delivery = %d, want 1", calls)
	}
}

func startReadTimeoutWebhookHTTP3Server(t *testing.T, handler http.Handler) (*http.Client, string) {
	t.Helper()

	certFile, keyFile := writeLocalhostHTTP3Cert(t)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatal(err)
	}

	udp, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	testsupport.CloseOnCleanup(t, "udp.Close", udp.Close)

	server := &http3.Server{Handler: handler, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}}}
	serveDone := make(chan error, 1)

	go func() { serveDone <- server.Serve(udp) }()

	t.Cleanup(func() {
		testsupport.CloseNow(t, "server.Close", server.Close)

		if serveErr := <-serveDone; serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			t.Errorf("HTTP/3 Serve() error = %v", serveErr)
		}
	})

	ca, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca)

	transport := &http3.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
	testsupport.CloseOnCleanup(t, "transport.Close", transport.Close)

	return &http.Client{Transport: transport, Timeout: 10 * time.Second}, "https://" + udp.LocalAddr().String() + "/webhook/iris"
}

func deliverSignedWebhookHTTP3(t *testing.T, client *http.Client, endpoint, secret string) int {
	t.Helper()

	const body = `{"messageId":"h3-deadline-message","text":"hello","room":"1","userId":"2"}`

	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(webhook.HeaderIrisMessageID, "h3-deadline-message")

	if signErr := webhooksign.SignRequest(req, secret, []byte(body)); signErr != nil {
		t.Fatal(signErr)
	}

	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			t.Errorf("response.Body.Close() error = %v", closeErr)
		}
	}()

	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatal(err)
	}

	return response.StatusCode
}
