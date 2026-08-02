package transport

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTruncateBodyDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 256<<10))}
	_ = truncateBody(reader)

	maxRead := int64(httpErrorBodyParseMaxLen + httpErrorBodyDrainMaxLen)
	if reader.bytesRead > maxRead {
		t.Fatalf("truncateBody read %d bytes, want at most %d", reader.bytesRead, maxRead)
	}
}

func TestDoRequestSuccessDrainsAtMostBoundedBytes(t *testing.T) {
	t.Parallel()

	reader := &countingReader{Reader: strings.NewReader(strings.Repeat("x", 4<<20))}
	c := NewH2CClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)}, nil
	})))

	if err := c.SendMessage(t.Context(), "room", "hello"); err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}

	if reader.bytesRead > successBodyDrainMaxLen {
		t.Fatalf("success drain read %d bytes, want at most %d", reader.bytesRead, int64(successBodyDrainMaxLen))
	}
	if reader.bytesRead == 0 {
		t.Fatal("success drain read 0 bytes; keep-alive reuse needs a best-effort drain")
	}
}

func TestDoRequestDrainsTrailerAfterDecode(t *testing.T) {
	t.Parallel()

	body := `{"requestId":"req-1"}` + strings.Repeat("\n", 64)
	reader := &countingReader{Reader: strings.NewReader(body)}
	c := NewH2CClient("https://iris.test", "token", WithRoundTripper(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(reader)}, nil
	})))

	if _, err := c.SendMessageAccepted(t.Context(), "room", "hello"); err != nil {
		t.Fatalf("SendMessageAccepted() error = %v", err)
	}

	if reader.bytesRead != int64(len(body)) {
		t.Fatalf("decoded response read %d of %d bytes; the trailer must be drained to reach EOF", reader.bytesRead, len(body))
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
