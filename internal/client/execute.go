package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/park285/iris-client-go/internal/jsonx"
)

const DefaultRawJSONMaxBytes = 1 << 20

var ErrResponseTooLarge = errors.New("iris: response body exceeds maximum allowed size")

// doSigned는 본문 없는 서명 요청의 공통 경로(전송, transport 에러 매핑, ≥400 매핑)를 수행한다.
// 성공 시 호출자가 resp.Body를 소비하고 닫을 책임을 진다.
func (c *H2CClient) doSigned(ctx context.Context, method, path string, role SecretRole) (*http.Response, error) {
	op := strings.ToLower(method)

	if c.initErr != nil {
		return nil, &TransportError{Op: opInit, URL: path, Err: c.initErr}
	}

	req, err := c.newSignedRequest(ctx, method, path, nil, role)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", op, path, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: op, URL: redactedURLForError(req.URL.String()), Err: err}
	}

	if resp.StatusCode >= 400 {
		defer func() {
			//nolint:errcheck,gosec
			resp.Body.Close()
		}()
		return nil, fmt.Errorf("%s %s: %w", op, path, readErrorResponse(path, resp))
	}

	return resp, nil
}

func (c *H2CClient) rawJSON(ctx context.Context, method, path string, role SecretRole) (jsonx.RawMessage, error) {
	return c.rawJSONLimited(ctx, method, path, role, DefaultRawJSONMaxBytes)
}

func (c *H2CClient) rawJSONLimited(ctx context.Context, method, path string, role SecretRole, limit int64) (jsonx.RawMessage, error) {
	resp, err := c.doSigned(ctx, method, path, role)
	if err != nil {
		return nil, err
	}
	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()

	if limit <= 0 {
		limit = DefaultRawJSONMaxBytes
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s %s: read response body: %w", strings.ToLower(method), path, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%s %s: %w (limit %d bytes)", strings.ToLower(method), path, ErrResponseTooLarge, limit)
	}
	return body, nil
}
