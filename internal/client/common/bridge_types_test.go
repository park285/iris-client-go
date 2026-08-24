package common

import (
	jsonv2 "encoding/json/v2"
	"testing"
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

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	assertBridgeHealthLiveness(t, got)
	assertBridgeHealthChecks(t, got.Checks)
	assertBridgeDiscoveryHooks(t, got.DiscoveryInstallAttempted, got.DiscoveryHooks)

	// Capabilities: 입력에서 생략되면 zero value
	if got.Capabilities.InspectChatRoom.Supported {
		t.Fatal("Capabilities.InspectChatRoom.Supported = true, want false (zero)")
	}

	// Error 필드는 입력에서 생략됨
	if got.Error != nil {
		t.Fatalf("Error = %v, want nil", got.Error)
	}
}

func assertBridgeHealthLiveness(t *testing.T, got BridgeHealthResult) {
	t.Helper()

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
}

func assertBridgeHealthChecks(t *testing.T, checks []BridgeHealthCheck) {
	t.Helper()

	if len(checks) != 2 {
		t.Fatalf("len(Checks) = %d, want 2", len(checks))
	}

	if checks[0].Name != "socket" || !checks[0].OK {
		t.Fatalf("Checks[0] = %+v, unexpected", checks[0])
	}

	if checks[1].Name != "classdex" || checks[1].OK {
		t.Fatalf("Checks[1] = %+v, unexpected", checks[1])
	}

	if checks[1].Detail == nil || *checks[1].Detail != "not found" {
		t.Fatalf("Checks[1].Detail = %v, want not found", checks[1].Detail)
	}
}

func assertBridgeDiscoveryHooks(t *testing.T, attempted bool, hooks []BridgeDiscoveryHook) {
	t.Helper()

	if !attempted {
		t.Fatal("DiscoveryInstallAttempted = false, want true")
	}

	if len(hooks) != 2 {
		t.Fatalf("len(DiscoveryHooks) = %d, want 2", len(hooks))
	}

	assertInstalledDiscoveryHook(t, hooks[0])
	assertFailedDiscoveryHook(t, hooks[1])
}

func assertInstalledDiscoveryHook(t *testing.T, hook BridgeDiscoveryHook) {
	t.Helper()

	if hook.Name != "sendMessage" || !hook.Installed || hook.InvocationCount != 42 {
		t.Fatalf("DiscoveryHooks[0] = %+v, unexpected", hook)
	}

	if hook.LastSeenEpochMs == nil || *hook.LastSeenEpochMs != 1711612800000 {
		t.Fatalf("DiscoveryHooks[0].LastSeenEpochMs = %v, want 1711612800000", hook.LastSeenEpochMs)
	}

	if hook.LastSummary == nil || *hook.LastSummary != "ok" {
		t.Fatalf("DiscoveryHooks[0].LastSummary = %v, want ok", hook.LastSummary)
	}
}

func assertFailedDiscoveryHook(t *testing.T, hook BridgeDiscoveryHook) {
	t.Helper()

	if hook.Name != "readImage" || hook.Installed {
		t.Fatalf("DiscoveryHooks[1] = %+v, unexpected", hook)
	}

	if hook.InstallError == nil || *hook.InstallError != "class not found" {
		t.Fatalf("DiscoveryHooks[1].InstallError = %v, want class not found", hook.InstallError)
	}

	if hook.LastSeenEpochMs != nil {
		t.Fatalf("DiscoveryHooks[1].LastSeenEpochMs = %v, want nil", hook.LastSeenEpochMs)
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

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	capabilities := got.Capabilities

	assertBridgeCapability(t, "InspectChatRoom", capabilities.InspectChatRoom, true, true)
	assertBridgeCapability(t, "OpenChatRoom", capabilities.OpenChatRoom, true, true)
	assertBridgeCapability(t, "SnapshotChatRoomMembers", capabilities.SnapshotChatRoomMembers, true, false)
	assertBridgeCapabilityReason(t, "SnapshotChatRoomMembers", capabilities.SnapshotChatRoomMembers, "bridge version too old")
	assertBridgeCapability(t, "SendText", capabilities.SendText, false, false)
	assertBridgeCapabilityReason(t, "SendText", capabilities.SendText, "text sender unavailable")
	assertBridgeCapability(t, "SendMarkdown", capabilities.SendMarkdown, true, false)
	assertBridgeCapabilityReason(t, "SendMarkdown", capabilities.SendMarkdown, "markdown hook unavailable")
}

func assertBridgeCapability(t *testing.T, label string, got BridgeDiagnosticsCapability, wantSupported, wantReady bool) {
	t.Helper()

	if got.Supported != wantSupported || got.Ready != wantReady {
		t.Fatalf("%s = %+v, want supported=%t ready=%t", label, got, wantSupported, wantReady)
	}
}

func assertBridgeCapabilityReason(t *testing.T, label string, got BridgeDiagnosticsCapability, wantReason string) {
	t.Helper()

	if got.Reason == nil || *got.Reason != wantReason {
		t.Fatalf("%s.Reason = %v, want %s", label, got.Reason, wantReason)
	}
}

func TestNativeCoreDiagnosticsJSON(t *testing.T) {
	raw := `{
		"state": "owned_by_rust_runtime",
		"binaryEnvelopeSchemaVersion": 1,
		"decryptKeyCache": {"hits": 1042, "misses": 37}
	}`

	var got NativeCoreDiagnostics

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
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

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if got.Reachable {
		t.Fatal("Reachable = true, want false")
	}

	if got.Error == nil || *got.Error != "connection refused" {
		t.Fatalf("Error = %v, want connection refused", got.Error)
	}
}
