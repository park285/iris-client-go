package common

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
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

func (response *MediaChunkResponse) UnmarshalJSON(data []byte) error {
	var wire struct {
		ChunkBase64 *string `json:"chunkBase64"`
		TotalLength *int64  `json:"totalLength"`
		MIMEType    *string `json:"mimeType"`
		SHA256      *string `json:"sha256"`
		EOF         *bool   `json:"eof"`
		MediaCount  *int    `json:"mediaCount"`
	}

	if err := jsonv2.Unmarshal(data, &wire, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("decode media chunk response: %w", err)
	}

	if wire.ChunkBase64 == nil || wire.TotalLength == nil || wire.MIMEType == nil ||
		wire.SHA256 == nil || wire.EOF == nil || wire.MediaCount == nil {
		return errors.New("decode media chunk response: required field missing or null")
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
