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

	"github.com/park285/iris-client-go/v5/internal/client/signing"
)

const (
	authEpochPath             = "/auth/epoch"
	authEpochMismatchCode     = "AUTH_EPOCH_MISMATCH"
	authEpochResponseMaxBytes = 1024
)

// 클라이언트별 캐시는 대상 URL과 TLS 신뢰 설정을 공유한다. 네트워크 I/O는 잠금 밖에서
// 호출자 goroutine으로 수행하며, 동시 호출의 대기는 각자의 context로 중단할 수 있다.
type authEpochCache struct {
	mu      sync.Mutex
	value   string
	loading chan struct{}
}

// 원래 HTTP 시도의 timeout을 discovery·재서명과 공유한다. 외부 reply retry의 시도별
// 예산은 그대로 두되 epoch 전환만으로 timeout을 새로 시작하지 않는다.
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
		// 공개 discovery의 본문 code는 보호 요청의 clientRequestId 판정으로 해석하지 않는다.
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
	// 명시적 probe 거절과 달리 guard의 미전송 태그에는 일시적인 DNS 실패·취소도 포함된다.
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
		if ch := value[i]; !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
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

// 검증된 401 code만 논리 호출당 한 번 재서명한다. 늦게 도착한 이전 epoch의 실패는
// 다른 호출이 이미 갱신한 캐시를 지우지 않는다. 일반 401과 전송 오류는 이 경로로 재시도하지 않는다.
func (c *APIClient) refreshAuthEpoch(req *http.Request, err error, refreshed *bool) bool {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.StatusCode != http.StatusUnauthorized || HTTPErrorCode(err) != authEpochMismatchCode {
		return false
	}

	used, _, bound := strings.Cut(req.Header.Get(HeaderIrisNonce), ".")
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
