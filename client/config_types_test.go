package client

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
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
		"discovered": {"bot_id": 42},
		"pending_restart": {"required": true, "fields": ["bot_http_port"]}
	}`

	var got ConfigResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.User.BotName != "iris" {
		t.Fatalf("User.BotName = %q, want iris", got.User.BotName)
	}
	if got.User.WebEndpoint != "http://localhost:8080" {
		t.Fatalf("User.WebEndpoint = %q, want http://localhost:8080", got.User.WebEndpoint)
	}
	if got.User.Webhooks["default"] != "http://hook.test" {
		t.Fatalf("User.Webhooks[default] = %q, want http://hook.test", got.User.Webhooks["default"])
	}
	if got.User.BotHTTPPort != 3000 {
		t.Fatalf("User.BotHTTPPort = %d, want 3000", got.User.BotHTTPPort)
	}
	if got.User.DBPollingRate != 500 {
		t.Fatalf("User.DBPollingRate = %d, want 500", got.User.DBPollingRate)
	}
	if got.User.MessageSendRate != 100 {
		t.Fatalf("User.MessageSendRate = %d, want 100", got.User.MessageSendRate)
	}
	if len(got.User.CommandRoutePrefixes["!"]) != 1 || got.User.CommandRoutePrefixes["!"][0] != "default" {
		t.Fatalf("User.CommandRoutePrefixes = %v, unexpected", got.User.CommandRoutePrefixes)
	}
	if len(got.User.ImageMessageTypeRoutes["photo"]) != 1 || got.User.ImageMessageTypeRoutes["photo"][0] != "img-handler" {
		t.Fatalf("User.ImageMessageTypeRoutes = %v, unexpected", got.User.ImageMessageTypeRoutes)
	}

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

func TestConfigUpdateResponseJSON(t *testing.T) {
	raw := `{
		"success": true,
		"name": "web_endpoint",
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
		"discovered": {"bot_id": 42},
		"pending_restart": {"required": false, "fields": []}
	}`

	var got ConfigUpdateResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Success {
		t.Fatal("Success = false, want true")
	}
	if got.Name != "web_endpoint" {
		t.Fatalf("Name = %q, want web_endpoint", got.Name)
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := jsonx.Marshal(tt.input)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}
			if string(got) != tt.wantJSON {
				t.Fatalf("Marshal() = %s, want %s", got, tt.wantJSON)
			}
		})
	}
}
