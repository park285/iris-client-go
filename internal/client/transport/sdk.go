package transport

import "errors"

type SDKConfig struct {
	BaseURL   string
	BotToken  string
	Transport string
}

func ResolveSDKConfig(opts []ClientOption) SDKConfig {
	var o clientOptions

	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	return SDKConfig{BaseURL: o.baseURL, BotToken: o.botToken, Transport: o.Transport}
}

// NewClientFromOptions는 옵션을 한 번만 적용해 client를 만든다. WithBaseURL·WithBotToken 값이 비면
// fallback 값을 쓰고, 끝내 비면 client를 만들지 않고 오류를 반환한다. 초기화에 실패하면 InitError
// 값을 그대로 반환한다.
func NewClientFromOptions(fallbackBaseURL, fallbackBotToken string, opts ...ClientOption) (*APIClient, error) {
	o := applyClientOptions(opts)

	baseURL := firstNonEmpty(o.baseURL, fallbackBaseURL)
	if baseURL == "" {
		return nil, errors.New("iris: base URL is required (set IRIS_BASE_URL or use WithBaseURL)")
	}

	botToken := firstNonEmpty(o.botToken, fallbackBotToken)
	if botToken == "" {
		return nil, errors.New("iris: bot token is required (set IRIS_BOT_TOKEN or use WithBotToken)")
	}

	c := newAPIClient(baseURL, botToken, o)
	if c.initErr != nil {
		return nil, c.initErr
	}

	return c, nil
}
