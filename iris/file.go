package iris

import (
	"io"

	client "github.com/park285/iris-client-go/internal/client/transport"
)

// ReplyFile describes one stable random-access file payload.
type ReplyFile = client.ReplyFile

// FileSender is the optional file-reply capability implemented by Iris clients.
type FileSender = client.FileSender

// NewReplyFile creates a file payload over caller-owned random-access storage.
func NewReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) ReplyFile {
	return client.NewReplyFile(fileName, contentType, byteLength, readerAt)
}

// NewReplyFileBytes creates a zero-copy file payload over data.
func NewReplyFileBytes(fileName, contentType string, data []byte) ReplyFile {
	return client.NewReplyFileBytes(fileName, contentType, data)
}
