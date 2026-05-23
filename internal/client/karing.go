package client

import (
	"context"
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

type KaringHololiveStream = KaringContentItem

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
	ClientRequestID *string                `json:"clientRequestId,omitempty"`
	Stream          *KaringHololiveStream  `json:"stream,omitempty"`
	Streams         []KaringHololiveStream `json:"streams,omitempty"`
	ExtraArgs       KaringTemplateArgs     `json:"extra_args,omitempty"`
	ReceiverName    string                 `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64                  `json:"receiver_room_id,omitempty"`
	TemplateID      int64                  `json:"template_id,omitempty"`
	SearchExact     *bool                  `json:"search_exact,omitempty"`
	SearchFrom      string                 `json:"search_from,omitempty"`
	SearchRoomType  string                 `json:"search_room_type,omitempty"`
	DryRun          bool                   `json:"dry_run,omitempty"`
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
