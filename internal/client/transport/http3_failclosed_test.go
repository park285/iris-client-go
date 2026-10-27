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

// h3 CA는 필수다. 옵션이 비면 시스템 루트로 넘어가지 않고 생성이 실패한다.
func TestHTTP3MissingCACertFileFailsClosed(t *testing.T) {
	t.Parallel()

	if _, err := newHTTP3Transport(applyClientOptions(nil)); !errors.Is(err, ErrMissingH3CACertFile) {
		t.Fatalf("newHTTP3Transport without CA: err = %v, want ErrMissingH3CACertFile", err)
	}
}
