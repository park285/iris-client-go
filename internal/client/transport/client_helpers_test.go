package transport

import "context"

// mustNewAPIClient는 테스트 전용 생성 helper다. 생성 오류는 테스트 설정 결함이라 panic으로 드러낸다.
// SDK 소유 http1 transport를 임의 test server의 보호 route에 붙여 재시도·서명 동작을 보려는
// 테스트를 위해 probe 전용 제한만 해제한다. 제한 자체는 실제 생성자를 쓰는 테스트가 검증한다.
func mustNewAPIClient(baseURL, botToken string, opts ...ClientOption) *APIClient {
	c, err := newAPIClient(baseURL, botToken, applyClientOptions(opts))
	if err != nil {
		panic(err)
	}

	c.http1ProbeOnly = false

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
