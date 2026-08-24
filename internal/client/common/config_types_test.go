package common

import (
	jsonv2 "encoding/json/v2"
	"testing"
)

func TestConfigResponseJSON(t *testing.T) {
	raw := `{
		"user": {
			"bot_name": "iris",
			"web_endpoint": "http://localhost:8080",
			"webhooks": {"default": "http://hook.test"},
			"bot_http_port": 3000,
			"db_polling_rate": 500,
			"message_send_rate": 100,
			"command_route_prefixes": {"!": ["default"]},
			"image_message_type_routes": {"photo": ["img-handler"]}
		},
		"applied": {
			"bot_name": "iris",
			"web_endpoint": "http://localhost:8080",
			"webhooks": {"default": "http://hook.test"},
			"bot_http_port": 3000,
			"db_polling_rate": 500,
			"message_send_rate": 100,
			"command_route_prefixes": {"!": ["default"]},
			"image_message_type_routes": {"photo": ["img-handler"]}
		},
		"discovered": {"botId": 42},
		"pending_restart": {"required": true, "fields": ["bot_http_port"]}
	}`

	var got ConfigResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertConfigUserState(t, got.User)

	if got.Applied.BotName != "iris" {
		t.Fatalf("Applied.BotName = %q, want iris", got.Applied.BotName)
	}

	if got.Discovered.BotID != 42 {
		t.Fatalf("Discovered.BotID = %d, want 42", got.Discovered.BotID)
	}

	if !got.PendingRestart.Required {
		t.Fatal("PendingRestart.Required = false, want true")
	}

	if len(got.PendingRestart.Fields) != 1 || got.PendingRestart.Fields[0] != "bot_http_port" {
		t.Fatalf("PendingRestart.Fields = %v, want [bot_http_port]", got.PendingRestart.Fields)
	}
}

func assertConfigUserState(t *testing.T, user ConfigState) {
	t.Helper()

	if user.BotName != "iris" {
		t.Fatalf("User.BotName = %q, want iris", user.BotName)
	}

	if user.WebEndpoint != "http://localhost:8080" {
		t.Fatalf("User.WebEndpoint = %q, want http://localhost:8080", user.WebEndpoint)
	}

	if user.Webhooks["default"] != "http://hook.test" {
		t.Fatalf("User.Webhooks[default] = %q, want http://hook.test", user.Webhooks["default"])
	}

	if user.BotHTTPPort != 3000 {
		t.Fatalf("User.BotHTTPPort = %d, want 3000", user.BotHTTPPort)
	}

	if user.DBPollingRate != 500 {
		t.Fatalf("User.DBPollingRate = %d, want 500", user.DBPollingRate)
	}

	if user.MessageSendRate != 100 {
		t.Fatalf("User.MessageSendRate = %d, want 100", user.MessageSendRate)
	}

	if len(user.CommandRoutePrefixes["!"]) != 1 || user.CommandRoutePrefixes["!"][0] != "default" {
		t.Fatalf("User.CommandRoutePrefixes = %v, unexpected", user.CommandRoutePrefixes)
	}

	if len(user.ImageMessageTypeRoutes["photo"]) != 1 || user.ImageMessageTypeRoutes["photo"][0] != "img-handler" {
		t.Fatalf("User.ImageMessageTypeRoutes = %v, unexpected", user.ImageMessageTypeRoutes)
	}
}

func TestConfigUpdateResponseJSON(t *testing.T) {
	raw := `{
		"success": true,
		"name": "endpoint",
		"persisted": true,
		"applied": true,
		"requiresRestart": false,
		"user": {
			"bot_name": "iris",
			"web_endpoint": "http://new:8080",
			"webhooks": {},
			"bot_http_port": 3000,
			"db_polling_rate": 500,
			"message_send_rate": 100,
			"command_route_prefixes": {},
			"image_message_type_routes": {}
		},
		"runtimeApplied": {
			"bot_name": "iris",
			"web_endpoint": "http://new:8080",
			"webhooks": {},
			"bot_http_port": 3000,
			"db_polling_rate": 500,
			"message_send_rate": 100,
			"command_route_prefixes": {},
			"image_message_type_routes": {}
		},
		"discovered": {"botId": 42},
		"pending_restart": {"required": false, "fields": []}
	}`

	var got ConfigUpdateResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Success {
		t.Fatal("Success = false, want true")
	}

	if got.Name != "endpoint" {
		t.Fatalf("Name = %q, want endpoint", got.Name)
	}

	if !got.Persisted {
		t.Fatal("Persisted = false, want true")
	}

	if !got.Applied {
		t.Fatal("Applied = false, want true")
	}

	if got.RequiresRestart {
		t.Fatal("RequiresRestart = true, want false")
	}

	if got.User.WebEndpoint != "http://new:8080" {
		t.Fatalf("User.WebEndpoint = %q, want http://new:8080", got.User.WebEndpoint)
	}

	if got.RuntimeApplied.WebEndpoint != "http://new:8080" {
		t.Fatalf("RuntimeApplied.WebEndpoint = %q, want http://new:8080", got.RuntimeApplied.WebEndpoint)
	}

	if got.Discovered.BotID != 42 {
		t.Fatalf("Discovered.BotID = %d, want 42", got.Discovered.BotID)
	}
}

func TestConfigUpdateRequestJSON(t *testing.T) {
	endpoint := "http://new:8080"
	rate := int64(200)
	expectedRevision := uint64(42)
	forwardUnmatched := true

	tests := []struct {
		name     string
		input    ConfigUpdateRequest
		wantJSON string
	}{
		{
			name:     "empty request omits all fields",
			input:    ConfigUpdateRequest{},
			wantJSON: `{}`,
		},
		{
			name:     "only endpoint",
			input:    ConfigUpdateRequest{Endpoint: &endpoint},
			wantJSON: `{"endpoint":"http://new:8080"}`,
		},
		{
			name:     "endpoint and rate",
			input:    ConfigUpdateRequest{Endpoint: &endpoint, Rate: &rate},
			wantJSON: `{"endpoint":"http://new:8080","rate":200}`,
		},
		{
			name: "routes and expected revision",
			input: ConfigUpdateRequest{
				CommandRoutePrefixes:              map[string][]string{"chatbot": {"!", "/"}},
				ImageMessageTypeRoutes:            map[string][]string{"images": {"2", "23"}},
				EventTypeRoutes:                   map[string][]string{"events": {"member_nickname_updated"}},
				ForwardUnmatchedMessagesToDefault: &forwardUnmatched,
				ExpectedRevision:                  &expectedRevision,
			},
			wantJSON: `{"commandRoutePrefixes":{"chatbot":["!","/"]},"imageMessageTypeRoutes":{"images":["2","23"]},"eventTypeRoutes":{"events":["member_nickname_updated"]},"forwardUnmatchedMessagesToDefault":true,"expectedRevision":42}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonv2.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			if string(got) != tt.wantJSON {
				t.Fatalf("Marshal() = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}
