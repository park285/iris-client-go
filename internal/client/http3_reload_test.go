package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func reloadTestCAPEM(t *testing.T, cn string) []byte {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func reloadTestPool(t *testing.T, pemBytes []byte) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemBytes) {
		t.Fatalf("append certs failed")
	}
	return pool
}

func TestNewHTTP3TransportFromCABuildsPoolFromBytes(t *testing.T) {
	t.Parallel()

	caPEM := reloadTestCAPEM(t, "iris-ca-1")
	rt, err := newHTTP3TransportFromCA(clientOptions{}, true, caPEM)
	if err != nil {
		t.Fatalf("newHTTP3TransportFromCA: %v", err)
	}
	if rt.TLSClientConfig == nil || rt.TLSClientConfig.RootCAs == nil {
		t.Fatalf("RootCAs not set from CA bytes")
	}
	if !rt.TLSClientConfig.RootCAs.Equal(reloadTestPool(t, caPEM)) {
		t.Fatalf("RootCAs does not match provided CA bytes")
	}

	systemRoots, err := newHTTP3TransportFromCA(clientOptions{h3AllowSystemRoots: true}, false, nil)
	if err != nil {
		t.Fatalf("newHTTP3TransportFromCA(allow system roots): %v", err)
	}
	if systemRoots.TLSClientConfig.RootCAs != nil {
		t.Fatalf("RootCAs should be nil (system roots) when explicitly opted in")
	}
}

func TestReloadingH3TransportSwapsOnCAChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	v1 := reloadTestCAPEM(t, "iris-ca-v1")
	if err := os.WriteFile(caFile, v1, 0o600); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	opts := clientOptions{h3CACertFile: caFile}
	initial, err := newHTTP3TransportFromCA(opts, true, v1)
	if err != nil {
		t.Fatalf("initial transport: %v", err)
	}
	reloader := newReloadingH3Transport(initial, opts, caFile, 10*time.Millisecond, v1)
	t.Cleanup(func() { _ = reloader.Close() })

	if reloader.current.Load() != initial {
		t.Fatalf("reloader did not start on initial transport")
	}

	v2 := reloadTestCAPEM(t, "iris-ca-v2")
	if err := os.WriteFile(caFile, v2, 0o600); err != nil {
		t.Fatalf("rewrite v2: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	var swapped *http3.Transport
	for time.Now().Before(deadline) {
		if cur := reloader.current.Load(); cur != initial {
			swapped = cur
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if swapped == nil {
		t.Fatalf("transport was not swapped after CA rotation")
	}
	if !swapped.TLSClientConfig.RootCAs.Equal(reloadTestPool(t, v2)) {
		t.Fatalf("swapped transport does not trust the rotated CA")
	}
}

func TestReloadingH3TransportPreservesDialGuardOnCAChange(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	v1 := reloadTestCAPEM(t, "iris-ca-v1")
	if err := os.WriteFile(caFile, v1, 0o600); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	blocked := errors.New("blocked h3 egress")
	var gotIP net.IP
	opts := clientOptions{
		h3CACertFile: caFile,
		h3DialGuard: func(ip net.IP) error {
			gotIP = append(net.IP(nil), ip...)

			return blocked
		},
	}
	initial, err := newHTTP3TransportFromCA(opts, true, v1)
	if err != nil {
		t.Fatalf("initial transport: %v", err)
	}
	reloader := newReloadingH3Transport(initial, opts, caFile, time.Hour, v1)
	t.Cleanup(func() { _ = reloader.Close() })

	v2 := reloadTestCAPEM(t, "iris-ca-v2")
	if err := os.WriteFile(caFile, v2, 0o600); err != nil {
		t.Fatalf("rewrite v2: %v", err)
	}
	reloader.reloadIfChanged()

	swapped := reloader.current.Load()
	if swapped == initial {
		t.Fatalf("transport was not swapped after CA rotation")
	}
	if swapped.Dial == nil {
		t.Fatal("swapped transport Dial is nil, want guard-wrapped dial")
	}

	_, err = swapped.Dial(t.Context(), "127.0.0.1:443", &tls.Config{MinVersion: tls.VersionTLS13}, &quic.Config{})
	if !errors.Is(err, ErrH3EgressDenied) {
		t.Fatalf("Dial() error = %v, want ErrH3EgressDenied", err)
	}
	if !errors.Is(err, blocked) {
		t.Fatalf("Dial() error = %v, want %v", err, blocked)
	}
	if !gotIP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("guard IP = %v, want 127.0.0.1", gotIP)
	}
}

func TestReloadingH3TransportKeepsCurrentOnBadCA(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	v1 := reloadTestCAPEM(t, "iris-ca-v1")
	if err := os.WriteFile(caFile, v1, 0o600); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	opts := clientOptions{h3CACertFile: caFile}
	initial, err := newHTTP3TransportFromCA(opts, true, v1)
	if err != nil {
		t.Fatalf("initial transport: %v", err)
	}
	reloader := newReloadingH3Transport(initial, opts, caFile, 10*time.Millisecond, v1)
	t.Cleanup(func() { _ = reloader.Close() })

	if err := os.WriteFile(caFile, []byte("not a pem"), 0o600); err != nil {
		t.Fatalf("write garbage: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	if reloader.current.Load() != initial {
		t.Fatalf("reloader swapped to a broken transport on unparseable CA")
	}
}

func TestSelectTransportH3ReloadDisabledByDefault(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, reloadTestCAPEM(t, "iris-ca"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	opts := applyClientOptions([]ClientOption{WithTransport("h3"), WithH3CACertFile(caFile)})
	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport: %v", err)
	}
	if _, ok := rt.(*http3.Transport); !ok {
		t.Fatalf("default (no reload interval) must return *http3.Transport, got %T", rt)
	}
	if closer != nil {
		_ = closer.Close()
	}
}

func TestSelectTransportH3ReloadEnabledReturnsReloader(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	caFile := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caFile, reloadTestCAPEM(t, "iris-ca"), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	opts := applyClientOptions([]ClientOption{
		WithTransport("h3"),
		WithH3CACertFile(caFile),
		WithH3CACertReloadInterval(20 * time.Millisecond),
	})
	rt, closer, err := selectTransport("https://example.com", opts)
	if err != nil {
		t.Fatalf("selectTransport: %v", err)
	}
	reloader, ok := rt.(*reloadingH3Transport)
	if !ok {
		t.Fatalf("reload-enabled h3 must return *reloadingH3Transport, got %T", rt)
	}
	if closer == nil {
		t.Fatalf("reloader must be returned as the closer")
	}
	if err := reloader.Close(); err != nil {
		t.Fatalf("reloader Close: %v", err)
	}
}

func TestResolveH3CAReloadInterval(t *testing.T) {
	if got := resolveH3CAReloadInterval(clientOptions{h3CAReloadInterval: 5 * time.Second}); got != 5*time.Second {
		t.Fatalf("option value = %v, want 5s", got)
	}

	t.Setenv(envH3CAReloadInterval, "15s")
	if got := resolveH3CAReloadInterval(clientOptions{}); got != 15*time.Second {
		t.Fatalf("env value = %v, want 15s", got)
	}
	// option overrides env
	if got := resolveH3CAReloadInterval(clientOptions{h3CAReloadInterval: 2 * time.Second}); got != 2*time.Second {
		t.Fatalf("option should override env, got %v", got)
	}

	t.Setenv(envH3CAReloadInterval, "garbage")
	if got := resolveH3CAReloadInterval(clientOptions{}); got != 0 {
		t.Fatalf("garbage env must yield 0 (disabled), got %v", got)
	}
}
