package transport

import (
	"cmp"
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
	// ClientRequestID는 Iris 정본 이름 client_request_id로 보낸다. Iris는 c3069c08부터 이 이름을 받으며,
	// 예전 tag인 clientRequestId는 제거 예정인 전환 alias로만 받는다
	// (DEC-20260926-iris-karing-contract-aliases-retirement).
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
	ClientRequestID *string `json:"client_request_id,omitempty"`
	// Item은 wire key item으로 한 항목을 보낸다.
	//
	// Deprecated: Iris는 Karing 요청의 item 키를 삭제했고 알 수 없는 필드를 400으로 거절한다
	// (DEC-20260926-iris-karing-contract-aliases-retirement). 한 항목도 Items에 길이 1인 목록으로
	// 넣는다: Items: []KaringContentItem{item}. 다음 coordinated major에서 삭제한다.
	Item           *KaringContentItem  `json:"item,omitempty"`
	Items          []KaringContentItem `json:"items,omitempty"`
	ExtraArgs      KaringTemplateArgs  `json:"extra_args,omitempty"`
	ReceiverName   string              `json:"receiver_name,omitempty"`
	ReceiverRoomID int64               `json:"receiver_room_id,omitempty,omitzero"`
	TemplateID     int64               `json:"template_id,omitempty,omitzero"`
	SearchExact    *bool               `json:"search_exact,omitempty"`
	SearchFrom     string              `json:"search_from,omitempty"`
	SearchRoomType string              `json:"search_room_type,omitempty"`
	DryRun         bool                `json:"dry_run,omitempty,omitzero"`
}

// KaringHololiveRequest는 SendKaringHololive의 요청이다. 공개 폐기 표시는 iris facade의 같은 이름
// alias에 있다(facade가 이 타입을 재노출하므로 여기에 표시하면 facade 선언이 스스로 경고를 낸다).
type KaringHololiveRequest struct {
	// ClientRequestID의 wire 이름은 KaringSendRequest.ClientRequestID와 같다.
	ClientRequestID *string             `json:"client_request_id,omitempty"`
	Stream          *KaringContentItem  `json:"stream,omitempty"`
	Streams         []KaringContentItem `json:"streams,omitempty"`
	ExtraArgs       KaringTemplateArgs  `json:"extra_args,omitempty"`
	ReceiverName    string              `json:"receiver_name,omitempty"`
	ReceiverRoomID  int64               `json:"receiver_room_id,omitempty,omitzero"`
	TemplateID      int64               `json:"template_id,omitempty,omitzero"`
	SearchExact     *bool               `json:"search_exact,omitempty"`
	SearchFrom      string              `json:"search_from,omitempty"`
	SearchRoomType  string              `json:"search_room_type,omitempty"`
	DryRun          bool                `json:"dry_run,omitempty,omitzero"`
}

// KaringDryRunResponse는 Karing 발송의 dry-run 응답과 live 202 응답을 함께 담는다. 현재 Iris는 두
// 응답 모두 camelCase로 보내고 개수는 itemCount 하나만 보낸다. 필드 tag는 v2 Marshal 출력을 바꾸지
// 않도록 그대로 두며, decode는 UnmarshalJSON이 소유한다.
type KaringDryRunResponse struct {
	OK           bool   `json:"ok"`
	DryRun       bool   `json:"dry_run"`
	ReceiverName string `json:"receiver_name,omitempty"`
	TemplateID   int64  `json:"template_id"`
	ItemCount    *int   `json:"item_count,omitempty"`
	// StreamCount는 이전 Iris가 /karing/content-list와 /karing/hololive 응답에 itemCount와 같은 값으로
	// 함께 보내던 stream_count(dry-run)·streamCount(live)다. 현재 Iris는 이 값을 보내지 않으므로 nil로
	// 남고, SDK가 ItemCount에서 채우지 않는다.
	//
	// Deprecated: 항목 개수는 ItemCount로 읽는다. 다음 coordinated major에서 삭제한다.
	StreamCount  *int               `json:"stream_count,omitempty"`
	TemplateArgs KaringTemplateArgs `json:"template_args"`
	Success      bool               `json:"success,omitempty,omitzero"`
	Delivery     string             `json:"delivery,omitempty"`
	RequestID    string             `json:"requestId,omitempty"`
	Kind         string             `json:"kind,omitempty"`
	Duplicate    *bool              `json:"duplicate,omitempty"`
}

// karingResponseField는 null과 영값도 키가 존재한 것으로 보존한다. 별도 raw JSON 사본 없이
// 한 번의 decode에서 존재 여부를 기록해 이전 wire key가 정본 값을 덮어쓰지 못하게 한다.
type karingResponseField[T any] struct {
	value   T
	present bool
}

func (f *karingResponseField[T]) UnmarshalJSONFrom(dec *jsontext.Decoder) error {
	f.present = true

	if err := jsonv2.UnmarshalDecode(dec, &f.value); err != nil {
		return fmt.Errorf("decode canonical karing field: %w", err)
	}

	return nil
}

func (f karingResponseField[T]) valueOr(legacy T) T {
	if f.present {
		return f.value
	}

	return legacy
}

// UnmarshalJSON은 현재 Iris의 camelCase 응답(dryRun, receiverName, templateId, itemCount,
// templateArgs)을 정본으로 읽는다.
//
// 폐기 예정 호환 입력: 이전 Iris의 snake_case dry-run 응답(dry_run, receiver_name, template_id,
// item_count, stream_count, template_args)과 live 응답의 streamCount는 정본 key가 없을 때만 읽는다.
// SDK v2가 이전 Iris에도 붙을 수 있게 남긴 decode 계약이며, 이 입력과 병합은 다음 coordinated
// major에서 삭제한다(DEC-20260926-iris-karing-contract-aliases-retirement,
// DEC-20260825-iris-client-go-public-surface-major-only).
func (r *KaringDryRunResponse) UnmarshalJSON(data []byte) error {
	type karingResponseWire struct {
		OK           bool                                    `json:"ok"`
		DryRun       karingResponseField[bool]               `json:"dryRun"`
		ReceiverName karingResponseField[string]             `json:"receiverName"`
		TemplateID   karingResponseField[int64]              `json:"templateId"`
		ItemCount    karingResponseField[*int]               `json:"itemCount"`
		TemplateArgs karingResponseField[KaringTemplateArgs] `json:"templateArgs"`
		Success      bool                                    `json:"success"`
		Delivery     string                                  `json:"delivery"`
		RequestID    string                                  `json:"requestId"`
		Kind         string                                  `json:"kind"`
		Duplicate    *bool                                   `json:"duplicate"`

		LegacyDryRun              bool               `json:"dry_run"`
		LegacyReceiverName        string             `json:"receiver_name"`
		LegacyTemplateID          int64              `json:"template_id"`
		LegacyItemCount           *int               `json:"item_count"`
		LegacyStreamCount         *int               `json:"stream_count"`
		LegacyAcceptedStreamCount *int               `json:"streamCount"`
		LegacyTemplateArgs        KaringTemplateArgs `json:"template_args"`
	}

	var wire karingResponseWire

	if err := jsonv2.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("decode karing dry-run response: %w", err)
	}

	*r = KaringDryRunResponse{
		OK:           wire.OK,
		DryRun:       wire.DryRun.valueOr(wire.LegacyDryRun),
		ReceiverName: wire.ReceiverName.valueOr(wire.LegacyReceiverName),
		TemplateID:   wire.TemplateID.valueOr(wire.LegacyTemplateID),
		ItemCount:    wire.ItemCount.valueOr(wire.LegacyItemCount),
		StreamCount:  cmp.Or(wire.LegacyStreamCount, wire.LegacyAcceptedStreamCount),
		TemplateArgs: wire.TemplateArgs.valueOr(wire.LegacyTemplateArgs),
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
	// Deprecated: Iris가 /karing/hololive route를 삭제했다. SendKaringContentList를 쓴다
	// (APIClient.SendKaringHololive의 이관 안내). 다음 coordinated major에서 삭제한다.
	SendKaringHololive(ctx context.Context, req KaringHololiveRequest) (*KaringDryRunResponse, error)
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

// SendKaringHololive는 POST /karing/hololive로 보낸다.
//
// Deprecated: Iris가 /karing/content-list의 alias route인 /karing/hololive를 삭제했으므로 삭제된
// Iris에서는 404 *HTTPError를 받는다(DEC-20260926-iris-karing-contract-aliases-retirement).
// SendKaringContentList로 옮기고 KaringHololiveRequest의 Stream·Streams는
// KaringContentListRequest.Items로 옮긴다(한 항목도 길이 1인 목록). 나머지 필드는 이름과 의미가
// 같다. 다음 coordinated major에서 삭제한다.
func (c *APIClient) SendKaringHololive(ctx context.Context, req KaringHololiveRequest) (*KaringDryRunResponse, error) {
	resp, err := c.postJSON[KaringDryRunResponse](ctx, PathKaringHololive, req, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("send iris karing hololive: %w", err)
	}

	return resp, nil
}
