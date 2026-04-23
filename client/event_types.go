package client

import "encoding/json"

type MemberEvent struct {
	Type      string  `json:"type"`
	Event     string  `json:"event"`
	ChatID    int64   `json:"chatId"`
	LinkID    *int64  `json:"linkId,omitempty"`
	UserID    int64   `json:"userId"`
	Nickname  *string `json:"nickname,omitempty"`
	Estimated bool    `json:"estimated"`
	Timestamp int64   `json:"timestamp"`
}

type NicknameChangeEvent struct {
	Type        string  `json:"type"`
	ChatID      int64   `json:"chatId"`
	LinkID      *int64  `json:"linkId,omitempty"`
	UserID      int64   `json:"userId"`
	OldNickname *string `json:"oldNickname,omitempty"`
	NewNickname *string `json:"newNickname,omitempty"`
	Timestamp   int64   `json:"timestamp"`
}

type RoleChangeEvent struct {
	Type      string `json:"type"`
	ChatID    int64  `json:"chatId"`
	LinkID    *int64 `json:"linkId,omitempty"`
	UserID    int64  `json:"userId"`
	OldRole   string `json:"oldRole"`
	NewRole   string `json:"newRole"`
	Timestamp int64  `json:"timestamp"`
}

type ProfileChangeEvent struct {
	Type               string  `json:"type"`
	ChatID             int64   `json:"chatId"`
	LinkID             *int64  `json:"linkId,omitempty"`
	UserID             int64   `json:"userId"`
	Timestamp          int64   `json:"timestamp"`
	Nickname           *string `json:"nickname,omitempty"`
	OldProfileImageURL *string `json:"oldProfileImageUrl,omitempty"`
	NewProfileImageURL *string `json:"newProfileImageUrl,omitempty"`
}

type RawSSEEvent struct {
	ID    int64
	Event string
	Data  json.RawMessage
}
