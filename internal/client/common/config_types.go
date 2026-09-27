package common

type ConfigState struct {
	BotName string `json:"bot_name"`
	// Deprecated: Webhooks["default"]를 사용하십시오. Iris의 endpoint 단일화 release는
	// web_endpoint를 보내지 않으며, 이 필드는 그때 빈 문자열로 decode됩니다.
	// 공개 필드는 다음 coordinated major까지 유지합니다.
	WebEndpoint                string              `json:"web_endpoint"`
	Webhooks                   map[string]string   `json:"webhooks"`
	BotHTTPPort                int                 `json:"bot_http_port"`
	DBPollingRate              int64               `json:"db_polling_rate"`
	ChatLogInvalidationEnabled bool                `json:"chat_log_invalidation_enabled"`
	MessageSendRate            int64               `json:"message_send_rate"`
	ReplyImageDir              string              `json:"reply_image_dir"`
	CommandRoutePrefixes       map[string][]string `json:"command_route_prefixes"`
	ImageMessageTypeRoutes     map[string][]string `json:"image_message_type_routes"`
	EventTypeRoutes            map[string][]string `json:"event_type_routes"`
}

type ConfigDiscoveredState struct {
	// BotID는 Iris가 보고한 effective bot id(discovered.botId)다. Iris는 active source 계정 스냅샷을
	// 얻지 못하면(source unavailable) botId를 null로 보내고, 이 필드는 그때 0으로 decode된다. NoAccount
	// 세대의 0 sentinel도 0이므로 0은 "확인된 bot id 없음"으로 다룬다. 두 경우를 구별하는 타입 변경은
	// 공개 필드 타입 변경이라 coordinated major에서만 검토한다
	// (DEC-20260825-iris-client-go-public-surface-major-only).
	BotID int64 `json:"botId"`
}

type ConfigPendingRestart struct {
	Required bool     `json:"required"`
	Fields   []string `json:"fields"`
}

type ConfigResponse struct {
	User           ConfigState           `json:"user"`
	Applied        ConfigState           `json:"applied"`
	Discovered     ConfigDiscoveredState `json:"discovered"`
	PendingRestart ConfigPendingRestart  `json:"pending_restart"`
}

type ConfigUpdateRequest struct {
	Endpoint                          *string             `json:"endpoint,omitempty"`
	Route                             *string             `json:"route,omitempty"`
	Rate                              *int64              `json:"rate,omitempty"`
	Port                              *int                `json:"port,omitempty"`
	CommandRoutePrefixes              map[string][]string `json:"commandRoutePrefixes,omitempty"`
	ImageMessageTypeRoutes            map[string][]string `json:"imageMessageTypeRoutes,omitempty"`
	EventTypeRoutes                   map[string][]string `json:"eventTypeRoutes,omitempty"`
	ForwardUnmatchedMessagesToDefault *bool               `json:"forwardUnmatchedMessagesToDefault,omitempty"`
	ExpectedRevision                  *uint64             `json:"expectedRevision,omitempty"`
}

type ConfigUpdateResponse struct {
	Success         bool                  `json:"success"`
	Name            string                `json:"name"`
	Persisted       bool                  `json:"persisted"`
	Applied         bool                  `json:"applied"`
	RequiresRestart bool                  `json:"requiresRestart"`
	User            ConfigState           `json:"user"`
	RuntimeApplied  ConfigState           `json:"runtimeApplied"`
	Discovered      ConfigDiscoveredState `json:"discovered"`
	PendingRestart  ConfigPendingRestart  `json:"pending_restart"`
}
