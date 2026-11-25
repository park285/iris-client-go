package iris

import client "github.com/park285/iris-client-go/v6/internal/client/transport"

type (
	HTTPError      = client.HTTPError
	TransportError = client.TransportError
)

const (
	HTTPErrorCodeClientRequestIDPayloadMismatch = client.HTTPErrorCodeClientRequestIDPayloadMismatch
	HTTPErrorCodeClientRequestIDFailed          = client.HTTPErrorCodeClientRequestIDFailed
	HTTPErrorCodeClientRequestIDOutcomeUnknown  = client.HTTPErrorCodeClientRequestIDOutcomeUnknown
	HTTPErrorCodeClientRequestIDAlreadyExists   = client.HTTPErrorCodeClientRequestIDAlreadyExists
)

var (
	ErrRetryable        = client.ErrRetryable
	ErrPermanent        = client.ErrPermanent
	ErrAuthFailed       = client.ErrAuthFailed
	ErrRateLimited      = client.ErrRateLimited
	ErrTransport        = client.ErrTransport
	ErrNotSent          = client.ErrNotSent
	ErrResponseTooLarge = client.ErrResponseTooLarge
)

func HTTPErrorCode(err error) string {
	return client.HTTPErrorCode(err)
}
