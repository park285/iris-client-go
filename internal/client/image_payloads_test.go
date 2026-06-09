package client

import (
	"strings"
	"testing"
)

func TestValidateReplyImagesAcceptsRuntimePolicyEnvelope(t *testing.T) {
	t.Parallel()

	images := [][]byte{
		{0x89, 'P', 'N', 'G', 1},
		{0xFF, 0xD8, 0xFF, 2},
	}
	if err := validateReplyImages(images); err != nil {
		t.Fatalf("validateReplyImages() error = %v, want nil", err)
	}
}

func TestValidateReplyImagesRejectsEmptyRequest(t *testing.T) {
	t.Parallel()

	if err := validateReplyImages(nil); err == nil || !strings.Contains(err.Error(), "at least one image") {
		t.Fatalf("validateReplyImages(nil) error = %v, want at least one image", err)
	}
}

func TestValidateReplyImagesRejectsEmptyImage(t *testing.T) {
	t.Parallel()

	if err := validateReplyImages([][]byte{{}}); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("validateReplyImages(empty image) error = %v, want empty image", err)
	}
}

func TestValidateReplyImagesRejectsTooManyImages(t *testing.T) {
	t.Parallel()

	images := make([][]byte, maxReplyImagesPerRequest+1)
	for i := range images {
		images[i] = []byte{0xFF, 0xD8, 0xFF, byte(i)}
	}
	if err := validateReplyImages(images); err == nil || !strings.Contains(err.Error(), "too many images") {
		t.Fatalf("validateReplyImages(too many) error = %v, want too many images", err)
	}
}

func TestValidateReplyImagesRejectsSingleImageOverRuntimeLimit(t *testing.T) {
	t.Parallel()

	image := make([]byte, maxReplySingleImageBytes+1)
	if err := validateReplyImages([][]byte{image}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("validateReplyImages(single too large) error = %v, want too large", err)
	}
}

func TestValidateReplyImagesRejectsTotalImageBytesOverRuntimeLimit(t *testing.T) {
	t.Parallel()

	images := [][]byte{
		make([]byte, maxReplySingleImageBytes),
		make([]byte, maxReplyTotalImageBytes-maxReplySingleImageBytes+1),
	}
	if err := validateReplyImages(images); err == nil || !strings.Contains(err.Error(), "payloads too large") {
		t.Fatalf("validateReplyImages(total too large) error = %v, want payloads too large", err)
	}
}

func TestValidateReplyMultipartEnvelopeRejectsMetadataOverRuntimeLimit(t *testing.T) {
	t.Parallel()

	metadata := make([]byte, maxReplyMetadataBytes+1)
	if err := validateReplyMultipartEnvelope(metadata, int64(len(metadata))); err == nil || !strings.Contains(err.Error(), "metadata too large") {
		t.Fatalf("validateReplyMultipartEnvelope(metadata too large) error = %v, want metadata too large", err)
	}
}

func TestValidateReplyMultipartEnvelopeRejectsBodyOverRuntimeLimit(t *testing.T) {
	t.Parallel()

	if err := validateReplyMultipartEnvelope([]byte(`{"type":"image"}`), int64(maxReplyMultipartBodyBytes+1)); err == nil || !strings.Contains(err.Error(), "multipart body too large") {
		t.Fatalf("validateReplyMultipartEnvelope(body too large) error = %v, want multipart body too large", err)
	}
}
