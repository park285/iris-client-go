package client

import (
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestBridgeHealthResultJSON(t *testing.T) {
	raw := `{
		"reachable": true,
		"running": true,
		"specReady": true,
		"checkedAtEpochMs": 1711612800000,
		"restartCount": 2,
		"lastCrashMessage": "OOM",
		"checks": [
			{"name": "socket", "ok": true},
			{"name": "classdex", "ok": false, "detail": "not found"}
		],
		"discoveryInstallAttempted": true,
		"discoveryHooks": [
			{
				"name": "sendMessage",
				"installed": true,
				"invocationCount": 42,
				"lastSeenEpochMs": 1711612800000,
				"lastSummary": "ok"
			},
			{
				"name": "readImage",
				"installed": false,
				"installError": "class not found",
				"invocationCount": 0
			}
		]
	}`

	var got BridgeHealthResult
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Reachable {
		t.Fatal("Reachable = false, want true")
	}
	if !got.Running {
		t.Fatal("Running = false, want true")
	}
	if !got.SpecReady {
		t.Fatal("SpecReady = false, want true")
	}
	if got.CheckedAtEpochMs == nil || *got.CheckedAtEpochMs != 1711612800000 {
		t.Fatalf("CheckedAtEpochMs = %v, want 1711612800000", got.CheckedAtEpochMs)
	}
	if got.RestartCount != 2 {
		t.Fatalf("RestartCount = %d, want 2", got.RestartCount)
	}
	if got.LastCrashMessage == nil || *got.LastCrashMessage != "OOM" {
		t.Fatalf("LastCrashMessage = %v, want OOM", got.LastCrashMessage)
	}

	// Checks
	if len(got.Checks) != 2 {
		t.Fatalf("len(Checks) = %d, want 2", len(got.Checks))
	}
	if got.Checks[0].Name != "socket" || !got.Checks[0].OK {
		t.Fatalf("Checks[0] = %+v, unexpected", got.Checks[0])
	}
	if got.Checks[1].Name != "classdex" || got.Checks[1].OK {
		t.Fatalf("Checks[1] = %+v, unexpected", got.Checks[1])
	}
	if got.Checks[1].Detail == nil || *got.Checks[1].Detail != "not found" {
		t.Fatalf("Checks[1].Detail = %v, want not found", got.Checks[1].Detail)
	}

	// Discovery hooks
	if !got.DiscoveryInstallAttempted {
		t.Fatal("DiscoveryInstallAttempted = false, want true")
	}
	if len(got.DiscoveryHooks) != 2 {
		t.Fatalf("len(DiscoveryHooks) = %d, want 2", len(got.DiscoveryHooks))
	}
	h0 := got.DiscoveryHooks[0]
	if h0.Name != "sendMessage" || !h0.Installed || h0.InvocationCount != 42 {
		t.Fatalf("DiscoveryHooks[0] = %+v, unexpected", h0)
	}
	if h0.LastSeenEpochMs == nil || *h0.LastSeenEpochMs != 1711612800000 {
		t.Fatalf("DiscoveryHooks[0].LastSeenEpochMs = %v, want 1711612800000", h0.LastSeenEpochMs)
	}
	if h0.LastSummary == nil || *h0.LastSummary != "ok" {
		t.Fatalf("DiscoveryHooks[0].LastSummary = %v, want ok", h0.LastSummary)
	}

	h1 := got.DiscoveryHooks[1]
	if h1.Name != "readImage" || h1.Installed {
		t.Fatalf("DiscoveryHooks[1] = %+v, unexpected", h1)
	}
	if h1.InstallError == nil || *h1.InstallError != "class not found" {
		t.Fatalf("DiscoveryHooks[1].InstallError = %v, want class not found", h1.InstallError)
	}
	if h1.LastSeenEpochMs != nil {
		t.Fatalf("DiscoveryHooks[1].LastSeenEpochMs = %v, want nil", h1.LastSeenEpochMs)
	}

	// Capabilities (absent → zero value)
	if got.Capabilities.InspectChatRoom.Supported {
		t.Fatal("Capabilities.InspectChatRoom.Supported = true, want false (zero)")
	}

	// Error field omitted
	if got.Error != nil {
		t.Fatalf("Error = %v, want nil", got.Error)
	}
}

func TestBridgeHealthResultWithCapabilitiesJSON(t *testing.T) {
	raw := `{
		"reachable": true,
		"running": true,
		"specReady": true,
		"restartCount": 0,
		"checks": [],
		"discoveryInstallAttempted": false,
		"discoveryHooks": [],
		"capabilities": {
			"inspectChatRoom": {"supported": true, "ready": true},
			"openChatRoom": {"supported": true, "ready": true},
			"snapshotChatRoomMembers": {"supported": true, "ready": false, "reason": "bridge version too old"},
			"sendText": {"supported": false, "ready": false, "reason": "text sender unavailable"},
			"sendMarkdown": {"supported": true, "ready": false, "reason": "markdown hook unavailable"}
		}
	}`

	var got BridgeHealthResult
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Capabilities.InspectChatRoom.Supported || !got.Capabilities.InspectChatRoom.Ready {
		t.Fatalf("InspectChatRoom = %+v, want supported=true ready=true", got.Capabilities.InspectChatRoom)
	}
	if !got.Capabilities.OpenChatRoom.Supported || !got.Capabilities.OpenChatRoom.Ready {
		t.Fatalf("OpenChatRoom = %+v, want supported=true ready=true", got.Capabilities.OpenChatRoom)
	}
	snap := got.Capabilities.SnapshotChatRoomMembers
	if !snap.Supported || snap.Ready {
		t.Fatalf("SnapshotChatRoomMembers = %+v, want supported=true ready=false", snap)
	}
	if snap.Reason == nil || *snap.Reason != "bridge version too old" {
		t.Fatalf("SnapshotChatRoomMembers.Reason = %v, want bridge version too old", snap.Reason)
	}
	sendText := got.Capabilities.SendText
	if sendText.Supported || sendText.Ready {
		t.Fatalf("SendText = %+v, want supported=false ready=false", sendText)
	}
	if sendText.Reason == nil || *sendText.Reason != "text sender unavailable" {
		t.Fatalf("SendText.Reason = %v, want text sender unavailable", sendText.Reason)
	}
	sendMarkdown := got.Capabilities.SendMarkdown
	if !sendMarkdown.Supported || sendMarkdown.Ready {
		t.Fatalf("SendMarkdown = %+v, want supported=true ready=false", sendMarkdown)
	}
	if sendMarkdown.Reason == nil || *sendMarkdown.Reason != "markdown hook unavailable" {
		t.Fatalf("SendMarkdown.Reason = %v, want markdown hook unavailable", sendMarkdown.Reason)
	}
}

func TestNativeCoreDiagnosticsJSON(t *testing.T) {
	raw := `{
		"state": "owned_by_rust_runtime",
		"binaryEnvelopeSchemaVersion": 1,
		"decryptKeyCache": {"hits": 1042, "misses": 37}
	}`

	var got NativeCoreDiagnostics
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.State != "owned_by_rust_runtime" {
		t.Fatalf("State = %q, want owned_by_rust_runtime", got.State)
	}
	if got.BinaryEnvelopeSchemaVersion != 1 {
		t.Fatalf("BinaryEnvelopeSchemaVersion = %d, want 1", got.BinaryEnvelopeSchemaVersion)
	}
	if got.DecryptKeyCache.Hits != 1042 {
		t.Fatalf("DecryptKeyCache.Hits = %d, want 1042", got.DecryptKeyCache.Hits)
	}
	if got.DecryptKeyCache.Misses != 37 {
		t.Fatalf("DecryptKeyCache.Misses = %d, want 37", got.DecryptKeyCache.Misses)
	}
}

func TestBridgeHealthResultWithErrorJSON(t *testing.T) {
	raw := `{
		"reachable": false,
		"running": false,
		"specReady": false,
		"restartCount": 0,
		"checks": [],
		"discoveryInstallAttempted": false,
		"discoveryHooks": [],
		"error": "connection refused"
	}`

	var got BridgeHealthResult
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Reachable {
		t.Fatal("Reachable = true, want false")
	}
	if got.Error == nil || *got.Error != "connection refused" {
		t.Fatalf("Error = %v, want connection refused", got.Error)
	}
}
