package transport

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	clientmultipart "github.com/park285/iris-client-go/internal/client/multipart"
	"github.com/park285/iris-client-go/internal/jsonx"
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

// ReplyFile describes a stable, random-access file source for one SendFile call.
// The caller retains ownership of ReaderAt and must keep its contents unchanged
// until SendFile returns.
type ReplyFile struct {
	FileName    string
	ContentType string
	ByteLength  int64
	readerAt    io.ReaderAt
}

// NewReplyFile creates a zero-copy file payload over readerAt. Validation is
// performed by SendFile so construction stays allocation-free and composable.
func NewReplyFile(fileName, contentType string, byteLength int64, readerAt io.ReaderAt) ReplyFile {
	return ReplyFile{
		FileName:    fileName,
		ContentType: contentType,
		ByteLength:  byteLength,
		readerAt:    readerAt,
	}
}

// NewReplyFileBytes creates a zero-copy in-memory file payload. Data must not be
// mutated until SendFile returns.
func NewReplyFileBytes(fileName, contentType string, data []byte) ReplyFile {
	return NewReplyFile(fileName, contentType, int64(len(data)), bytes.NewReader(data))
}

// FileSender is an additive capability separate from Sender so existing custom
// Sender implementations remain source-compatible.
type FileSender interface {
	SendFile(ctx context.Context, room string, file ReplyFile, opts ...SendOption) (*ReplyAcceptedResponse, error)
}

var _ FileSender = (*H2CClient)(nil)

func (c *H2CClient) SendFile(ctx context.Context, room string, file ReplyFile, opts ...SendOption) (*ReplyAcceptedResponse, error) {
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

// SendFilePath opens one regular file for the duration of the request and closes
// it on every return path. An empty contentType is inferred from the extension,
// falling back to application/octet-stream.
func (c *H2CClient) SendFilePath(
	ctx context.Context,
	room string,
	path string,
	contentType string,
	opts ...SendOption,
) (resp *ReplyAcceptedResponse, err error) {
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
		return "application/octet-stream"
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		return "application/octet-stream"
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

func (c *H2CClient) postFileMultipart(
	ctx context.Context,
	metadata replyFileMetadata,
	file ReplyFile,
	contentType string,
) (*ReplyAcceptedResponse, error) {
	metadataBytes, err := jsonx.Marshal(metadata)
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
	if err := clientmultipart.ValidateReplyMultipartEnvelope(metadataBytes, bodyFactory.BodyLength()); err != nil {
		return nil, fmt.Errorf("validate multipart envelope: %w", err)
	}

	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, PathReply, metadata.ClientRequestID != nil, func(attemptCtx context.Context) (*http.Request, error) {
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
			_ = body.Close()
			return nil, fmt.Errorf("post %s: %w", PathReply, requestErr)
		}
		req.Header.Set("Content-Type", bodyFactory.ContentType())
		req.ContentLength = bodyFactory.BodyLength()
		req.GetBody = bodyFactory.NewBody
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}
