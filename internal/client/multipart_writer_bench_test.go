package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func BenchmarkSendImage_BufferedBaseline(b *testing.B) {
	image := bytes.Repeat([]byte{0xFF}, 10*1024*1024)
	c := newBenchmarkReplyClient(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sendImageBufferedBaseline(context.Background(), c, "room", image); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMultipartNaiveStreamingRegression(b *testing.B) {
	image := bytes.Repeat([]byte{0xFF}, 10*1024*1024)
	c := newBenchmarkReplyClient(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := sendImageNaiveStreaming(context.Background(), c, "room", image); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSendImage_Streaming(b *testing.B) {
	image := bytes.Repeat([]byte{0xFF}, 10*1024*1024)
	c := newBenchmarkReplyClient(b)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.SendImage(context.Background(), "room", image); err != nil {
			b.Fatal(err)
		}
	}
}

func newBenchmarkReplyClient(b *testing.B) *H2CClient {
	b.Helper()

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			b.Errorf("Copy(body) error = %v", err)
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(bytes.NewReader([]byte(
				`{"success":true,"delivery":"queued","requestId":"bench","room":"room","type":"image"}`,
			))),
		}, nil
	})

	return NewH2CClient("http://localhost", "", WithRoundTripper(rt))
}

func sendImageBufferedBaseline(ctx context.Context, c *H2CClient, room string, imageData []byte) (*ReplyAcceptedResponse, error) {
	images := [][]byte{imageData}
	contentTypes := []string{detectImageContentType(imageData)}
	metadata := replyImageMetadata{
		Type:   "image",
		Room:   room,
		Images: buildImageManifest(images, contentTypes),
	}

	metadataBytes, err := jsonx.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	boundary := generateMultipartBoundary()
	contentType := "multipart/form-data; boundary=" + boundary

	var body bytes.Buffer
	if err := writeBufferedBaselineMultipartBody(&body, boundary, metadataBytes, images); err != nil {
		return nil, fmt.Errorf("encode multipart body: %w", err)
	}
	bodyBytes := body.Bytes()

	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, PathReply, false, func(attemptCtx context.Context) (*http.Request, error) {
		req, err := c.newSignedRequest(attemptCtx, http.MethodPost, PathReply, bodyBytes, SecretRoleBotControl)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", contentType)
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func writeBufferedBaselineMultipartBody(w io.Writer, boundary string, metadataBytes []byte, images [][]byte) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("set boundary: %w", err)
	}

	if err := mw.WriteField("metadata", string(metadataBytes)); err != nil {
		return fmt.Errorf("write metadata field: %w", err)
	}

	for i, img := range images {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="image-%d"`, i))
		header.Set("Content-Type", detectImageContentType(img))
		partWriter, err := mw.CreatePart(header)
		if err != nil {
			return fmt.Errorf("create image part: %w", err)
		}
		if _, err := partWriter.Write(img); err != nil {
			return fmt.Errorf("write image data: %w", err)
		}
	}

	return mw.Close()
}

func sendImageNaiveStreaming(ctx context.Context, c *H2CClient, room string, imageData []byte) (*ReplyAcceptedResponse, error) {
	images := [][]byte{imageData}
	contentTypes := []string{detectImageContentType(imageData)}
	metadata := replyImageMetadata{
		Type:   "image",
		Room:   room,
		Images: buildImageManifest(images, contentTypes),
	}

	metadataBytes, err := jsonx.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode metadata: %w", err)
	}

	boundary := generateMultipartBoundary()
	contentType := "multipart/form-data; boundary=" + boundary

	var resp ReplyAcceptedResponse
	if err := c.postWithRetry(ctx, PathReply, false, func(attemptCtx context.Context) (*http.Request, error) {
		pr, pw := io.Pipe()
		go func() {
			pw.CloseWithError(writeNaiveStreamingMultipartBody(pw, boundary, metadataBytes, images))
		}()

		req, err := http.NewRequestWithContext(attemptCtx, http.MethodPost, c.baseURL+PathReply, pr)
		if err != nil {
			_ = pr.Close()
			return nil, fmt.Errorf("build iris request: %w", err)
		}
		req.Header.Set("Content-Type", contentType)
		return req, nil
	}, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func writeNaiveStreamingMultipartBody(w io.Writer, boundary string, metadataBytes []byte, images [][]byte) error {
	mw := multipart.NewWriter(w)
	if err := mw.SetBoundary(boundary); err != nil {
		return fmt.Errorf("set boundary: %w", err)
	}

	if err := mw.WriteField("metadata", string(metadataBytes)); err != nil {
		return fmt.Errorf("write metadata field: %w", err)
	}

	for i, img := range images {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="image"; filename="image-%d"`, i))
		header.Set("Content-Type", detectImageContentType(img))
		partWriter, err := mw.CreatePart(header)
		if err != nil {
			return fmt.Errorf("create image part: %w", err)
		}
		if _, err := partWriter.Write(img); err != nil {
			return fmt.Errorf("write image data: %w", err)
		}
	}

	return mw.Close()
}
