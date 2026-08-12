package transport

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	PathMediaChunk = "/media/chunk"

	mediaTypeImage      = "2"
	mediaTypeMultiImage = "27"
	mediaTypeFile       = "18"
	mediaMaxIndex       = 9
	mediaMaxCount       = mediaMaxIndex + 1
	mediaMaxChunkBytes  = 512 * 1024
	mediaMaxTotalBytes  = 30 * 1024 * 1024
	mediaMaxMIMEBytes   = 128
)

// MediaClient는 Iris가 제공하는 optional typed media chunk capability입니다.
// Sender와 분리해 기존 custom client의 source compatibility를 유지합니다.
type MediaClient interface {
	FetchMediaChunk(ctx context.Context, req MediaChunkRequest) (*MediaChunkResponse, error)
}

var _ MediaClient = (*H2CClient)(nil)

// FetchMediaChunk은 인증된 Iris media broker에서 하나의 chunk를 가져옵니다.
func (c *H2CClient) FetchMediaChunk(ctx context.Context, req MediaChunkRequest) (*MediaChunkResponse, error) {
	normalized, err := normalizeMediaChunkRequest(req)
	if err != nil {
		return nil, fmt.Errorf("validate media chunk request: %w", err)
	}

	var response MediaChunkResponse
	if err := c.postStrictJSON(ctx, PathMediaChunk, normalized, &response, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("fetch iris media chunk: %w", err)
	}
	if err := validateMediaChunkResponse(response, normalized); err != nil {
		return nil, fmt.Errorf("validate iris media chunk response: %w", err)
	}

	return &response, nil
}

func normalizeMediaChunkRequest(req MediaChunkRequest) (MediaChunkRequest, error) {
	messageID, err := normalizeMediaStringIdentity("messageId", req.MessageID)
	if err != nil {
		return MediaChunkRequest{}, err
	}
	req.MessageID = messageID
	if req.SourceGenerationID < 0 {
		return MediaChunkRequest{}, fmt.Errorf("sourceGenerationId must be non-negative, got %d", req.SourceGenerationID)
	}
	if validationErr := validatePositiveMediaIdentity("rawSourceLogId", req.RawSourceLogID); validationErr != nil {
		return MediaChunkRequest{}, validationErr
	}
	if validationErr := validatePositiveMediaIdentity("sourceLogId", req.SourceLogID); validationErr != nil {
		return MediaChunkRequest{}, validationErr
	}
	chatID, err := normalizePositiveDecimalMediaIdentity("chatId", req.ChatID)
	if err != nil {
		return MediaChunkRequest{}, err
	}
	req.ChatID = chatID
	chatLogID, err := normalizePositiveDecimalMediaIdentity("chatLogId", req.ChatLogID)
	if err != nil {
		return MediaChunkRequest{}, err
	}
	req.ChatLogID = chatLogID
	switch req.Type {
	case mediaTypeImage, mediaTypeMultiImage, mediaTypeFile:
	default:
		return MediaChunkRequest{}, fmt.Errorf("type must be one of %q, %q, or %q", mediaTypeImage, mediaTypeMultiImage, mediaTypeFile)
	}
	if req.MediaIndex < 0 || req.MediaIndex > mediaMaxIndex {
		return MediaChunkRequest{}, fmt.Errorf("mediaIndex must be between 0 and %d, got %d", mediaMaxIndex, req.MediaIndex)
	}
	if req.Offset < 0 {
		return MediaChunkRequest{}, fmt.Errorf("offset must be non-negative, got %d", req.Offset)
	}
	if req.Length < 1 || req.Length > mediaMaxChunkBytes {
		return MediaChunkRequest{}, fmt.Errorf("length must be between 1 and %d, got %d", mediaMaxChunkBytes, req.Length)
	}
	if req.Offset > math.MaxInt64-int64(req.Length) {
		return MediaChunkRequest{}, fmt.Errorf("offset plus length overflows int64: offset=%d length=%d", req.Offset, req.Length)
	}

	return req, nil
}

func validateMediaChunkResponse(response MediaChunkResponse, request MediaChunkRequest) error {
	if err := validateMediaChunkMetadata(response, request.MediaIndex); err != nil {
		return err
	}
	decoded, err := decodeMediaChunk(response.ChunkBase64, request.Length)
	if err != nil {
		return err
	}
	return validateMediaChunkRange(response, request, len(decoded))
}

func validateMediaChunkMetadata(response MediaChunkResponse, mediaIndex int) error {
	if response.TotalLength < 1 || response.TotalLength > mediaMaxTotalBytes {
		return fmt.Errorf("totalLength must be between 1 and %d, got %d", mediaMaxTotalBytes, response.TotalLength)
	}
	if response.MediaCount < 1 || response.MediaCount > mediaMaxCount {
		return fmt.Errorf("mediaCount must be between 1 and %d, got %d", mediaMaxCount, response.MediaCount)
	}
	if mediaIndex < 0 || mediaIndex >= response.MediaCount {
		return fmt.Errorf("mediaIndex %d is outside mediaCount %d", mediaIndex, response.MediaCount)
	}
	if !isValidMediaMIMEType(response.MIMEType) {
		return fmt.Errorf("mimeType is invalid")
	}
	if !isLowerSHA256Hex(response.SHA256) {
		return fmt.Errorf("sha256 must be lowercase 64-hex SHA-256")
	}
	return nil
}

func decodeMediaChunk(chunkBase64 string, requestedLength int) ([]byte, error) {
	maxEncodedLength := (requestedLength + 2) / 3 * 4
	if len(chunkBase64) > maxEncodedLength {
		return nil, fmt.Errorf("chunkBase64 exceeds requested length")
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(chunkBase64)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != chunkBase64 {
		return nil, fmt.Errorf("chunkBase64 must be canonical base64")
	}
	if len(decoded) < 1 || len(decoded) > requestedLength {
		return nil, fmt.Errorf("decoded chunk length must be between 1 and %d, got %d", requestedLength, len(decoded))
	}
	return decoded, nil
}

func validateMediaChunkRange(response MediaChunkResponse, request MediaChunkRequest, decodedLength int) error {
	chunkLength := int64(decodedLength)
	if request.Offset < 0 || request.Offset > math.MaxInt64-chunkLength {
		return fmt.Errorf("media chunk range overflows int64")
	}
	end := request.Offset + chunkLength
	if end > response.TotalLength {
		return fmt.Errorf("media chunk ends at %d beyond totalLength %d", end, response.TotalLength)
	}
	if response.EOF != (end == response.TotalLength) {
		return fmt.Errorf("eof does not match media chunk end")
	}
	if !response.EOF && decodedLength != request.Length {
		return fmt.Errorf("non-final chunk length must equal requested length %d, got %d", request.Length, decodedLength)
	}
	return nil
}

func isValidMediaMIMEType(value string) bool {
	if value == "" || len(value) > mediaMaxMIMEBytes || !isASCII(value) {
		return false
	}
	parts := strings.Split(value, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return false
	}
	return isMediaMIMEToken(parts[0]) && isMediaMIMEToken(parts[1])
}

func isASCII(value string) bool {
	for index := range len(value) {
		if value[index] >= 0x80 {
			return false
		}
	}
	return true
}

func isMediaMIMEToken(value string) bool {
	for index := range len(value) {
		character := value[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') {
			continue
		}
		switch character {
		case '!', '#', '$', '&', '^', '_', '.', '+', '-':
		default:
			return false
		}
	}
	return true
}

func isLowerSHA256Hex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for index := range len(value) {
		character := value[index]
		if (character >= '0' && character <= '9') || (character >= 'a' && character <= 'f') {
			continue
		}
		return false
	}
	return true
}

func validatePositiveMediaIdentity(label string, value int64) error {
	if value <= 0 {
		return fmt.Errorf("%s must be positive, got %d", label, value)
	}
	return nil
}

func normalizeMediaStringIdentity(label, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must not be blank", label)
	}
	return trimmed, nil
}

func normalizePositiveDecimalMediaIdentity(label, value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s must not be blank", label)
	}
	if value[0] == '0' {
		return "", fmt.Errorf("%s must be a canonical positive decimal", label)
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return "", fmt.Errorf("%s must be a canonical positive decimal", label)
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return "", fmt.Errorf("%s must be a canonical positive decimal", label)
	}
	return value, nil
}
