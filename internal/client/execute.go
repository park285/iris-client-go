package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/park285/iris-client-go/internal/jsonx"
)

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
		return nil, &TransportError{Op: op, URL: req.URL.String(), Err: err}
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
	resp, err := c.doSigned(ctx, method, path, role)
	if err != nil {
		return nil, err
	}
	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()
	return io.ReadAll(resp.Body)
}
