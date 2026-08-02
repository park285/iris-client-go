package multipart

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"strings"
	"testing"
)

func TestMultipartBodyFactoryRebuildsBodyForRetry(t *testing.T) {
	metadata := []byte(`{"type":"image","room":"room"}`)
	images := [][]byte{[]byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}}
	factory, err := newMultipartBodyFactory(metadata, images, []string{"image/png"})
	if err != nil {
		t.Fatalf("newMultipartBodyFactory() error = %v", err)
	}

	first, err := readFactoryBody(factory)
	if err != nil {
		t.Fatalf("first body: %v", err)
	}
	second, err := readFactoryBody(factory)
	if err != nil {
		t.Fatalf("second body: %v", err)
	}

	if len(first) == 0 {
		t.Fatal("first body is empty")
	}
	if !bytes.Equal(second, first) {
		t.Fatal("second body differs from first body")
	}
}

func TestMultipartBodyFactoryContentType(t *testing.T) {
	factory, err := newMultipartBodyFactory([]byte(`{}`), [][]byte{[]byte("image")}, []string{"application/octet-stream"})
	if err != nil {
		t.Fatalf("newMultipartBodyFactory() error = %v", err)
	}

	mediaType, params, err := mime.ParseMediaType(factory.ContentType())
	if err != nil {
		t.Fatalf("ParseMediaType() error = %v", err)
	}
	if mediaType != "multipart/form-data" {
		t.Fatalf("media type = %q, want multipart/form-data", mediaType)
	}
	if params["boundary"] == "" {
		t.Fatal("boundary is empty")
	}
}

func readFactoryBody(factory *multipartBodyFactory) ([]byte, error) {
	body, err := factory.NewBody()
	if err != nil {
		return nil, err
	}
	defer func() { _ = body.Close() }()

	payload, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}
	if int64(len(payload)) != factory.BodyLength() {
		return nil, errors.New("body length mismatch")
	}
	return payload, nil
}

// envelope 위반은 해시 전에 잡혀야 하므로 factory 자체가 만들어지지 않는다.
func TestMultipartBodyFactoryRejectsOversizeEnvelopeBeforeHashing(t *testing.T) {
	oversizeMetadata := bytes.Repeat([]byte("m"), maxReplyMetadataBytes+1)

	factory, err := newMultipartBodyFactory(oversizeMetadata, [][]byte{[]byte("image")}, []string{"image/png"})
	if err == nil {
		t.Fatal("newMultipartBodyFactory() error = nil, want an envelope rejection")
	}
	if factory != nil {
		t.Fatal("newMultipartBodyFactory() returned a factory alongside the envelope rejection")
	}
	if !strings.Contains(err.Error(), "metadata too large") {
		t.Fatalf("newMultipartBodyFactory() error = %v, want the metadata envelope error", err)
	}
}
