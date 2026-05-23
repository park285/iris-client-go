package client

type ConfigState struct {
	BotName                string              `json:"bot_name"`
	WebEndpoint            string              `json:"web_endpoint"`
	Webhooks               map[string]string   `json:"webhooks"`
	BotHTTPPort            int                 `json:"bot_http_port"`
	DBPollingRate          int64               `json:"db_polling_rate"`
	MessageSendRate        int64               `json:"message_send_rate"`
	CommandRoutePrefixes   map[string][]string `json:"command_route_prefixes"`
	ImageMessageTypeRoutes map[string][]string `json:"image_message_type_routes"`
}

type ConfigDiscoveredState struct {
	BotID int64 `json:"bot_id"`
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
	Endpoint *string `json:"endpoint,omitempty"`
	Route    *string `json:"route,omitempty"`
	Rate     *int64  `json:"rate,omitempty"`
	Port     *int    `json:"port,omitempty"`
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
