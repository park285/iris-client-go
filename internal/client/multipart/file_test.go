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
		{name: "nil reader", fileName: testFileName, contentType: testContentTypePlain, byteLength: 1},
		{name: "empty", fileName: testFileName, contentType: testContentTypePlain, byteLength: 0, reader: bytes.NewReader(nil)},
		{name: "too large", fileName: testFileName, contentType: testContentTypePlain, byteLength: maxReplySingleFileBytes + 1, reader: bytes.NewReader(nil)},
		{name: "unicode control", fileName: "a\u0085.txt", contentType: testContentTypePlain, byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "path separator", fileName: "../a.txt", contentType: testContentTypePlain, byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "header quote", fileName: `a".txt`, contentType: testContentTypePlain, byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "header semicolon", fileName: "a;.txt", contentType: testContentTypePlain, byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "mime parameter", fileName: testFileName, contentType: "text/plain; charset=utf-8", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
		{name: "mime missing subtype", fileName: testFileName, contentType: "text", byteLength: 1, reader: bytes.NewReader([]byte("a"))},
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

	_, err := DigestReaderAt(t.Context(), bytes.NewReader([]byte("short")), 10)
	if err == nil || !strings.Contains(err.Error(), "short source") {
		t.Fatalf("DigestReaderAt() error = %v, want short source", err)
	}
}

func TestDigestReaderAtHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := DigestReaderAt(ctx, bytes.NewReader(bytes.Repeat([]byte("x"), fileCopyBufferBytes+1)), fileCopyBufferBytes+1)
	if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
		t.Fatalf("DigestReaderAt() error = %v, want context canceled", err)
	}
}

func TestFileBodyFactoryBuildsDeterministicRetryBody(t *testing.T) {
	t.Parallel()

	const (
		boundary    = "iris-file-test-boundary"
		fileName    = "분기 보고서.PDF"
		contentType = "application/pdf"
	)

	payload := []byte("file-payload")
	metadata := []byte(`{"type":"file","room":"room-a"}`)

	factory, err := NewFileBodyFactory(
		t.Context(),
		boundary,
		metadata,
		fileName,
		contentType,
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

	assertFileFactoryBodyDigest(t, factory, first)

	reader := stdmultipart.NewReader(bytes.NewReader(first), boundary)

	assertFileFactoryMetadataPart(t, reader, metadata)
	assertFileFactoryFilePart(t, reader, fileName, contentType, payload)

	if _, err := reader.NextPart(); err != io.EOF {
		t.Fatalf("NextPart(after file) error = %v, want EOF", err)
	}
}

func assertFileFactoryBodyDigest(t *testing.T, factory *FileBodyFactory, body []byte) {
	t.Helper()

	if int64(len(body)) != factory.BodyLength() {
		t.Fatalf("body length = %d, want %d", len(body), factory.BodyLength())
	}

	hash := sha256.Sum256(body)
	if got := hex.EncodeToString(hash[:]); got != factory.BodySHA256() {
		t.Fatalf("body hash = %s, want %s", got, factory.BodySHA256())
	}
}

func assertFileFactoryMetadataPart(t *testing.T, reader *stdmultipart.Reader, metadata []byte) {
	t.Helper()

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
}

func assertFileFactoryFilePart(t *testing.T, reader *stdmultipart.Reader, fileName, contentType string, payload []byte) {
	t.Helper()

	filePart, err := reader.NextPart()
	if err != nil {
		t.Fatalf("NextPart(file) error = %v", err)
	}

	if filePart.FormName() != "file" || filePart.FileName() != fileName {
		t.Fatalf("file part = form %q filename %q", filePart.FormName(), filePart.FileName())
	}

	if got := filePart.Header.Get("Content-Type"); got != contentType {
		t.Fatalf("file Content-Type = %q", got)
	}

	gotPayload, err := io.ReadAll(filePart)
	if err != nil {
		t.Fatalf("ReadAll(file) error = %v", err)
	}

	if !bytes.Equal(gotPayload, payload) {
		t.Fatalf("file payload = %q, want %q", gotPayload, payload)
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
