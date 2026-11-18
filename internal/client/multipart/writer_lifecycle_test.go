package multipart

import (
	"errors"
	"io"
	"testing"
)

func TestMultipartBodySupportsConcurrentReadAndClose(t *testing.T) {
	t.Parallel()

	factory, err := NewBodyFactory("boundary", []byte(`{"type":"image","room":"room"}`), [][]byte{{0x89, 'P', 'N', 'G'}}, []string{"image/png"})
	if err != nil {
		t.Fatal(err)
	}

	body, err := factory.NewBody()
	if err != nil {
		t.Fatal(err)
	}

	closed := make(chan error, 1)

	go func() { closed <- body.Close() }()

	buffer := make([]byte, 1)

	for range 1000 {
		if _, err := body.Read(buffer); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
			t.Fatal(err)
		}
	}

	if err := <-closed; err != nil {
		t.Fatal(err)
	}

	if _, err := body.Read(buffer); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Read after Close error = %v, want closed pipe", err)
	}
}
