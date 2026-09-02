package transport

import (
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
)

func (c *APIClient) postStrictJSON[T any](ctx context.Context, path string, body any, role SecretRole) (*T, error) {
	payload, err := jsonv2.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	if c.initErr != nil {
		return nil, &TransportError{Op: opInit, URL: path, Err: c.initErr}
	}

	req, err := c.newSignedRequest(ctx, http.MethodPost, path, payload, role)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}

	req.Header.Set("Content-Type", contentTypeJSON)

	resp, err := c.do(req, "post", path, path)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, DefaultRawJSONMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decode %s response: read body: %w", path, err)
	}

	if len(bodyBytes) > DefaultRawJSONMaxBytes {
		return nil, fmt.Errorf("decode %s response: %w (limit %d bytes)", path, ErrResponseTooLarge, DefaultRawJSONMaxBytes)
	}

	var result T

	if err := jsonv2.Unmarshal(bodyBytes, &result, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	return &result, nil
}
