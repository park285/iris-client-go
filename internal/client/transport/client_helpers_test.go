package transport

import "context"

// mustNewAPIClient는 기존 서명·응답 단위 테스트에 고정 epoch 캐시를 주입한다.
// 임의 test server를 위한 probe 제한 해제와 분리해, 실제 discovery 회귀는 NewClient를 쓴다.
func mustNewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	c, err := newAPIClient(baseURL, botToken, applyClientOptions(opts))
	if err != nil {
		panic(err)
	}

	c.http1ProbeOnly = false
	c.epoch.value = testAuthEpoch

	return c
}

// sendMessage는 본문을 쓰지 않는 테스트에서 텍스트 reply 결과 error만 본다. SDK의 텍스트 발송 경로는
// SendMessageAccepted 하나다.
func sendMessage(ctx context.Context, c *APIClient, room, message string, opts ...SendOption) error {
	_, err := c.SendMessageAccepted(ctx, room, message, opts...)

	return err
}

func mustDetectImageContentType(data []byte) string {
	contentType, err := detectImageContentType(data)
	if err != nil {
		panic(err)
	}

	return contentType
}
