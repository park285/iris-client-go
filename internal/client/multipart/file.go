package multipart

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

const (
	maxReplySingleFileBytes = 30 * 1024 * 1024
	maxReplyFileNameBytes   = 255
	maxReplyFileMIMEBytes   = 127
	fileCopyBufferBytes     = 32 * 1024
)

var fileCopyBufferPool = sync.Pool{
	New: func() any { return new([fileCopyBufferBytes]byte) },
}

// NormalizeReplyFile validates the client-side representation against the Iris
// multipart file-reply contract. The file name is preserved byte-for-byte while
// the MIME type is canonicalized to lower case.
func NormalizeReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) (string, error) {
	if readerAt == nil {
		return "", errors.New("iris: file reader is nil")
	}
	if byteLength <= 0 {
		return "", errors.New("iris: file payload is empty")
	}
	if byteLength > maxReplySingleFileBytes {
		return "", fmt.Errorf("iris: file payload exceeds %d bytes", maxReplySingleFileBytes)
	}
	if err := validateReplyFileName(fileName); err != nil {
		return "", err
	}

	normalizedContentType, err := normalizeReplyFileContentType(contentType)
	if err != nil {
		return "", err
	}

	return normalizedContentType, nil
}

func validateReplyFileName(fileName string) error {
	if fileName == "" || fileName == "." || fileName == ".." {
		return errors.New("iris: invalid file name")
	}
	if !utf8.ValidString(fileName) || len(fileName) > maxReplyFileNameBytes {
		return errors.New("iris: invalid file name")
	}
	for _, ch := range fileName {
		if unicode.IsControl(ch) || strings.ContainsRune(`/\";`, ch) {
			return errors.New("iris: invalid file name")
		}
	}

	return nil
}

func normalizeReplyFileContentType(contentType string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(contentType))
	if normalized == "" || len(normalized) > maxReplyFileMIMEBytes || strings.Contains(normalized, ";") {
		return "", fmt.Errorf("iris: invalid file content type %q", contentType)
	}

	topLevel, subtype, ok := strings.Cut(normalized, "/")
	if !ok || topLevel == "" || subtype == "" || strings.Contains(subtype, "/") {
		return "", fmt.Errorf("iris: invalid file content type %q", contentType)
	}
	if !isMIMEToken(topLevel) || !isMIMEToken(subtype) {
		return "", fmt.Errorf("iris: invalid file content type %q", contentType)
	}

	return normalized, nil
}

func isMIMEToken(value string) bool {
	for i := 0; i < len(value); i++ {
		if !isMIMETokenByte(value[i]) {
			return false
		}
	}
	return true
}

func isMIMETokenByte(value byte) bool {
	if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' {
		return true
	}

	switch value {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	default:
		return false
	}
}

// DigestReaderAt computes the exact file digest without taking ownership of the
// supplied reader. A short or unstable source fails before any network request.
func DigestReaderAt(ctx context.Context, readerAt io.ReaderAt, byteLength int64) (string, error) {
	hash := sha256.New()
	if err := copyReaderAtContext(ctx, hash, readerAt, byteLength); err != nil {
		return "", fmt.Errorf("digest reply file: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

type FileBodyFactory struct {
	contentType string
	bodySHA256  string
	bodyLength  int64
	prefix      []byte
	suffix      []byte
	readerAt    io.ReaderAt
	fileLength  int64
}

func NewFileBodyFactory(
	ctx context.Context,
	boundary string,
	metadataBytes []byte,
	fileName string,
	contentType string,
	byteLength int64,
	readerAt io.ReaderAt,
) (*FileBodyFactory, error) {
	prefix := fmt.Appendf(nil, "--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n", boundary)
	prefix = append(prefix, metadataBytes...)
	prefix = fmt.Appendf(
		prefix,
		"\r\n--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: %s\r\n\r\n",
		boundary,
		fileName,
		contentType,
	)
	suffix := fmt.Appendf(nil, "\r\n--%s--\r\n", boundary)

	bodyLength, err := checkedMultipartLength(int64(len(prefix)), byteLength, int64(len(suffix)))
	if err != nil {
		return nil, err
	}
	if err := ValidateReplyMultipartEnvelope(metadataBytes, bodyLength); err != nil {
		return nil, err
	}
	if readerAt == nil {
		return nil, errors.New("iris: file reader is nil")
	}

	bodyHash := sha256.New()
	_, _ = bodyHash.Write(prefix)
	if err := copyReaderAtContext(ctx, bodyHash, readerAt, byteLength); err != nil {
		return nil, fmt.Errorf("hash multipart file body: %w", err)
	}
	_, _ = bodyHash.Write(suffix)

	return &FileBodyFactory{
		contentType: "multipart/form-data; boundary=" + boundary,
		bodySHA256:  hex.EncodeToString(bodyHash.Sum(nil)),
		bodyLength:  bodyLength,
		prefix:      prefix,
		suffix:      suffix,
		readerAt:    readerAt,
		fileLength:  byteLength,
	}, nil
}

func checkedMultipartLength(parts ...int64) (int64, error) {
	var total int64
	for _, part := range parts {
		if part < 0 || total > math.MaxInt64-part {
			return 0, errors.New("iris: multipart body length overflow")
		}
		total += part
	}
	return total, nil
}

func (f *FileBodyFactory) NewBody() (io.ReadCloser, error) {
	return &fileBodyReader{
		reader: io.MultiReader(
			bytes.NewReader(f.prefix),
			io.NewSectionReader(f.readerAt, 0, f.fileLength),
			bytes.NewReader(f.suffix),
		),
	}, nil
}

func (f *FileBodyFactory) ContentType() string {
	return f.contentType
}

func (f *FileBodyFactory) BodySHA256() string {
	return f.bodySHA256
}

func (f *FileBodyFactory) BodyLength() int64 {
	return f.bodyLength
}

type fileBodyReader struct {
	reader io.Reader
	closed atomic.Bool
}

func (r *fileBodyReader) Read(p []byte) (int, error) {
	if r.closed.Load() {
		return 0, io.ErrClosedPipe
	}
	return r.reader.Read(p)
}

func (r *fileBodyReader) Close() error {
	r.closed.Store(true)
	return nil
}

func copyReaderAtContext(ctx context.Context, dst io.Writer, readerAt io.ReaderAt, byteLength int64) error {
	if readerAt == nil {
		return errors.New("reader is nil")
	}
	if byteLength < 0 {
		return errors.New("negative byte length")
	}

	section := io.NewSectionReader(readerAt, 0, byteLength)
	buffer, ok := fileCopyBufferPool.Get().(*[fileCopyBufferBytes]byte)
	if !ok {
		return errors.New("invalid file copy buffer")
	}
	defer func() {
		clear(buffer[:])
		fileCopyBufferPool.Put(buffer)
	}()

	var copied int64
	for copied < byteLength {
		if err := ctx.Err(); err != nil {
			return err
		}

		remaining := byteLength - copied
		chunk := buffer[:]
		if remaining < int64(len(chunk)) {
			chunk = chunk[:remaining]
		}

		n, readErr := section.Read(chunk)
		if n > 0 {
			written, writeErr := dst.Write(chunk[:n])
			copied += int64(written)
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return readErr
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}

	if copied != byteLength {
		return fmt.Errorf("short source: read %d of %d bytes", copied, byteLength)
	}

	return nil
}
