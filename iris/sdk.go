package iris

import client "github.com/park285/iris-client-go/v4/internal/client/transport"

// NewClient는 옵션을 한 번만 적용해 client를 만든다. SDK는 환경 변수를 읽지 않으므로 Base URL과
// bot-control 자격은 WithBaseURL·WithBotToken으로, h3 CA는 WithH3CACertFile로 넘긴다. 설정이 비었거나
// Base URL·transport·CA가 잘못되면 client를 만들지 않고 오류를 반환한다.
func NewClient(opts ...ClientOption) (*APIClient, error) {
	return client.NewClient(opts...)
}
