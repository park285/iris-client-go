package client

import "fmt"

const (
	// Keep the SDK admission envelope aligned with Iris runtime MultipartImageStagingPolicy.
	// The runtime rejects requests beyond these bounds; failing client-side avoids
	// signing and streaming a payload that the server will deterministically drop.
	maxReplyImagesPerRequest   = 8
	maxReplyMultipartBodyBytes = 31 * 1024 * 1024
	maxReplyMetadataBytes      = 64 * 1024
	maxReplySingleImageBytes   = 20 * 1024 * 1024
	maxReplyTotalImageBytes    = 30 * 1024 * 1024
)

func validateReplyImages(images [][]byte) error {
	if len(images) == 0 {
		return fmt.Errorf("iris: at least one image is required")
	}
	if len(images) > maxReplyImagesPerRequest {
		return fmt.Errorf("iris: too many images (%d, max %d)", len(images), maxReplyImagesPerRequest)
	}

	totalBytes := 0
	for index, image := range images {
		if len(image) == 0 {
			return fmt.Errorf("iris: image %d is empty", index)
		}
		if len(image) > maxReplySingleImageBytes {
			return fmt.Errorf(
				"iris: image %d too large (%d bytes, max %d)",
				index,
				len(image),
				maxReplySingleImageBytes,
			)
		}
		totalBytes += len(image)
		if totalBytes > maxReplyTotalImageBytes {
			return fmt.Errorf(
				"iris: image payloads too large (%d bytes, max %d)",
				totalBytes,
				maxReplyTotalImageBytes,
			)
		}
	}

	return nil
}

func validateReplyMultipartEnvelope(metadataBytes []byte, bodyLength int64) error {
	if len(metadataBytes) > maxReplyMetadataBytes {
		return fmt.Errorf(
			"iris: metadata too large (%d bytes, max %d)",
			len(metadataBytes),
			maxReplyMetadataBytes,
		)
	}
	if bodyLength > int64(maxReplyMultipartBodyBytes) {
		return fmt.Errorf(
			"iris: multipart body too large (%d bytes, max %d)",
			bodyLength,
			maxReplyMultipartBodyBytes,
		)
	}

	return nil
}
