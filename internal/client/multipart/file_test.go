package multipart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	stdmultipart "mime/multipart"
	"strings"
	"testing"
)

func TestNormalizeReplyFileMatchesIrisContract(t *testing.T) {
	t.Parallel()

	reader := bytes.NewReader([]byte("payload"))
	contentType, err := NormalizeReplyFile("분기 보고서 final.PDF", "Application/PDF", 7, reader)
	if err != nil {
		t.Fatalf("NormalizeReplyFile() error = %v", err)
	}
	if contentType != "application/pdf" {
		t.Fatalf("contentType = %q, want application/pdf", contentType)
	}
}

func TestNormalizeReplyFileRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		fileName    string
		contentType string
		byteLength  int64
		reader      io.ReaderAt
	}{
		{name: "nil reader", fileName: "a.txt", contentType: "text/plain", byteLength: 1},
		{name: "empty", fileName: "a.txt", contentType: "text/plain", byteLength: 0, reader: bytes.NewReader(nil)},
		{name: "too large", fileName: "a.txt", contentType: "text/plain", byteLength: maxReplySingleFileBytes + 1, reader: bytes.NewReader(nil)},
		{name: "unicode control", fileName: "a\u0085.txt", contentType: "text/plain", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "path separator", fileName: "../a.txt", contentType: "text/plain", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "header quote", fileName: `a".txt`, contentType: "text/plain", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "header semicolon", fileName: "a;.txt", contentType: "text/plain", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "mime parameter", fileName: "a.txt", contentType: "text/plain; charset=utf-8", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "mime missing subtype", fileName: "a.txt", contentType: "text", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := NormalizeReplyFile(test.fileName, test.contentType, test.byteLength, test.reader); err == nil {
				t.Fatal("NormalizeReplyFile() error = nil, want validation error")
			}
		})
	}
}

func TestDigestReaderAtRejectsShortSource(t *testing.T) {
	t.Parallel()

	_, err := DigestReaderAt(context.Background(), bytes.NewReader([]byte("short")), 10)
	if err == nil || !strings.Contains(err.Error(), "short source") {
		t.Fatalf("DigestReaderAt() error = %v, want short source", err)
	}
}

func TestDigestReaderAtHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := DigestReaderAt(ctx, bytes.NewReader(bytes.Repeat([]byte("x"), fileCopyBufferBytes+1)), fileCopyBufferBytes+1)
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("DigestReaderAt() error = %v, want context canceled", err)
	}
}

func TestFileBodyFactoryBuildsDeterministicRetryBody(t *testing.T) {
	t.Parallel()

	const boundary = "iris-file-test-boundary"
	payload := []byte("file-payload")
	metadata := []byte(`{"type":"file","room":"room-a"}`)

	factory, err := NewFileBodyFactory(
		context.Background(),
		boundary,
		metadata,
		"분기 보고서.PDF",
		"application/pdf",
		int64(len(payload)),
		bytes.NewReader(payload),
	)
	if err != nil {
		t.Fatalf("NewFileBodyFactory() error = %v", err)
	}

	first := readFileFactoryBody(t, factory)
	second := readFileFactoryBody(t, factory)
	if !bytes.Equal(first, second) {
		t.Fatal("retry bodies differ")
	}
	if int64(len(first)) != factory.BodyLength() {
		t.Fatalf("body length = %d, want %d", len(first), factory.BodyLength())
	}

	hash := sha256.Sum256(first)
	if got := hex.EncodeToString(hash[:]); got != factory.BodySHA256() {
		t.Fatalf("body hash = %s, want %s", got, factory.BodySHA256())
	}

	reader := stdmultipart.NewReader(bytes.NewReader(first), boundary)
	metadataPart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("NextPart(metadata) error = %v", err)
	}
	if metadataPart.FormName() != "metadata" {
		t.Fatalf("metadata form name = %q", metadataPart.FormName())
	}
	gotMetadata, err := io.ReadAll(metadataPart)
	if err != nil {
		t.Fatalf("ReadAll(metadata) error = %v", err)
	}
	if !bytes.Equal(gotMetadata, metadata) {
		t.Fatalf("metadata = %q, want %q", gotMetadata, metadata)
	}

	filePart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("NextPart(file) error = %v", err)
	}
	if filePart.FormName() != "file" || filePart.FileName() != "분기 보고서.PDF" {
		t.Fatalf("file part = form %q filename %q", filePart.FormName(), filePart.FileName())
	}
	if got := filePart.Header.Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("file Content-Type = %q", got)
	}
	gotPayload, err := io.ReadAll(filePart)
	if err != nil {
		t.Fatalf("ReadAll(file) error = %v", err)
	}
	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("file payload = %q, want %q", gotPayload, payload)
	}
	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("NextPart(after file) error = %v, want EOF", err)
	}
}

func readFileFactoryBody(t *testing.T, factory *FileBodyFactory) []byte {
	t.Helper()

	body, err := factory.NewBody()
	if err != nil {
		t.Fatalf("NewBody() error = %v", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll(body) error = %v", err)
	}
	return data
}
