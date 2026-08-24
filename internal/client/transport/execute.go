package transport

import (
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultRawJSONMaxBytes = 1 << 20

const (
	successBodyDrainMaxLen = httpErrorBodyParseMaxLen + httpErrorBodyDrainMaxLen
	decodedBodyDrainMaxLen = httpErrorBodyDrainMaxLen
)

var ErrResponseTooLarge = errors.New("iris: response body exceeds maximum allowed size")

// 상한까지만 읽으므로 그보다 큰 본문은 EOF에 닿지 못하고, 뒤따르는 Close가 keep-alive
// 재사용 대신 연결을 끊는다. 응답 크기에 비례한 무제한 읽기를 막기 위한 의도적 교환이다.
func drainBounded(body io.Reader, limit int64) {
	//nolint:errcheck,gosec // keep-alive 재사용을 위한 best-effort drain.
	io.Copy(io.Discard, io.LimitReader(body, limit))
}

// errURL은 TransportError.URL 표기로, 기존 에러 표면을 유지하기 위해 doSigned는 전체(비밀
// 제거) URL을, 재시도 POST 경로는 경로만 넘긴다. 성공 시 resp.Body는 호출자가 닫는다.
func (c *APIClient) do(req *http.Request, op, path, errURL string) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: op, URL: errURL, Err: err}
	}

	if !isSuccessfulHTTPStatus(resp.StatusCode) {
		defer resp.Body.Close()

		return nil, fmt.Errorf("%s %s: %w", op, path, readErrorResponse(path, resp))
	}

	return resp, nil
}

// doSigned는 본문 없는 서명 요청의 공통 경로(전송, transport 에러 매핑, ≥400 매핑)를 수행한다.
// 성공 시 호출자가 resp.Body를 소비하고 닫을 책임을 진다.
func (c *APIClient) doSigned(ctx context.Context, method, path string, role SecretRole) (*http.Response, error) {
	op := strings.ToLower(method)

	if c.initErr != nil {
		return nil, &TransportError{Op: opInit, URL: path, Err: c.initErr}
	}

	req, err := c.newSignedRequest(ctx, method, path, nil, role)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", op, path, err)
	}

	return c.do(req, op, path, redactedURLForError(req.URL.String())) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
}

func (c *APIClient) doGet[T any](ctx context.Context, path string, role SecretRole) (*T, error) {
	resp, err := c.doSigned(ctx, http.MethodGet, path, role)
	if err != nil {
		return nil, err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
	}

	defer resp.Body.Close()

	return decodeJSONBody[T](resp.Body, path)
}

func (c *APIClient) doSignedJSON[T any](req *http.Request, path string) (*T, error) {
	resp, err := c.do(req, "post", path, path)
	if err != nil {
		return nil, err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
	}

	defer resp.Body.Close()

	return decodeJSONBody[T](resp.Body, path)
}

func (c *APIClient) doSignedDiscard(req *http.Request, path string) error {
	resp, err := c.do(req, "post", path, path)
	if err != nil {
		return err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
	}

	defer resp.Body.Close()

	drainBounded(resp.Body, successBodyDrainMaxLen)

	return nil
}

func decodeJSONBody[T any](body io.Reader, path string) (*T, error) {
	var result T

	if err := jsonv2.UnmarshalRead(body, &result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	drainBounded(body, decodedBodyDrainMaxLen)

	return &result, nil
}

func (c *APIClient) rawJSON(ctx context.Context, method, path string, role SecretRole) (jsontext.Value, error) {
	return c.rawJSONLimited(ctx, method, path, role, DefaultRawJSONMaxBytes) //nolint:wrapcheck // 하위 호출의 오류가 작업 맥락을 이미 담고 있어 그대로 전달한다.
}

func (c *APIClient) rawJSONLimited(ctx context.Context, method, path string, role SecretRole, limit int64) (jsontext.Value, error) {
	resp, err := c.doSigned(ctx, method, path, role)
	if err != nil {
		return nil, err //nolint:wrapcheck // do·doSigned가 op와 path 맥락으로 이미 래핑한다.
	}

	defer resp.Body.Close()

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
