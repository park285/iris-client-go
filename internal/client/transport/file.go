package transport

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	clientmultipart "github.com/park285/iris-client-go/v3/internal/client/multipart"
)

const msgTypeFile = "file"

type filePartSpec struct {
	Index       int    `json:"index"`
	SHA256Hex   string `json:"sha256Hex"`
	ByteLength  int64  `json:"byteLength"`
	ContentType string `json:"contentType"`
	FileName    string `json:"fileName"`
}

type replyFileMetadata struct {
	ClientRequestID *string        `json:"clientRequestId,omitempty"`
	Type            string         `json:"type"`
	Room            string         `json:"room"`
	ThreadID        *string        `json:"threadId,omitempty"`
	ThreadScope     *int           `json:"threadScope,omitempty"`
	Files           []filePartSpec `json:"files"`
}

// ReplyFile은 SendFile에서 사용할 임의 접근 파일이다.
// 호출자는 ReaderAt를 소유하며 SendFile이 반환할 때까지 내용을 유지해야 한다.
type ReplyFile struct {
	FileName    string
	ContentType string
	ByteLength  int64
	readerAt    io.ReaderAt
}

// NewReplyFile은 readerAt를 복사하거나 할당하지 않고 파일 페이로드를 만든다. 검증은 SendFile이 맡는다.
func NewReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) ReplyFile {
	return ReplyFile{
		FileName:    fileName,
		ContentType: contentType,
		ByteLength:  byteLength,
		readerAt:    readerAt,
	}
}

// NewReplyFileBytes는 data를 복사하지 않고 파일 페이로드를 만든다. SendFile 반환 전에는 data를 변경하면 안 된다.
func NewReplyFileBytes(fileName, contentType string, data []byte) ReplyFile {
	return NewReplyFile(fileName, contentType, int64(len(data)), bytes.NewReader(data))
}

// FileSender는 기존 Sender 구현을 변경하지 않고 파일 응답 기능을 추가한다.
type FileSender interface {
	SendFile(ctx context.Context, room string, file ReplyFile, opts ...SendOption) (*ReplyAcceptedResponse, error)
}

var _ FileSender = (*APIClient)(nil)

func (c *APIClient) SendFile(ctx context.Context, room string, file ReplyFile, opts ...SendOption) (*ReplyAcceptedResponse, error) {
	o := applySendOptions(opts)
	if err := validateSendOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	if err := validateFileReplyOptions(o); err != nil {
		return nil, fmt.Errorf("validate send options: %w", err)
	}

	contentType, err := clientmultipart.NormalizeReplyFile(file.FileName, file.ContentType, file.ByteLength, file.readerAt)
	if err != nil {
		return nil, fmt.Errorf("validate file payload: %w", err)
	}

	fileSHA256, err := clientmultipart.DigestReaderAt(ctx, file.readerAt, file.ByteLength)
	if err != nil {
		return nil, fmt.Errorf("prepare file payload: %w", err)
	}

	metadata := replyFileMetadata{
		ClientRequestID: normalizeClientRequestID(o.ClientRequestID),
		Type:            msgTypeFile,
		Room:            room,
		ThreadID:        normalizeReplyThreadID(o.ThreadID),
		ThreadScope:     normalizeReplyThreadScope(o.ThreadScope),
		Files: []filePartSpec{{
			Index:       0,
			SHA256Hex:   fileSHA256,
			ByteLength:  file.ByteLength,
			ContentType: contentType,
			FileName:    file.FileName,
		}},
	}

	resp, err := c.postFileMultipart(ctx, metadata, file, contentType)
	if err != nil {
		return nil, fmt.Errorf("send iris file: %w", err)
	}

	return resp, nil
}

// SendFilePath는 요청 동안 일반 파일을 열고 모든 반환 경로에서 닫는다.
// 비어 있는 contentType은 확장자로 추론하며, 알 수 없으면 application/octet-stream을 사용한다.
func (c *APIClient) SendFilePath(
	ctx context.Context,
	room string,
	path string,
	contentType string,
	opts ...SendOption,
) (resp *ReplyAcceptedResponse, err error) {
	// #nosec G304 -- 전송할 파일 경로는 호출자가 지정하는 공개 API 인자다.
	fileHandle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open iris reply file: %w", err)
	}

	defer func() {
		if closeErr := fileHandle.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close iris reply file: %w", closeErr))
		}
	}()

	info, err := fileHandle.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat iris reply file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, errors.New("iris: reply file path must reference a regular file")
	}

	resolvedContentType := strings.TrimSpace(contentType)
	if resolvedContentType == "" {
		resolvedContentType = mediaTypeForFilePath(path)
	}

	return c.SendFile(
		ctx,
		room,
		NewReplyFile(filepath.Base(path), resolvedContentType, info.Size(), fileHandle),
		opts...,
	)
}

func mediaTypeForFilePath(path string) string {
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(path)))
	if contentType == "" {
		return mimeApplicationOctetStream
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return mimeApplicationOctetStream
	}

	return mediaType
}

func validateFileReplyOptions(o sendOptions) error {
	if o.ImageContentType != nil {
		return errors.New("iris: imageContentType is supported only for SendImage")
	}

	if len(o.Mentions) > 0 {
		return errors.New("iris: mentions are supported only for text and markdown replies")
	}

	if hasAttachmentJSON(o.AttachmentJSON) {
		return errAttachmentJSONRequiresText
	}

	return nil
}

func (c *APIClient) postFileMultipart(
	ctx context.Context,
	metadata replyFileMetadata,
	file ReplyFile,
	contentType string,
) (*ReplyAcceptedResponse, error) {
	metadataBytes, err := jsonv2.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode metadata: %w", PathReply, err)
	}

	bodyFactory, err := clientmultipart.NewFileBodyFactory(
		ctx,
		generateMultipartBoundary(),
		metadataBytes,
		file.FileName,
		contentType,
		file.ByteLength,
		file.readerAt,
	)
	if err != nil {
		return nil, fmt.Errorf("post %s: create multipart body factory: %w", PathReply, err)
	}

	return c.retryPostJSON[ReplyAcceptedResponse](ctx, PathReply, metadata.ClientRequestID != nil, func(attemptCtx context.Context) (*http.Request, error) {
		body, bodyErr := bodyFactory.NewBody()
		if bodyErr != nil {
			return nil, fmt.Errorf("post %s: create multipart body: %w", PathReply, bodyErr)
		}

		req, requestErr := c.newSignedStreamRequest(
			attemptCtx,
			http.MethodPost,
			PathReply,
			body,
			bodyFactory.BodySHA256(),
			SecretRoleBotControl,
		)
		if requestErr != nil {
			return nil, errors.Join(fmt.Errorf("post %s: %w", PathReply, requestErr), body.Close())
		}

		req.Header.Set("Content-Type", bodyFactory.ContentType())

		req.ContentLength = bodyFactory.BodyLength()
		req.GetBody = bodyFactory.NewBody

		return req, nil
	})
}
