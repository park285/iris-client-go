package transport

import (
	"context"
	"fmt"
	"net/http"
)

const pingDrainMaxBytes = 4 << 10

// Ping은 GET /ready를 한 번만 확인한다. Iris /ready는 인증 없는 route라 서명하지 않는다. 다른
// endpoint로 넘어가거나 다시 시도하는 경로는 없으며, 재시도와 로그는 호출자가 소유한다. 200 외의
// 응답은 *HTTPError(4xx는 ErrPermanent, 5xx는 ErrRetryable)이고 전송 실패는 *TransportError다.
// 시도 하나의 상한은 고정값 defaultPingProbeTimeout(5초)이다.
func (c *APIClient) Ping(ctx context.Context) error {
	const path = PathReady

	probeCtx, cancel := context.WithTimeout(ctx, defaultPingProbeTimeout)
	defer cancel()

	req, err := c.newRequest(probeCtx, http.MethodGet, path, nil)
	if err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return &TransportError{Op: "ping", URL: redactedURLForError(req.URL.String()), Err: err, Idempotent: true}
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: %w", readErrorResponse(path, resp))
	}

	drainBounded(resp.Body, pingDrainMaxBytes)

	return nil
}
