package client

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
)

func TestHTTP3ClientPingsLocalServer(t *testing.T) {
	t.Parallel()

	certFile, keyFile := writeLocalhostHTTP3Cert(t)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}

	requests := make(chan *http.Request, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.WriteHeader(http.StatusOK)
	})

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()

	server := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		},
	}
	go func() { _ = server.Serve(udp) }()
	defer server.Close()

	port := udp.LocalAddr().(*net.UDPAddr).Port
	client := NewH2CClient(
		"https://localhost:"+strconv.Itoa(port),
		"token",
		WithTransport("h3"),
		WithH3CACertFile(certFile),
		WithH3ServerName("localhost"),
		WithPingStrategy(PingStrategyReady),
	)
	defer client.Close()

	if client.InitError() != nil {
		t.Fatalf("InitError() = %v", client.InitError())
	}

	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}

	got := <-requests
	if got.ProtoMajor != 3 {
		t.Fatalf("ProtoMajor = %d, want 3", got.ProtoMajor)
	}
}

func TestHTTP3ClientUsesEnvCACertFile(t *testing.T) {
	certFile, keyFile := writeLocalhostHTTP3Cert(t)
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("load cert: %v", err)
	}

	requests := make(chan *http.Request, 1)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r
		w.WriteHeader(http.StatusOK)
	})

	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()

	server := &http3.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			MinVersion:   tls.VersionTLS13,
			Certificates: []tls.Certificate{cert},
		},
	}
	go func() { _ = server.Serve(udp) }()
	defer server.Close()

	t.Setenv("IRIS_TRANSPORT", "h3")
	t.Setenv("IRIS_H3_CA_CERT_FILE", certFile)
	t.Setenv("IRIS_H3_SERVER_NAME", "localhost")

	port := udp.LocalAddr().(*net.UDPAddr).Port
	client := NewH2CClient(
		"https://localhost:"+strconv.Itoa(port),
		"token",
		WithPingStrategy(PingStrategyReady),
	)
	defer client.Close()

	if client.InitError() != nil {
		t.Fatalf("InitError() = %v", client.InitError())
	}

	if !client.Ping(t.Context()) {
		t.Fatal("Ping() = false, want true")
	}

	got := <-requests
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
