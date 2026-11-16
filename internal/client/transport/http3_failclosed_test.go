package transport

import (
	"bytes"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestIC02HTTP3EmptyCACertFileFailsClosed_7c3654b4(t *testing.T) {
	dir := t.TempDir()
	caFile := filepath.Join(dir, "empty-ca.pem")

	if err := os.WriteFile(caFile, []byte{}, 0o600); err != nil {
		t.Fatalf("write empty ca: %v", err)
	}

	opts := applyClientOptions([]ClientOption{WithH3CACertFile(caFile)})
	_, err := newHTTP3Transport(opts)

	if !errors.Is(err, ErrEmptyH3CACertFile) {
		t.Fatalf("newHTTP3Transport with empty CA file: err = %v, want ErrEmptyH3CACertFile", err)
	}
}

func TestHTTP3MissingCACertFileFailsClosed(t *testing.T) {
	t.Parallel()

	if _, err := newHTTP3Transport(applyClientOptions(nil)); !errors.Is(err, ErrMissingH3CACertFile) {
		t.Fatalf("newHTTP3Transport without CA: err = %v, want ErrMissingH3CACertFile", err)
	}
}

func TestHTTP3CACertFileAcceptsOnlyCertificateBlocks(t *testing.T) {
	t.Parallel()

	firstFile, _ := writeLocalhostHTTP3Cert(t)
	secondFile, _ := writeLocalhostHTTP3Cert(t)
	first := readTestFile(t, firstFile)
	second := readTestFile(t, secondFile)
	opts := applyClientOptions(nil)

	bundle := bytes.Join([][]byte{[]byte("\n"), first, []byte("\n\n"), second, []byte("\n")}, nil)
	if _, err := newHTTP3TransportFromCA(opts, bundle); err != nil {
		t.Fatalf("newHTTP3TransportFromCA(two certificates) error = %v, want nil", err)
	}

	withHeader, _ := pem.Decode(first)
	if withHeader == nil {
		t.Fatal("test certificate PEM block is missing")
	}

	withHeader.Headers = map[string]string{"Proc-Type": "4,ENCRYPTED"}

	tests := []struct {
		name string
		pem  []byte
	}{
		{name: "leading data", pem: append([]byte("leading\n"), first...)},
		{name: "trailing data", pem: append(append([]byte(nil), first...), []byte("trailing")...)},
		{name: "non certificate block", pem: append(append([]byte(nil), first...), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("invalid")})...)},
		{name: "certificate block with headers", pem: pem.EncodeToMemory(withHeader)},
		{name: "invalid certificate", pem: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte("invalid")})},
		{name: "malformed block", pem: []byte("-----BEGIN CERTIFICATE-----\nnot base64\n")},
		{name: "whitespace only", pem: []byte(" \n\t\n")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if transport, err := newHTTP3TransportFromCA(opts, tc.pem); err == nil {
				t.Fatalf("newHTTP3TransportFromCA() = %v, want error", transport)
			}
		})
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // 테스트가 만든 임시 파일만 읽는다.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return data
}
