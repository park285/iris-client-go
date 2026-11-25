package transport

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/park285/iris-client-go/v6/internal/client/signing"
	"github.com/park285/iris-client-go/v6/internal/irishmac"
)

const (
	authEpochPath             = "/auth/epoch"
	authEpochMismatchCode     = "AUTH_EPOCH_MISMATCH"
	authEpochResponseMaxBytes = 1024
)

type authEpochCache struct {
	mu      sync.Mutex
	value   string
	loading chan struct{}
}

func (c *APIClient) requestContext(ctx context.Context) (context.Context, context.CancelCauseFunc) {
	ctx, cancel := context.WithCancelCause(ctx)

	if c.client.Timeout <= 0 {
		return ctx, cancel
	}

	bounded, stop := context.WithTimeout(ctx, c.client.Timeout)

	return bounded, func(cause error) {
		cancel(cause)
		stop()
	}
}

func (c *APIClient) authEpoch(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", authEpochTransportError("discover authentication epoch", err)
		}

		c.epoch.mu.Lock()

		if value := c.epoch.value; value != "" {
			c.epoch.mu.Unlock()

			return value, nil
		}

		if loading := c.epoch.loading; loading != nil {
			c.epoch.mu.Unlock()

			select {
			case <-ctx.Done():
				return "", authEpochTransportError("wait for authentication epoch", ctx.Err())
			case <-loading:
				continue
			}
		}

		loading := make(chan struct{})

		c.epoch.loading = loading
		c.epoch.mu.Unlock()

		value, err := c.fetchAuthEpoch(ctx)

		c.epoch.mu.Lock()

		if err == nil {
			c.epoch.value = value
		}

		c.epoch.loading = nil
		close(loading)
		c.epoch.mu.Unlock()

		if err != nil {
			if errors.Is(err, ErrNotSent) {
				return "", err
			}

			return "", fmt.Errorf("%w: discover authentication epoch: %w", ErrNotSent, err)
		}

		return value, nil
	}
}

func (c *APIClient) fetchAuthEpoch(ctx context.Context) (string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, authEpochPath, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", authEpochTransportError("discover authentication epoch", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &HTTPError{
			StatusCode: resp.StatusCode,
			URL:        authEpochPath,
			RetryAfter: parseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now()),
		}
	}

	payload, err := io.ReadAll(io.LimitReader(resp.Body, authEpochResponseMaxBytes+1))
	if err != nil {
		return "", authEpochTransportError("read authentication epoch", err)
	}

	if len(payload) > authEpochResponseMaxBytes {
		return "", ErrResponseTooLarge
	}

	var response struct {
		Epoch string `json:"epoch"`
	}

	if err := jsonv2.Unmarshal(payload, &response); err != nil {
		return "", errors.New("invalid authentication epoch JSON")
	}

	if !validAuthEpoch(response.Epoch) {
		return "", errors.New("invalid authentication epoch format")
	}

	return response.Epoch, nil
}

func authEpochTransportError(op string, err error) error {
	if errors.Is(err, errHTTP1ProbeOnly) {
		return err
	}

	if !errors.Is(err, ErrNotSent) {
		err = fmt.Errorf("%w: %w", ErrNotSent, err)
	}

	return &TransportError{
		Op:  op,
		URL: authEpochPath,
		Err: err,
	}
}

func validAuthEpoch(value string) bool {
	if len(value) != 32 {
		return false
	}

	for i := range len(value) {
		if ch := value[i]; (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}

	return true
}

func (c *APIClient) signRequest(req *http.Request, method, path, bodySHA256 string) error {
	if c.signer == nil {
		return nil
	}

	epoch, err := c.authEpoch(req.Context())
	if err != nil {
		return err
	}

	if err := signing.SetIrisHMACHeaders(req, c.signer, method, path, bodySHA256, epoch); err != nil {
		return fmt.Errorf("sign iris request: %w", err)
	}

	return nil
}

func (c *APIClient) refreshAuthEpoch(req *http.Request, err error, refreshed *bool) bool {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != http.StatusUnauthorized || HTTPErrorCode(err) != authEpochMismatchCode {
		return false
	}

	used, _, bound := strings.Cut(req.Header.Get(irishmac.HeaderIrisNonce), ".")
	if !bound || !validAuthEpoch(used) || c.signer == nil {
		return false
	}

	c.epoch.mu.Lock()

	if c.epoch.value == used {
		c.epoch.value = ""
	}

	c.epoch.mu.Unlock()

	if *refreshed {
		return false
	}

	*refreshed = true

	return true
}
