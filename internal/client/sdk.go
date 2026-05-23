package client

type SDKConfig struct {
	BaseURL  string
	BotToken string
}

func ResolveSDKConfig(opts []ClientOption) SDKConfig {
	var o clientOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	return SDKConfig{BaseURL: o.baseURL, BotToken: o.botToken}
}
