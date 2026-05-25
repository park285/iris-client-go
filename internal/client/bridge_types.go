package client

type BridgeHealthCheck struct {
	Name   string  `json:"name"`
	OK     bool    `json:"ok"`
	Detail *string `json:"detail,omitempty"`
}

type BridgeDiscoveryHook struct {
	Name            string  `json:"name"`
	Installed       bool    `json:"installed"`
	InstallError    *string `json:"installError,omitempty"`
	InvocationCount int     `json:"invocationCount"`
	LastSeenEpochMs *int64  `json:"lastSeenEpochMs,omitempty"`
	LastSummary     *string `json:"lastSummary,omitempty"`
}

type BridgeDiagnosticsCapability struct {
	Supported bool    `json:"supported"`
	Ready     bool    `json:"ready"`
	Reason    *string `json:"reason,omitempty"`
}

type BridgeDiagnosticsCapabilities struct {
	InspectChatRoom         BridgeDiagnosticsCapability `json:"inspectChatRoom"`
	SnapshotChatRoomMembers BridgeDiagnosticsCapability `json:"snapshotChatRoomMembers"`
}

type BridgeHealthResult struct {
	Reachable                 bool                          `json:"reachable"`
	Running                   bool                          `json:"running"`
	SpecReady                 bool                          `json:"specReady"`
	CheckedAtEpochMs          *int64                        `json:"checkedAtEpochMs,omitempty"`
	RestartCount              int                           `json:"restartCount"`
	LastCrashMessage          *string                       `json:"lastCrashMessage,omitempty"`
	Checks                    []BridgeHealthCheck           `json:"checks"`
	DiscoveryInstallAttempted bool                          `json:"discoveryInstallAttempted"`
	DiscoveryHooks            []BridgeDiscoveryHook         `json:"discoveryHooks"`
	Capabilities              BridgeDiagnosticsCapabilities `json:"capabilities"`
	Error                     *string                       `json:"error,omitempty"`
}

type KeyCacheStats struct {
	Hits   uint64 `json:"hits"`
	Misses uint64 `json:"misses"`
}

type NativeCoreDiagnostics struct {
	State                       string        `json:"state"`
	BinaryEnvelopeSchemaVersion int           `json:"binaryEnvelopeSchemaVersion"`
	DecryptKeyCache             KeyCacheStats `json:"decryptKeyCache"`
}

type TextPingWarmResponse struct {
	Accepted       bool  `json:"accepted"`
	ChatID         int64 `json:"chatId"`
	WarmQueueDepth int   `json:"warmQueueDepth"`
}
