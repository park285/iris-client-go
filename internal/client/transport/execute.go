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

	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

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

func drainBounded(body io.Reader, limit int64) {
	//nolint:errcheck,gosec // keep-alive 재사용을 위한 best-effort drain.
	io.Copy(io.Discard, io.LimitReader(body, limit))
}

func (c *APIClient) do(req *http.Request, op, path, errURL string, idempotent bool, epochRefreshed *bool) (*http.Response, error) {
	for {
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, &TransportError{Op: op, URL: errURL, Err: err, Idempotent: idempotent}
		}

		if isSuccessfulHTTPStatus(resp.StatusCode) {
			return resp, nil
		}

		responseErr := readErrorResponse(path, resp)
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, &stopReplyRetryError{cause: fmt.Errorf("%s %s: %w; close response body: %w", op, path, responseErr, closeErr)}
		}

		if !c.refreshAuthEpoch(req, responseErr, epochRefreshed) {
			if errors.Is(responseErr, ErrNotSent) {
				return nil, &stopReplyRetryError{cause: fmt.Errorf("%s %s: %w", op, path, responseErr)}
			}

			return nil, fmt.Errorf("%s %s: %w", op, path, responseErr)
		}

		replay := req.Clone(req.Context())
		if err = c.signRequest(replay, req.Method, path, req.Header.Get(irishmac.HeaderIrisBodySHA256)); err != nil {
			return nil, &stopReplyRetryError{cause: err}
		}

		if req.Body != nil && req.Body != http.NoBody {
			if req.GetBody == nil {
				return nil, &stopReplyRetryError{cause: fmt.Errorf("%w: authentication epoch refresh requires a replayable body", ErrNotSent)}
			}

			replay.Body, err = req.GetBody()
			if err != nil {
				return nil, &stopReplyRetryError{cause: fmt.Errorf("%w: replay request body: %w", ErrNotSent, err)}
			}
		}

		req = replay
	}
}

func (c *APIClient) doSigned(ctx context.Context, method, path string) (*http.Response, error) {
	op := strings.ToLower(method)

	req, err := c.newSignedRequest(ctx, method, path, nil)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", op, path, err)
	}

	var epochRefreshed bool

	return c.do(req, op, path, redactedURLForError(req.URL.String()), true, &epochRefreshed)
}

func (c *APIClient) doGet[T any](ctx context.Context, path string) (*T, error) {
	reqCtx, cancel := c.requestContext(ctx)
	defer cancel(nil)

	resp, err := c.doSigned(reqCtx, http.MethodGet, path) //nolint:bodyclose // F03: decodeJSONBody가 Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](reqCtx, cancel, resp.Body, http.MethodGet, path, true)
}

func (c *APIClient) doSignedJSON[T any](req *http.Request, cancel context.CancelCauseFunc, path string, idempotent bool, epochRefreshed *bool) (*T, error) {
	resp, err := c.do(req, "post", path, path, idempotent, epochRefreshed) //nolint:bodyclose // F03: decodeJSONBody가 Close를 단독 소유하며 public Close-once 회귀로 검증한다.
	if err != nil {
		return nil, err
	}

	return c.decodeJSONBody[T](req.Context(), cancel, resp.Body, http.MethodPost, path, idempotent)
}

func (c *APIClient) armSuccessBodyDrainDeadline(cancel context.CancelCauseFunc) (stop func() bool) {
	timeout := successBodyDrainTimeout

	if c.opts.Timeout > 0 {
		timeout = min(timeout, c.opts.Timeout)
	}

	return time.AfterFunc(timeout, func() { cancel(ErrResponseDrainTimeout) }).Stop
}

func (c *APIClient) decodeJSONBody[T any](
	ctx context.Context,
	cancel context.CancelCauseFunc,
	body io.ReadCloser,
	method, path string,
	idempotent bool,
) (response *T, err error) {
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
