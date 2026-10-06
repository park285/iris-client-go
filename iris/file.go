package iris

import (
	"io"

	client "github.com/park285/iris-client-go/v3/internal/client/transport"
)

// ReplyFile은 전송 중 내용이 유지되는 임의 접근 파일 페이로드다.
type ReplyFile = client.ReplyFile

// FileSender는 Iris 클라이언트가 구현하는 선택적 파일 응답 기능이다.
type FileSender = client.FileSender

// NewReplyFile은 호출자가 소유한 임의 접근 저장소로 파일 페이로드를 만든다.
func NewReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) ReplyFile {
	return client.NewReplyFile(fileName, contentType, byteLength, readerAt)
}

// NewReplyFileBytes는 data를 복사하지 않고 파일 페이로드를 만든다.
func NewReplyFileBytes(fileName, contentType string, data []byte) ReplyFile {
	return client.NewReplyFileBytes(fileName, contentType, data)
}
