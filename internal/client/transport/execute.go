package transport

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// strictJSONResponseMaxBytes는 strict media 응답 본문 상한이다.
const strictJSONResponseMaxBytes = 1 << 20

const (
	typedJSONResponseMaxBytes = 16 << 20
	decodedBodyDrainMaxLen    = httpErrorBodyDrainMaxLen
	successBodyDrainTimeout   = 100 * time.Millisecond
)

var (
	ErrResponseTooLarge        = errors.New("iris: response body exceeds maximum allowed size")
	ErrResponseDrainTimeout    = errors.New("iris: response body drain timed out")
	errUnexpectedResponseBytes = errors.New("iris: unexpected bytes after JSON response")
)

// 상한까지만 읽으므로 그보다 큰 본문은 EOF에 닿지 못하고, 뒤따르는 Close가 keep-alive
// 재사용 대신 연결을 끊는다. 응답 크기에 비례한 무제한 읽기를 막기 위한 의도적 교환이다.
func drainBounded(body io.Reader, limit int64) {
	//nolint:errcheck,gosec // keep-alive 재사용을 위한 best-effort drain.
	io.Copy(io.Discard, io.LimitReader(body, limit))
}

// errURL은 TransportError.URL 표기로, 기존 에러 표면을 유지하기 위해 doSigned는 전체(비밀
// 제거) URL을, 재시도 POST 경로는 경로만 넘긴다. Idempotent는 TransportError.Idempotent가 된다.
// 성공 시 resp.Body는 호출자가 닫는다.
func (c *APIClient) do(req *http.Request, op, path, errURL string, idempotent bool) (*http.Response, error) {
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, &TransportError{Op: op, URL: errURL, Err: err, Idempotent: idempotent}
	}

	if !isSuccessfulHTTPStatus(resp.StatusCode) {
		defer resp.Body.Close()

		return nil, fmt.Errorf("%s %s: %w", op, path, readErrorResponse(path, resp))
	}

	return resp, nil
}

// doSigned는 본문 없는 서명 요청의 공통 경로(전송, transport 에러 매핑, ≥400 매핑)를 수행한다.
// 성공 시 호출자가 resp.Body를 소비하고 닫을 책임을 진다.
func (c *APIClient) doSigned(ctx context.Context, method, path string) (*http.Response, error) {
	op := strings.ToLower(method)

	req, err := c.newSignedRequest(ctx, method, path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", op, path, err)
	}

	// 본문 없는 서명 요청은 조회 GET뿐이라 다시 보내도 부수효과가 없다.
	return c.do(req, op, path, redactedURLForError(req.URL.String()), true)
}

// doGet은 요청 하나에 취소 원인을 실을 수 있는 context를 파생한다. 그 취소로 decodeJSONBody가
// trailer drain 시간 상한을 건다.
func (c *APIClient) doGet[T any](ctx context.Context, path string) (*T, error) {
	reqCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	resp, err := c.doSigned(reqCtx, http.MethodGet, path) //nolint:bodyclose // F03: decodeJSONBody가 Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](reqCtx, cancel, resp.Body, http.MethodGet, path, true)
}

// cancel은 req.Context()를 취소하는 시도별 CancelCauseFunc다(retryPost가 소유한다).
func (c *APIClient) doSignedJSON[T any](req *http.Request, cancel context.CancelCauseFunc, path string, idempotent bool) (*T, error) {
	resp, err := c.do(req, "post", path, path, idempotent) //nolint:bodyclose // F03: decodeJSONBody가 Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](req.Context(), cancel, resp.Body, http.MethodPost, path, idempotent)
}

// armSuccessBodyDrainDeadline은 결과가 정해진 뒤의 성공 본문 drain 구간에만 시간 상한을 건다. 상한에
// 닿으면 cancel(ErrResponseDrainTimeout)으로 요청 context를 취소해 막힌 본문 Read를 푼다. 그래서
// WithHTTPClient로 넣는 transport도 요청 context 취소로 본문 Read를 풀어야 한다. 반환한 stop은 drain이
// 끝나는 즉시 호출한다.
func (c *APIClient) armSuccessBodyDrainDeadline(cancel context.CancelCauseFunc) (stop func() bool) {
	timeout := successBodyDrainTimeout

	if c.opts.Timeout > 0 {
		timeout = min(timeout, c.opts.Timeout)
	}

	return time.AfterFunc(timeout, func() { cancel(ErrResponseDrainTimeout) }).Stop
}

// decodeJSONBody는 typed 본문을 디코드하고 trailer를 같은 goroutine에서 바이트·시간 한도 안에 비운다.
// 인자 ctx는 요청 context이고 cancel은 그 context를 취소한다. 시간 상한은
// armSuccessBodyDrainDeadline이 trailer drain 구간에만 건다.
func (c *APIClient) decodeJSONBody[T any](
	ctx context.Context,
	cancel context.CancelCauseFunc,
	body io.ReadCloser,
	method, path string,
	idempotent bool,
) (response *T, err error) {
	// typed 본문의 종료는 이 owner만 맡는다. net/http와 HTTP/3 본문의 Close는 I/O를 기다리지
	// 않으므로 동기 호출한다.
	defer func() {
		if closeErr := body.Close(); closeErr != nil {
			response = nil
			err = &TransportError{
				Op:         strings.ToLower(method),
				URL:        path,
				Err:        errors.Join(err, fmt.Errorf("close response body: %w", closeErr)),
				Idempotent: idempotent,
			}
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
				Op:         strings.ToLower(method),
				URL:        path,
				Err:        decodeErr,
				Idempotent: idempotent,
			}
		}

		return nil, decodeErr
	}

	stopDrainDeadline := c.armSuccessBodyDrainDeadline(cancel)
	trailer := io.MultiReader(bytes.NewReader(decoder.UnreadBuffer()), limited)
	read, drainErr := validateJSONTrailer(io.LimitReader(trailer, decodedBodyDrainMaxLen+1))

	stopDrainDeadline()

	if err := classifyTrailerDrain(ctx, limited, read, drainErr); err != nil {
		return nil, &TransportError{
			Op:         strings.ToLower(method),
			URL:        path,
			Err:        fmt.Errorf("decode response trailer: %w", err),
			Idempotent: idempotent,
		}
	}

	return &result, nil
}

// classifyTrailerDrain은 바이트·시간 한도 안에 EOF를 읽었을 때만 nil을 반환한다. 시간 초과는 Read
// 오류 값이 아니라 요청 context의 취소 원인으로 판정한다. HTTP/3는 취소된 본문 Read에서
// *quic.StreamError를 돌려주므로 오류 값으로는 시간 초과와 부모 취소를 구분할 수 없다.
func classifyTrailerDrain(ctx context.Context, body *io.LimitedReader, read int64, err error) error {
	if read > decodedBodyDrainMaxLen {
		return fmt.Errorf("%w (limit %d bytes)", ErrResponseTooLarge, decodedBodyDrainMaxLen)
	}

	if body.N == 0 {
		err = errors.Join(err, fmt.Errorf("%w (limit %d bytes)", ErrResponseTooLarge, typedJSONResponseMaxBytes))
	}

	if err == nil || errors.Is(err, errUnexpectedResponseBytes) || errors.Is(err, ErrResponseTooLarge) {
		return err
	}

	if errors.Is(context.Cause(ctx), ErrResponseDrainTimeout) {
		return ErrResponseDrainTimeout
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("drain response trailer: %w", ctxErr)
	}

	return err
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
