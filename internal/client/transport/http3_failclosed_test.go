package transport

import (
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

func TestIC02HTTP3MissingCAPathRequiresExplicitSystemRootsOptIn_7c3654b4(t *testing.T) {
	t.Setenv(envH3CACertFile, "")
	t.Setenv(envH3AllowSystemRoots, "")

	opts := applyClientOptions(nil)
	if _, err := newHTTP3Transport(opts); !errors.Is(err, ErrMissingH3CACertFile) {
		t.Fatalf("no CA path without opt-in: err = %v, want ErrMissingH3CACertFile", err)
	}

	optedIn := applyClientOptions([]ClientOption{WithH3AllowSystemRoots(true)})
	rt, err := newHTTP3Transport(optedIn)
	if err != nil {
		t.Fatalf("no CA path with WithH3AllowSystemRoots: err = %v, want nil", err)
	}
	if rt.TLSClientConfig.RootCAs != nil {
		t.Fatalf("RootCAs must be nil (system roots) when opted in without a CA file")
	}
}

func TestIC02HTTP3MissingCAPathSystemRootsViaEnv_7c3654b4(t *testing.T) {
	t.Setenv(envH3CACertFile, "")
	t.Setenv(envH3AllowSystemRoots, "1")

	rt, err := newHTTP3Transport(applyClientOptions(nil))
	if err != nil {
		t.Fatalf("IRIS_H3_ALLOW_SYSTEM_ROOTS=1: err = %v, want nil", err)
	}
	if rt.TLSClientConfig.RootCAs != nil {
		t.Fatalf("RootCAs must be nil (system roots) with env opt-in")
	}
}
