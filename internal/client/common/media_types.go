package common

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// MediaChunkRequest는 Iris가 인증된 Kakao media source에서 읽을 chunk를 지정합니다.
// ChatID와 ChatLogID는 wire에서 숫자 정밀도가 손실되지 않도록 문자열로 유지합니다.
type MediaChunkRequest struct {
	MessageID          string `json:"messageId"`
	SourceGenerationID int64  `json:"sourceGenerationId"`
	RawSourceLogID     int64  `json:"rawSourceLogId"`
	SourceLogID        int64  `json:"sourceLogId"`
	ChatID             string `json:"chatId"`
	ChatLogID          string `json:"chatLogId"`
	Type               string `json:"type"`
	MediaIndex         int    `json:"mediaIndex"`
	Offset             int64  `json:"offset"`
	Length             int    `json:"length"`
}

// MediaChunkResponse는 Iris media broker가 반환하는 서버 소유 chunk metadata입니다.
// 응답의 semantic, size, MIME, SHA-256 검증은 소비자가 수행합니다.
type MediaChunkResponse struct {
	ChunkBase64 string `json:"chunkBase64"`
	TotalLength int64  `json:"totalLength"`
	MIMEType    string `json:"mimeType"`
	SHA256      string `json:"sha256"`
	EOF         bool   `json:"eof"`
	MediaCount  int    `json:"mediaCount"`
}

// UnmarshalJSON rejects schema drift before an authenticated broker response
// reaches a media consumer. All six fields are required by the v1 contract.
func (response *MediaChunkResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		ChunkBase64 *string
		TotalLength *int64
		MIMEType    *string
		SHA256      *string
		EOF         *bool
		MediaCount  *int
	}

	err := decodeStrictJSONObject(data, map[string]struct{}{
		"chunkBase64": {},
		"totalLength": {},
		"mimeType":    {},
		"sha256":      {},
		"eof":         {},
		"mediaCount":  {},
	}, func(field string, value json.RawMessage) error {
		switch field {
		case "chunkBase64":
			return decodeRequiredJSONValue(value, "chunkBase64", &wire.ChunkBase64)
		case "totalLength":
			return decodeRequiredJSONValue(value, "totalLength", &wire.TotalLength)
		case "mimeType":
			return decodeRequiredJSONValue(value, "mimeType", &wire.MIMEType)
		case "sha256":
			return decodeRequiredJSONValue(value, "sha256", &wire.SHA256)
		case "eof":
			return decodeRequiredJSONValue(value, "eof", &wire.EOF)
		case "mediaCount":
			return decodeRequiredJSONValue(value, "mediaCount", &wire.MediaCount)
		default:
			return fmt.Errorf("decode media chunk response: unknown field %q", field)
		}
	})
	if err != nil {
		return fmt.Errorf("decode media chunk response: %w", err)
	}
	if wire.ChunkBase64 == nil || wire.TotalLength == nil || wire.MIMEType == nil ||
		wire.SHA256 == nil || wire.EOF == nil || wire.MediaCount == nil {
		return errors.New("decode media chunk response: required field missing")
	}

	*response = MediaChunkResponse{
		ChunkBase64: *wire.ChunkBase64,
		TotalLength: *wire.TotalLength,
		MIMEType:    *wire.MIMEType,
		SHA256:      *wire.SHA256,
		EOF:         *wire.EOF,
		MediaCount:  *wire.MediaCount,
	}
	return nil
}

func decodeStrictJSONObject(
	data []byte,
	knownFields map[string]struct{},
	assign func(string, json.RawMessage) error,
) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	openingToken, openingErr := decoder.Token()
	if openingErr != nil {
		return fmt.Errorf("expected JSON object: %w", openingErr)
	}
	if delimiter, ok := openingToken.(json.Delim); !ok || delimiter != '{' {
		return errors.New("expected JSON object")
	}

	seen := make(map[string]struct{}, len(knownFields))
	for decoder.More() {
		fieldToken, tokenErr := decoder.Token()
		if tokenErr != nil {
			return fmt.Errorf("read field name: %w", tokenErr)
		}
		field, ok := fieldToken.(string)
		if !ok {
			return errors.New("field name must be a string")
		}
		if _, ok := knownFields[field]; !ok {
			return fmt.Errorf("unknown field %q", field)
		}
		if _, duplicate := seen[field]; duplicate {
			return fmt.Errorf("duplicate field %q", field)
		}
		seen[field] = struct{}{}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode field %q: %w", field, err)
		}
		if err := assign(field, value); err != nil {
			return err
		}
	}

	closingToken, closingErr := decoder.Token()
	if closingErr != nil {
		return fmt.Errorf("close JSON object: %w", closingErr)
	}
	if delimiter, ok := closingToken.(json.Delim); !ok || delimiter != '}' {
		return errors.New("expected end of JSON object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func decodeRequiredJSONValue[T any](value json.RawMessage, field string, target **T) error {
	var decoded *T
	if err := json.Unmarshal(value, &decoded); err != nil {
		return fmt.Errorf("decode field %q: %w", field, err)
	}
	if decoded == nil {
		return fmt.Errorf("decode field %q: null is not allowed", field)
	}
	*target = decoded
	return nil
}
