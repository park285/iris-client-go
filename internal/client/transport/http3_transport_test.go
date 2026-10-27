package transport

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"

	"github.com/park285/iris-client-go/v4/internal/testsupport"
)

func TestHTTP3DialGuardRejectsResolvedIPs(t *testing.T) {
	tests := []struct {
		name    string
		addr    string
		matches func(net.IP) bool
	}{
		{
			name: "ipv4 literal",
			addr: "127.0.0.1:443",
			matches: func(ip net.IP) bool {
				return ip.Equal(net.ParseIP("127.0.0.1"))
			},
		},
		{
			name: "ipv6 literal",
			addr: "[::1]:443",
			matches: func(ip net.IP) bool {
				return ip.Equal(net.ParseIP("::1"))
			},
		},
		{
			name: "hostname",
			addr: "localhost:443",
			matches: func(ip net.IP) bool {
				return ip.IsLoopback()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked := errors.New("blocked h3 egress")

			var gotIP net.IP

			opts := applyClientOptions([]ClientOption{withTestH3CA(t)})

			opts.h3DialGuard = func(_ context.Context, ip net.IP) error {
				gotIP = append(net.IP(nil), ip...)

				return blocked
			}

			rt, err := newHTTP3Transport(opts)
			if err != nil {
				t.Fatalf("newHTTP3Transport() error = %v", err)
			}

			if rt.Dial == nil {
				t.Fatal("Dial is nil, want guard-wrapped dial")
			}

			_, err = rt.Dial(t.Context(), tt.addr, &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
			if !errors.Is(err, ErrH3EgressDenied) {
				t.Fatalf("Dial() error = %v, want ErrH3EgressDenied", err)
			}

			if !errors.Is(err, blocked) {
				t.Fatalf("Dial() error = %v, want %v", err, blocked)
			}

			if !tt.matches(gotIP) {
				t.Fatalf("guard IP = %v, want match for %s", gotIP, tt.addr)
			}
		})
	}
}

func TestHTTP3DialGuardResolveHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var called bool

	dial := guardedH3DialContext(func(context.Context, net.IP) error {
		called = true

		return nil
	})

	_, err := dial(ctx, "localhost:443", &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Dial() error = %v, want context.Canceled", err)
	}

	if called {
		t.Fatal("guard was called after canceled resolve")
	}
}

type localHTTP3Server struct {
	certFile string
	port     int
	requests <-chan *http.Request
}

func startLocalHTTP3Server(t *testing.T) localHTTP3Server {
	t.Helper()

	requests := make(chan *http.Request, 1)
	server := startLocalHTTP3ServerWithHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r

		w.WriteHeader(http.StatusOK)

		testsupport.WriteResponse(t, w, "{}")
	}))

	server.requests = requests

	return server
}

func startLocalHTTP3ServerWithHandler(t *testing.T, handler http.Handler) localHTTP3Server {
	t.Helper()

	certFile, keyFile := writeLocalhostHTTP3Cert(t)

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}

	udp, err := (&net.ListenConfig{}).ListenPacket(t.Context(), "udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	testsupport.CloseOnCleanup(t, "udp.Close", udp.Close)

	server := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		},
	}

	serveErr := make(chan error, 1)

	go func() { serveErr <- server.Serve(udp) }()

	t.Cleanup(func() {
		testsupport.CloseNow(t, "server.Close", server.Close)

		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("Serve() error = %v", err)
		}
	})

	return localHTTP3Server{
		certFile: certFile,
		port:     testsupport.AssertType[*net.UDPAddr](t, "udp.LocalAddr()", udp.LocalAddr()).Port,
	}
}

// 동기 drain은 HTTP/3 본문 Read가 요청 context 취소로 풀린다는 전제에 기댄다. 실제 quic-go
// 스트림에서 그 전제와 시간 상한, ErrResponseDrainTimeout 분류를 확인한다.
func TestHTTP3DecodedResponseDrainIsTimeBounded(t *testing.T) {
	t.Parallel()

	server := startLocalHTTP3ServerWithHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusOK)

		if _, err := w.Write([]byte(`{"requestId":"req-1"}`)); err != nil {
			t.Errorf("write response: %v", err)

			return
		}

		if err := http.NewResponseController(w).Flush(); err != nil {
			t.Errorf("flush response: %v", err)

			return
		}

		// trailer를 보내지 않고 스트림을 열어 둬 client의 trailer drain을 멈춰 세운다.
		<-r.Context().Done()
	}))

	client := mustNewAPIClient(
		"https://localhost:"+strconv.Itoa(server.port),
		"token",
		WithTransport("h3"),
		WithH3CACertFile(server.certFile),
		WithH3ServerName("localhost"),
	)

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	started := time.Now()

	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")
	if !errors.Is(err, ErrResponseDrainTimeout) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrResponseDrainTimeout", err)
	}

	if !errors.Is(err, ErrTransport) {
		t.Fatalf("SendMessageAccepted() error = %v, want ErrTransport classification", err)
	}

	if elapsed := time.Since(started); elapsed > 10*successBodyDrainTimeout {
		t.Fatalf("HTTP/3 decoded response drain took %s", elapsed)
	}
}

func TestHTTP3ClientPingsLocalServer(t *testing.T) {
	t.Parallel()

	server := startLocalHTTP3Server(t)
	guarded := make(chan net.IP, 1)
	client := mustNewAPIClient(
		"https://localhost:"+strconv.Itoa(server.port),
		"token",
		WithTransport("h3"),
		WithH3CACertFile(server.certFile),
		WithH3ServerName("localhost"),
		WithH3DialGuard(func(_ context.Context, ip net.IP) error {
			guarded <- append(net.IP(nil), ip...)

			if !ip.IsLoopback() {
				return errors.New("blocked non-loopback h3 egress")
			}

			return nil
		}),
	)

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	if err := client.Ping(t.Context()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	guardedIP := <-guarded
	if !guardedIP.IsLoopback() {
		t.Fatalf("guard IP = %v, want loopback", guardedIP)
	}

	got := <-server.requests
	if got.ProtoMajor != 3 {
		t.Fatalf("ProtoMajor = %d, want 3", got.ProtoMajor)
	}
}

func writeLocalhostHTTP3Cert(t *testing.T) (string, string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	dir := t.TempDir()
	certFile := filepath.Join(dir, "localhost.crt")
	keyFile := filepath.Join(dir, "localhost.key")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}

	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}

	return certFile, keyFile
}

func TestNewHTTP3TransportQUICConfigMatchesSharedProfile(t *testing.T) {
	transport, err := newHTTP3Transport(applyClientOptions([]ClientOption{withTestH3CA(t)}))
	if err != nil {
		t.Fatalf("newHTTP3Transport() error = %v", err)
	}

	if transport.QUICConfig == nil {
		t.Fatal("QUICConfig = nil")
	}

	if got := transport.QUICConfig.InitialPacketSize; got != h3InitialPacketSize {
		t.Fatalf("InitialPacketSize = %d, want %d", got, h3InitialPacketSize)
	}

	if got := transport.QUICConfig.HandshakeIdleTimeout; got != h3HandshakeIdleTimeout {
		t.Fatalf("HandshakeIdleTimeout = %s, want %s; an unset handshake bound leaves dials on the quic-go default", got, h3HandshakeIdleTimeout)
	}

	if got := transport.QUICConfig.KeepAlivePeriod; got != h3KeepAlivePeriod {
		t.Fatalf("KeepAlivePeriod = %s, want %s", got, h3KeepAlivePeriod)
	}

	if got := transport.QUICConfig.MaxIdleTimeout; got != h3MaxIdleTimeout {
		t.Fatalf("MaxIdleTimeout = %s, want %s", got, h3MaxIdleTimeout)
	}
}

// withTestH3CA는 SDK 소유 h3 transport를 만드는 테스트에 신뢰 CA 파일을 넘긴다.
func withTestH3CA(t *testing.T) ClientOption {
	t.Helper()

	certFile, _ := writeLocalhostHTTP3Cert(t)

	return WithH3CACertFile(certFile)
}
