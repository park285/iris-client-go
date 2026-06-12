package client

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
)

type multipartBodyFactory struct {
	contentType string
	bodySHA256  string
	bodyLength  int64
	chunks      [][]byte
}

func newMultipartBodyFactory(metadataBytes []byte, images [][]byte, contentTypes []string) *multipartBodyFactory {
	boundary := generateMultipartBoundary()
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

	return &multipartBodyFactory{
		contentType: "multipart/form-data; boundary=" + boundary,
		bodySHA256:  hex.EncodeToString(hash.Sum(nil)),
		bodyLength:  bodyLength,
		chunks:      chunks,
	}
}

func (f *multipartBodyFactory) NewBody() (io.ReadCloser, error) {
	return &multipartBodyReader{chunks: f.chunks}, nil
}

func (f *multipartBodyFactory) ContentType() string {
	return f.contentType
}

func (f *multipartBodyFactory) BodySHA256() string {
	return f.bodySHA256
}

func (f *multipartBodyFactory) BodyLength() int64 {
	return f.bodyLength
}

type multipartBodyReader struct {
	chunks [][]byte
	index  int
	offset int
	closed bool
}

func (r *multipartBodyReader) Read(p []byte) (int, error) {
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

func (r *multipartBodyReader) Close() error {
	r.closed = true
	return nil
}
