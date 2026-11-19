package transport

import (
	"errors"
	"strings"
)

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
