package iris

import (
	"errors"

	client "github.com/park285/iris-client-go/v2/internal/client/transport"
)

type (
	HTTPError      = client.HTTPError
	TransportError = client.TransportError
)

const (
	HTTPErrorCodeClientRequestIDPayloadMismatch = "CLIENT_REQUEST_ID_PAYLOAD_MISMATCH"
	HTTPErrorCodeClientRequestIDFailed          = "CLIENT_REQUEST_ID_FAILED"
	HTTPErrorCodeClientRequestIDOutcomeUnknown  = "CLIENT_REQUEST_ID_OUTCOME_UNKNOWN"
	HTTPErrorCodeClientRequestIDAlreadyExists   = "CLIENT_REQUEST_ID_ALREADY_EXISTS"
)

var (
	ErrRetryable   = client.ErrRetryable
	ErrPermanent   = client.ErrPermanent
	ErrAuthFailed  = client.ErrAuthFailed
	ErrRateLimited = client.ErrRateLimited
	ErrTransport   = client.ErrTransport
	// ErrResponseTooLarge는 응답의 byte 상한 초과를 나타내며 SDK 자동 재시도를 하지 않는다.
	ErrResponseTooLarge = client.ErrResponseTooLarge

	ErrInboundSecretRequired   = client.ErrInboundSecretRequired
	ErrCertReloadTokenRequired = client.ErrCertReloadTokenRequired
)

func IsH3EgressDenied(err error) bool {
	return errors.Is(err, client.ErrH3EgressDenied)
}

// HTTPErrorCode는 Iris HTTP error chain의 검증된 machine-readable code를 반환한다.
// Code가 없거나 응답이 공개 token 계약을 벗어나면 빈 문자열을 반환한다.
func HTTPErrorCode(err error) string {
	return client.HTTPErrorCode(err)
}
