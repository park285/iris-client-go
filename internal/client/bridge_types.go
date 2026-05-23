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

type BridgeHealthResult struct {
	Reachable                 bool                  `json:"reachable"`
	Running                   bool                  `json:"running"`
	SpecReady                 bool                  `json:"specReady"`
	CheckedAtEpochMs          *int64                `json:"checkedAtEpochMs,omitempty"`
	RestartCount              int                   `json:"restartCount"`
	LastCrashMessage          *string               `json:"lastCrashMessage,omitempty"`
	Checks                    []BridgeHealthCheck   `json:"checks"`
	DiscoveryInstallAttempted bool                  `json:"discoveryInstallAttempted"`
	DiscoveryHooks            []BridgeDiscoveryHook `json:"discoveryHooks"`
	Error                     *string               `json:"error,omitempty"`
}
