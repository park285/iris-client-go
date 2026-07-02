package client

import (
	"context"
	"encoding/json"
	"fmt"
)

type KaringTemplateArgs map[string]string

type KaringStreamStatus string

const (
	KaringStreamStatusLive     KaringStreamStatus = "LIVE"
	KaringStreamStatusUpcoming KaringStreamStatus = "UPCOMING"
)

type KaringContentItem struct {
	Title        string             `json:"title,omitempty"`
	URL          string             `json:"url,omitempty"`
	MemberName   string             `json:"member_name,omitempty"`
	ChannelName  string             `json:"channel_name,omitempty"`
	Status       KaringStreamStatus `json:"status,omitempty"`
	StartAt      string             `json:"start_at,omitempty"`
	ThumbnailURL string             `json:"thumbnail_url,omitempty"`
	Platform     string             `json:"platform,omitempty"`
}

type KaringSendRequest struct {
	ClientRequestID *string            `json:"clientRequestId,omitempty"`
	ReceiverName    string             `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64              `json:"receiver_room_id,omitempty"`
	TemplateID      int64              `json:"template_id,omitempty"`
	TemplateArgs    KaringTemplateArgs `json:"template_args,omitempty"`
	AppKey          string             `json:"app_key,omitempty"`
	Origin          string             `json:"origin,omitempty"`
	SearchExact     *bool              `json:"search_exact,omitempty"`
	SearchFrom      string             `json:"search_from,omitempty"`
	SearchRoomType  string             `json:"search_room_type,omitempty"`
	DryRun          bool               `json:"dry_run,omitempty"`
}

type KaringContentListRequest struct {
	ClientRequestID *string             `json:"clientRequestId,omitempty"`
	Item            *KaringContentItem  `json:"item,omitempty"`
	Items           []KaringContentItem `json:"items,omitempty"`
	ExtraArgs       KaringTemplateArgs  `json:"extra_args,omitempty"`
	ReceiverName    string              `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64               `json:"receiver_room_id,omitempty"`
	TemplateID      int64               `json:"template_id,omitempty"`
	SearchExact     *bool               `json:"search_exact,omitempty"`
	SearchFrom      string              `json:"search_from,omitempty"`
	SearchRoomType  string              `json:"search_room_type,omitempty"`
	DryRun          bool                `json:"dry_run,omitempty"`
}

type KaringHololiveRequest struct {
	ClientRequestID *string             `json:"clientRequestId,omitempty"`
	Stream          *KaringContentItem  `json:"stream,omitempty"`
	Streams         []KaringContentItem `json:"streams,omitempty"`
	ExtraArgs       KaringTemplateArgs  `json:"extra_args,omitempty"`
	ReceiverName    string              `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64               `json:"receiver_room_id,omitempty"`
	TemplateID      int64               `json:"template_id,omitempty"`
	SearchExact     *bool               `json:"search_exact,omitempty"`
	SearchFrom      string              `json:"search_from,omitempty"`
	SearchRoomType  string              `json:"search_room_type,omitempty"`
	DryRun          bool                `json:"dry_run,omitempty"`
}

type KaringDryRunResponse struct {
	OK           bool               `json:"ok"`
	DryRun       bool               `json:"dry_run"`
	ReceiverName string             `json:"receiver_name,omitempty"`
	TemplateID   int64              `json:"template_id"`
	ItemCount    *int               `json:"item_count,omitempty"`
	StreamCount  *int               `json:"stream_count,omitempty"`
	TemplateArgs KaringTemplateArgs `json:"template_args"`
	Success      bool               `json:"success,omitempty"`
	Delivery     string             `json:"delivery,omitempty"`
	RequestID    string             `json:"requestId,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Duplicate    *bool              `json:"duplicate,omitempty"`
}

// Iris의 dry-run 응답(KaringDryRunResponse)은 snake_case, live 202 응답(KaringAcceptedResponse)은
// camelCase로 직렬화되므로 두 케이싱을 모두 받아 병합한다.
func (r *KaringDryRunResponse) UnmarshalJSON(data []byte) error {
	type karingResponseWire struct {
		OK           bool               `json:"ok"`
		DryRun       bool               `json:"dry_run"`
		ReceiverName string             `json:"receiver_name"`
		TemplateID   int64              `json:"template_id"`
		ItemCount    *int               `json:"item_count"`
		StreamCount  *int               `json:"stream_count"`
		TemplateArgs KaringTemplateArgs `json:"template_args"`
		Success      bool               `json:"success"`
		Delivery     string             `json:"delivery"`
		RequestID    string             `json:"requestId"`
		Kind         string             `json:"kind"`
		Duplicate    *bool              `json:"duplicate"`

		AcceptedReceiverName string `json:"receiverName"`
		AcceptedTemplateID   int64  `json:"templateId"`
		AcceptedItemCount    *int   `json:"itemCount"`
		AcceptedStreamCount  *int   `json:"streamCount"`
	}

	var wire karingResponseWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	receiverName := wire.ReceiverName
	if receiverName == "" {
		receiverName = wire.AcceptedReceiverName
	}
	templateID := wire.TemplateID
	if templateID == 0 {
		templateID = wire.AcceptedTemplateID
	}
	itemCount := wire.ItemCount
	if itemCount == nil {
		itemCount = wire.AcceptedItemCount
	}
	streamCount := wire.StreamCount
	if streamCount == nil {
		streamCount = wire.AcceptedStreamCount
	}

	*r = KaringDryRunResponse{
		OK:           wire.OK,
		DryRun:       wire.DryRun,
		ReceiverName: receiverName,
		TemplateID:   templateID,
		ItemCount:    itemCount,
		StreamCount:  streamCount,
		TemplateArgs: wire.TemplateArgs,
		Success:      wire.Success,
		Delivery:     wire.Delivery,
		RequestID:    wire.RequestID,
		Kind:         wire.Kind,
		Duplicate:    wire.Duplicate,
	}

	return nil
}

type KaringClient interface {
	SendKaring(ctx context.Context, req KaringSendRequest) (*KaringDryRunResponse, error)
	SendKaringContentList(ctx context.Context, req KaringContentListRequest) (*KaringDryRunResponse, error)
	SendKaringHololive(ctx context.Context, req KaringHololiveRequest) (*KaringDryRunResponse, error)
}

var _ KaringClient = (*H2CClient)(nil)

func (c *H2CClient) SendKaring(ctx context.Context, req KaringSendRequest) (*KaringDryRunResponse, error) {
	var resp KaringDryRunResponse
	if err := c.postJSON(ctx, PathKaringSend, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris karing: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) SendKaringContentList(ctx context.Context, req KaringContentListRequest) (*KaringDryRunResponse, error) {
	var resp KaringDryRunResponse
	if err := c.postJSON(ctx, PathKaringContentList, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris karing content list: %w", err)
	}
	return &resp, nil
}

func (c *H2CClient) SendKaringHololive(ctx context.Context, req KaringHololiveRequest) (*KaringDryRunResponse, error) {
	var resp KaringDryRunResponse
	if err := c.postJSON(ctx, PathKaringHololive, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris karing hololive: %w", err)
	}
	return &resp, nil
}
