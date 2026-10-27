package transport

import (
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
)

func (c *APIClient) postStrictJSON[T any](ctx context.Context, path string, body any) (*T, error) {
	payload, err := jsonv2.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("post %s: encode request body: %w", path, err)
	}

	req, err := c.newSignedRequest(ctx, http.MethodPost, path, payload)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}

	req.Header.Set("Content-Type", contentTypeJSON)

	// strict JSON POST는 media chunk 조회 전용이라 다시 보내도 부수효과가 없다.
	resp, err := c.do(req, "post", path, path, true)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, strictJSONResponseMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("decode %s response: read body: %w", path, err)
	}

	if len(bodyBytes) > strictJSONResponseMaxBytes {
		return nil, fmt.Errorf("decode %s response: %w (limit %d bytes)", path, ErrResponseTooLarge, strictJSONResponseMaxBytes)
	}

	var result T

	if err := jsonv2.Unmarshal(bodyBytes, &result, jsonv2.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	return &result, nil
}
