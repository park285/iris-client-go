package transport

import (
	"errors"
	"strings"
)

// NewClient는 옵션을 한 번만 적용해 client를 만든다. SDK는 환경 변수를 읽지 않으므로 Base URL과
// bot-control 자격은 WithBaseURL·WithBotToken으로만 받는다. 설정이 비었거나 Base URL·transport·CA가
// 잘못되면 client를 만들지 않고 오류를 반환한다.
func NewClient(opts ...ClientOption) (*APIClient, error) {
	o := applyClientOptions(opts)

	baseURL := strings.TrimSpace(o.baseURL)
	if baseURL == "" {
		return nil, errors.New("iris: base URL is required (use WithBaseURL)")
	}

	botToken := strings.TrimSpace(o.botToken)
	if botToken == "" {
		return nil, errors.New("iris: bot token is required (use WithBotToken)")
	}

	return newAPIClient(baseURL, botToken, o)
}
