package multipart

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

type BodyFactory struct {
	contentType string
	bodySHA256  string
	bodyLength  int64
	chunks      [][]byte
}

func NewBodyFactory(boundary string, metadataBytes []byte, images [][]byte, contentTypes []string) *BodyFactory {
	chunks := make([][]byte, 0, 3+len(images)*3+1)

	chunks = append(chunks,
		fmt.Appendf(nil, "--%s\r\nContent-Disposition: form-data; name=\"metadata\"\r\n\r\n", boundary),
		metadataBytes,
		[]byte("\r\n"),
	)

	for i, img := range images {
		chunks = append(chunks,
			fmt.Appendf(nil, "--%s\r\nContent-Disposition: form-data; name=\"image\"; filename=\"image-%d\"\r\nContent-Type: %s\r\n\r\n", boundary, i, contentTypes[i]),
			img,
			[]byte("\r\n"),
		)
	}

	chunks = append(chunks, fmt.Appendf(nil, "--%s--\r\n", boundary))

	hash := sha256.New()
	var bodyLength int64
	for _, chunk := range chunks {
		_, _ = hash.Write(chunk)
		bodyLength += int64(len(chunk))
	}

	return &BodyFactory{
		contentType: "multipart/form-data; boundary=" + boundary,
		bodySHA256:  hex.EncodeToString(hash.Sum(nil)),
		bodyLength:  bodyLength,
		chunks:      chunks,
	}
}

func (f *BodyFactory) NewBody() (io.ReadCloser, error) {
	return &bodyReader{chunks: f.chunks}, nil
}

func (f *BodyFactory) ContentType() string {
	return f.contentType
}

func (f *BodyFactory) BodySHA256() string {
	return f.bodySHA256
}

func (f *BodyFactory) BodyLength() int64 {
	return f.bodyLength
}

type bodyReader struct {
	chunks [][]byte
	index  int
	offset int
	closed bool
}

func (r *bodyReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}

	written := 0
	for written < len(p) && r.index < len(r.chunks) {
		chunk := r.chunks[r.index]
		if r.offset >= len(chunk) {
			r.index++
			r.offset = 0
			continue
		}

		n := copy(p[written:], chunk[r.offset:])
		written += n
		r.offset += n
	}

	if written > 0 {
		return written, nil
	}
	return 0, io.EOF
}

func (r *bodyReader) Close() error {
	r.closed = true
	return nil
}
