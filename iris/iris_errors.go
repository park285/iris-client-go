package iris

import (
	"errors"

	"github.com/park285/iris-client-go/internal/client"
)

type HTTPError = client.HTTPError
type TransportError = client.TransportError
type PingError = client.PingError

var (
	ErrRetryable   = client.ErrRetryable
	ErrPermanent   = client.ErrPermanent
	ErrAuthFailed  = client.ErrAuthFailed
	ErrRateLimited = client.ErrRateLimited
	ErrTransport   = client.ErrTransport

	ErrCertReloadTokenRequired = client.ErrCertReloadTokenRequired
	ErrInboundSecretRequired   = client.ErrInboundSecretRequired
)

func IsH3EgressDenied(err error) bool {
	return errors.Is(err, client.ErrH3EgressDenied)
}
