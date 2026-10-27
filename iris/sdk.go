package iris

import (
	"os"

	client "github.com/park285/iris-client-go/v3/internal/client/transport"
)

const (
	envBaseURL  = "IRIS_BASE_URL"
	envBotToken = "IRIS_BOT_TOKEN" // #nosec G101 -- 자격증명이 아니라 환경 변수 이름이다.
)

// NewClient는 옵션을 한 번만 적용해 client를 만든다. WithBaseURL·WithBotToken이 비면
// IRIS_BASE_URL·IRIS_BOT_TOKEN을 읽는다.
func NewClient(opts ...ClientOption) (*APIClient, error) {
	return client.NewClientFromOptions(os.Getenv(envBaseURL), os.Getenv(envBotToken), opts...)
}
