package transport

import (
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
)

func (c *H2CClient) postStrictJSON(ctx context.Context, path string, body, out any, role SecretRole) error {
	payload, err := jsonv2.Marshal(body)
	if err != nil {
		return fmt.Errorf("post %s: encode request body: %w", path, err)
	}
	if c.initErr != nil {
		return &TransportError{Op: opInit, URL: path, Err: c.initErr}
	}

	req, err := c.newSignedRequest(ctx, http.MethodPost, path, payload, role)
	if err != nil {
		return fmt.Errorf("post %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return &TransportError{Op: "post", URL: path, Err: err}
	}
	defer func() {
		//nolint:errcheck,gosec // deferred path best-effort body close.
		resp.Body.Close()
	}()

	if !isSuccessfulHTTPStatus(resp.StatusCode) {
		return fmt.Errorf("post %s: %w", path, readErrorResponse(path, resp))
	}
	if out == nil {
		drainBounded(resp.Body, successBodyDrainMaxLen)
		return nil
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, DefaultRawJSONMaxBytes+1))
	if err != nil {
		return fmt.Errorf("decode %s response: read body: %w", path, err)
	}
	if len(bodyBytes) > DefaultRawJSONMaxBytes {
		return fmt.Errorf("decode %s response: %w (limit %d bytes)", path, ErrResponseTooLarge, DefaultRawJSONMaxBytes)
	}
	if err := jsonv2.Unmarshal(bodyBytes, out, jsonv2.RejectUnknownMembers(true)); err != nil {
		return fmt.Errorf("decode %s response: %w", path, err)
	}
	return nil
}
