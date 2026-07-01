package client

import "fmt"

const (
	// SDK admission envelope을 Iris 런타임의 MultipartImageStagingPolicy와 맞춰 둔다.
	// 런타임은 이 한계를 넘는 요청을 거부하므로, 클라이언트에서 미리 실패시키면 서버가
	// 결정론적으로 버릴 페이로드를 서명하고 스트리밍하는 낭비를 피한다.
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
