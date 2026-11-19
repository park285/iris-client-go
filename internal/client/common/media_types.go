package common

import (
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
)

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
