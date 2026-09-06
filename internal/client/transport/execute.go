package transport

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const DefaultRawJSONMaxBytes = 1 << 20

const (
	typedJSONResponseMaxBytes = 16 << 20
	successBodyDrainMaxLen    = httpErrorBodyParseMaxLen + httpErrorBodyDrainMaxLen
	decodedBodyDrainMaxLen    = httpErrorBodyDrainMaxLen
	decodedBodyDrainTimeout   = 100 * time.Millisecond
)

var (
	ErrResponseTooLarge        = errors.New("iris: response body exceeds maximum allowed size")
	ErrResponseDrainTimeout    = errors.New("iris: response body drain timed out")
	errResponseCloseTimeout    = errors.New("iris: response body close timed out")
	errUnexpectedResponseBytes = errors.New("iris: unexpected bytes after JSON response")
)

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

	return c.do(req, op, path, redactedURLForError(req.URL.String()))
}

func (c *APIClient) doGet[T any](ctx context.Context, path string, role SecretRole) (*T, error) {
	resp, err := c.doSigned(ctx, http.MethodGet, path, role) //nolint:bodyclose // F03: decodeJSONBody가 bounded Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](ctx, resp.Body, http.MethodGet, path)
}

func (c *APIClient) doSignedJSON[T any](req *http.Request, path string) (*T, error) {
	resp, err := c.do(req, "post", path, path) //nolint:bodyclose // F03: decodeJSONBody가 bounded Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](req.Context(), resp.Body, http.MethodPost, path)
}

func (c *APIClient) doSignedDiscard(req *http.Request, path string) error {
	resp, err := c.do(req, "post", path, path)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	drainBounded(resp.Body, successBodyDrainMaxLen)

	return nil
}

func (c *APIClient) decodeJSONBody[T any](ctx context.Context, body io.ReadCloser, method, path string) (response *T, err error) {
	// typed 본문의 종료는 이 owner만 맡는다. 바깥 Close는 종료 예산을 무력화한다.
	defer func() {
		if closeErr := closeDecodedBodyBounded(ctx, c.logger, body); closeErr != nil {
			response = nil
			err = &TransportError{Op: strings.ToLower(method), URL: path, Err: errors.Join(err, closeErr)}
		}
	}()

	var result T

	// DEC-20260906-sdk-typed-json-response-budget: 전송 계층이 압축을 해제한 본문을 제한한다.
	limited := &io.LimitedReader{R: body, N: typedJSONResponseMaxBytes + 1}
	decoder := jsontext.NewDecoder(limited)
	decodeErr := jsonv2.UnmarshalDecode(decoder, &result)

	if limited.N == 0 {
		decodeErr = errors.Join(decodeErr, fmt.Errorf("%w (limit %d bytes)", ErrResponseTooLarge, typedJSONResponseMaxBytes))
	}

	if decodeErr != nil {
		decodeErr = fmt.Errorf("decode %s response: %w", path, decodeErr)

		if method == http.MethodPost {
			return nil, &TransportError{
				Op:  strings.ToLower(method),
				URL: path,
				Err: decodeErr,
			}
		}

		return nil, decodeErr
	}

	buffered := bytes.Clone(decoder.UnreadBuffer())
	drainTimeout := decodedBodyDrainTimeout

	if c.opts.Timeout > 0 {
		drainTimeout = min(drainTimeout, c.opts.Timeout)
	}

	if err := drainDecodedBodyBounded(ctx, c.logger, limited, buffered, drainTimeout); err != nil {
		return nil, &TransportError{
			Op:  strings.ToLower(method),
			URL: path,
			Err: fmt.Errorf("decode response trailer: %w", err),
		}
	}

	return &result, nil
}

type decodedBodyDrainResult struct {
	bytes int64
	err   error
}

// drainDecodedBodyBounded attempts keep-alive reuse only when EOF is observed
// within both the byte and time budgets. The decoder owner closes the body
// after every outcome, including a read still blocked at the deadline.
func drainDecodedBodyBounded(
	ctx context.Context,
	logger *slog.Logger,
	body *io.LimitedReader,
	buffered []byte,
	timeout time.Duration,
) error {
	drained := make(chan decodedBodyDrainResult, 1)

	safeGo(logger, "iris_client_response_body_drain_panic", func() {
		trailer := io.MultiReader(bytes.NewReader(buffered), body)
		read, err := validateJSONTrailer(io.LimitReader(trailer, decodedBodyDrainMaxLen+1))
		// timeout 뒤에도 reader를 소유하는 이 goroutine 안에서만 잔여 예산을 읽는다.
		if body.N == 0 {
			err = errors.Join(err, fmt.Errorf("%w (limit %d bytes)", ErrResponseTooLarge, typedJSONResponseMaxBytes))
		}

		drained <- decodedBodyDrainResult{bytes: read, err: err}
	})

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case result := <-drained:
		if result.bytes > decodedBodyDrainMaxLen {
			return fmt.Errorf("%w (limit %d bytes)", ErrResponseTooLarge, decodedBodyDrainMaxLen)
		}

		if result.err != nil {
			return result.err
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("drain response trailer: %w", ctx.Err())
	case <-timer.C:
		return ErrResponseDrainTimeout
	}
}

func validateJSONTrailer(trailer io.Reader) (int64, error) {
	var total int64

	buffer := make([]byte, 4096)

	for {
		read, err := trailer.Read(buffer)

		total += int64(read)

		for _, char := range buffer[:read] {
			switch char {
			case ' ', '\t', '\r', '\n':
			default:
				return total, errUnexpectedResponseBytes
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				return total, nil
			}

			return total, fmt.Errorf("read response trailer: %w", err)
		}
	}
}

func closeDecodedBodyBounded(ctx context.Context, logger *slog.Logger, body io.Closer) error {
	closed := make(chan error, 1)

	safeGo(logger, "iris_client_response_body_close_panic", func() {
		closed <- body.Close()
	})

	timer := time.NewTimer(decodedBodyDrainTimeout)
	defer timer.Stop()

	select {
	case err := <-closed:
		if err != nil {
			return fmt.Errorf("close response body: %w", err)
		}

		return nil
	case <-ctx.Done():
		return fmt.Errorf("close response body: %w", ctx.Err())
	case <-timer.C:
		return errResponseCloseTimeout
	}
}

func (c *APIClient) rawJSON(ctx context.Context, method, path string, role SecretRole) (jsontext.Value, error) {
	return c.rawJSONLimited(ctx, method, path, role, DefaultRawJSONMaxBytes)
}

func (c *APIClient) rawJSONLimited(ctx context.Context, method, path string, role SecretRole, limit int64) (jsontext.Value, error) {
	resp, err := c.doSigned(ctx, method, path, role)
	if err != nil {
		return nil, err
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
