package iris

import (
	"io"

	client "github.com/park285/iris-client-go/v6/internal/client/transport"
)

type ReplyFile = client.ReplyFile

type FileSender = client.FileSender

func NewReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) ReplyFile {
	return client.NewReplyFile(fileName, contentType, byteLength, readerAt)
}
