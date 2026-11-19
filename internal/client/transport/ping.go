package transport

import (
	"context"
	"fmt"
	"net/http"
)

const pingDrainMaxBytes = 4 << 10

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
