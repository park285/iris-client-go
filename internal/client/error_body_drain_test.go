package client

import (
	"io"
	"strings"
	"testing"
)

func TestTruncateBodyDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 256<<10))}
	_ = truncateBody(reader)

	maxRead := int64(httpErrorBodyMaxLen + httpErrorBodyDrainMaxLen)
	if reader.bytesRead > maxRead {
		t.Fatalf("truncateBody read %d bytes, want at most %d", reader.bytesRead, maxRead)
	}
}

type countingReader struct {
	*strings.Reader
	bytesRead int64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	r.bytesRead += int64(n)
	if err == io.EOF {
		return n, err
	}
	return n, err
}
