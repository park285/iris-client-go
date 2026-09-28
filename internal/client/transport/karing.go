package transport

import (
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
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
	// ClientRequestID는 Iris 정본 이름 client_request_id로 보낸다.
	ClientRequestID *string            `json:"client_request_id,omitempty"`
	ReceiverName    string             `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64              `json:"receiver_room_id,omitempty,omitzero"`
	TemplateID      int64              `json:"template_id,omitempty,omitzero"`
	TemplateArgs    KaringTemplateArgs `json:"template_args,omitempty"`
	AppKey          string             `json:"app_key,omitempty"`
	Origin          string             `json:"origin,omitempty"`
	SearchExact     *bool              `json:"search_exact,omitempty"`
	SearchFrom      string             `json:"search_from,omitempty"`
	SearchRoomType  string             `json:"search_room_type,omitempty"`
	DryRun          bool               `json:"dry_run,omitempty,omitzero"`
}

type KaringContentListRequest struct {
	// ClientRequestID의 wire 이름은 KaringSendRequest.ClientRequestID와 같다.
	ClientRequestID *string             `json:"client_request_id,omitempty"`
	Items           []KaringContentItem `json:"items,omitempty"`
	ExtraArgs       KaringTemplateArgs  `json:"extra_args,omitempty"`
	ReceiverName    string              `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64               `json:"receiver_room_id,omitempty,omitzero"`
	TemplateID      int64               `json:"template_id,omitempty,omitzero"`
	SearchExact     *bool               `json:"search_exact,omitempty"`
	SearchFrom      string              `json:"search_from,omitempty"`
	SearchRoomType  string              `json:"search_room_type,omitempty"`
	DryRun          bool                `json:"dry_run,omitempty,omitzero"`
}

// KaringDryRunResponse는 Iris의 camelCase dry-run/live 응답이다.
type KaringDryRunResponse struct {
	OK           bool               `json:"ok"`
	DryRun       bool               `json:"dryRun"`
	ReceiverName string             `json:"receiverName,omitempty"`
	TemplateID   int64              `json:"templateId"`
	ItemCount    *int               `json:"itemCount,omitempty"`
	TemplateArgs KaringTemplateArgs `json:"templateArgs"`
	Success      bool               `json:"success,omitempty,omitzero"`
	Delivery     string             `json:"delivery,omitempty"`
	RequestID    string             `json:"requestId,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Duplicate    *bool              `json:"duplicate,omitempty"`
}

// UnmarshalJSON은 제거된 응답 별칭을 명시적으로 거절한다.
func (r *KaringDryRunResponse) UnmarshalJSON(data []byte) error {
	var fields map[string]jsontext.Value

	if err := jsonv2.Unmarshal(data, &fields); err != nil {
		return fmt.Errorf("decode karing response: %w", err)
	}

	for _, name := range []string{"dry_run", "receiver_name", "template_id", "item_count", "stream_count", "streamCount", "template_args"} {
		if _, found := fields[name]; found {
			return fmt.Errorf("decode karing response: retired field %q", name)
		}
	}

	type canonical KaringDryRunResponse

	if err := jsonv2.Unmarshal(data, (*canonical)(r)); err != nil {
		return fmt.Errorf("decode karing response: %w", err)
	}

	return nil
}

type KaringClient interface {
	SendKaring(ctx context.Context, req KaringSendRequest) (*KaringDryRunResponse, error)
	SendKaringContentList(ctx context.Context, req KaringContentListRequest) (*KaringDryRunResponse, error)
}

var _ KaringClient = (*APIClient)(nil)

func (c *APIClient) SendKaring(ctx context.Context, req KaringSendRequest) (*KaringDryRunResponse, error) {
	resp, err := c.postJSON[KaringDryRunResponse](ctx, PathKaringSend, req, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris karing: %w", err)
	}

	return resp, nil
}

func (c *APIClient) SendKaringContentList(ctx context.Context, req KaringContentListRequest) (*KaringDryRunResponse, error) {
	resp, err := c.postJSON[KaringDryRunResponse](ctx, PathKaringContentList, req, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris karing content list: %w", err)
	}

	return resp, nil
}
