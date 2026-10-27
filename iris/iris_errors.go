package iris

import client "github.com/park285/iris-client-go/v4/internal/client/transport"

type (
	HTTPError      = client.HTTPError
	TransportError = client.TransportError
)

// 409 CLIENT_REQUEST_ID_* code의 정본은 internal/client/transport/errors.go다.
const (
	HTTPErrorCodeClientRequestIDPayloadMismatch = client.HTTPErrorCodeClientRequestIDPayloadMismatch
	HTTPErrorCodeClientRequestIDFailed          = client.HTTPErrorCodeClientRequestIDFailed
	HTTPErrorCodeClientRequestIDOutcomeUnknown  = client.HTTPErrorCodeClientRequestIDOutcomeUnknown
	HTTPErrorCodeClientRequestIDAlreadyExists   = client.HTTPErrorCodeClientRequestIDAlreadyExists
)

var (
	ErrRetryable   = client.ErrRetryable
	ErrPermanent   = client.ErrPermanent
	ErrAuthFailed  = client.ErrAuthFailed
	ErrRateLimited = client.ErrRateLimited
	// ErrTransport는 요청이 Iris에 도달했는지 알 수 없는 결과 불명 실패다. ErrNotSent와 함께 성립하지 않는다.
	ErrTransport = client.ErrTransport
	// ErrNotSent는 요청이 나가기 전에 실패가 확정돼 부수효과가 없었음을 뜻한다(egress 거부, dial 전
	// resolve 실패·취소, http1 transport의 probe 외 요청).
	ErrNotSent = client.ErrNotSent
	// ErrResponseTooLarge는 응답의 byte 상한 초과를 나타내며 SDK 자동 재시도를 하지 않는다.
	ErrResponseTooLarge = client.ErrResponseTooLarge
)

// HTTPErrorCode는 Iris HTTP error chain의 검증된 machine-readable code를 반환한다.
// Code가 없거나 응답이 공개 token 계약을 벗어나면 빈 문자열을 반환한다.
// SDK가 HTTP 응답에서 보존한 code만 읽으며, 직접 만든 HTTPError.Body는 해석하지 않는다.
func HTTPErrorCode(err error) string {
	return client.HTTPErrorCode(err)
}
