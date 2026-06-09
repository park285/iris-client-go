package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMultipartBodyFactoryRebuildsBodyForRetry(t *testing.T) {
	metadata := []byte(`{"type":"image","room":"room"}`)
	images := [][]byte{[]byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3}}
	factory := newMultipartBodyFactory(metadata, images)

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

func TestH2CClientSendImageUsesKnownLengthMultipartRequest(t *testing.T) {
	t.Parallel()

	var sawKnownLength bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKnownLength = r.ContentLength > 0 && len(r.TransferEncoding) == 0

		metadata, images := readMultipartReplyRequest(t, r)
		if metadata.Type != "image" || metadata.Room != "room" {
			t.Fatalf("metadata = %+v, want image room", metadata)
		}
		if len(images) != 1 || !bytes.Equal(images[0], []byte{0x89, 'P', 'N', 'G'}) {
			t.Fatalf("images = %v, want one png payload", images)
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ReplyAcceptedResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "stream",
			Room:      "room",
			Type:      "image",
		}); err != nil {
			t.Errorf("Encode() error = %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "", WithHTTPClient(server.Client()))
	if _, err := c.SendImage(t.Context(), "room", []byte{0x89, 'P', 'N', 'G'}); err != nil {
		t.Fatalf("SendImage() error = %v", err)
	}

	if !sawKnownLength {
		t.Fatal("request did not arrive with known content length")
	}
}

func TestMultipartBodyFactoryContentType(t *testing.T) {
	factory := newMultipartBodyFactory([]byte(`{}`), [][]byte{[]byte("image")})

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
