package client

// SDKConfig holds SDK-level settings extracted from ClientOption.
// Used by iris.NewClient to resolve base URL and bot token.
type SDKConfig struct {
	BaseURL  string
	BotToken string
}

// ResolveSDKConfig applies options and extracts SDK-level config.
func ResolveSDKConfig(opts []ClientOption) SDKConfig {
	var o clientOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return SDKConfig{BaseURL: o.baseURL, BotToken: o.botToken}
}
