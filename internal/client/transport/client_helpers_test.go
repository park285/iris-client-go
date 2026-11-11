package transport

import "context"

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
