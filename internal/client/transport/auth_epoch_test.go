package transport

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v5/internal/client/signing"
	"github.com/park285/iris-client-go/v5/internal/testsupport"
)

const (
	testAuthEpoch     = "000102030405060708090a0b0c0d0e0f"
	testNextAuthEpoch = "101112131415161718191a1b1c1d1e1f"
	testEpochSecret   = "synthetic-epoch-test-token"
	testEpochMismatch = `{"message":"authentication epoch changed","code":"AUTH_EPOCH_MISMATCH"}`
)

func epochResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func epochDocument(epoch string) string {
	return `{"epoch":"` + epoch + `"}`
}

func newEpochTestClient(t *testing.T, rt http.RoundTripper, opts ...ClientOption) *APIClient {
	t.Helper()

	options := []ClientOption{
		WithBaseURL("https://iris.test"), WithBotToken(testEpochSecret),
		WithHTTPClient(&http.Client{Transport: rt}),
	}

	client, err := NewClient(append(options, opts...)...)
	if err != nil {
		t.Fatal(err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	return client
}

func TestAuthEpochBootstrapValidationAndCache(t *testing.T) {
	valid := epochDocument(testAuthEpoch)
	for _, tc := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{name: "valid", status: http.StatusOK, body: valid, valid: true},
		{name: "exact limit", status: http.StatusOK, body: valid + strings.Repeat(" ", authEpochResponseMaxBytes-len(valid)), valid: true},
		{name: "oversized", status: http.StatusOK, body: valid + strings.Repeat(" ", authEpochResponseMaxBytes-len(valid)+1)},
		{name: "unsupported", status: http.StatusNotFound, body: valid},
		{name: "wrong success status", status: http.StatusAccepted, body: valid},
		{name: "uppercase", status: http.StatusOK, body: epochDocument(strings.ToUpper(testAuthEpoch))},
		{name: "short", status: http.StatusOK, body: epochDocument(testAuthEpoch[:31])},
		{name: "nonhex", status: http.StatusOK, body: epochDocument(strings.Repeat("g", 32))},
		{name: "missing", status: http.StatusOK, body: `{}`},
		{name: "null", status: http.StatusOK, body: `null`},
		{name: "wrong type", status: http.StatusOK, body: `{"epoch":123}`},
		{name: "duplicate", status: http.StatusOK, body: `{"epoch":"` + testAuthEpoch + `","epoch":"` + testAuthEpoch + `"}`},
		{name: "second value", status: http.StatusOK, body: valid + `{}`},
		{name: "trailing garbage", status: http.StatusOK, body: valid + `x`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var discoveries, protected int

			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == authEpochPath {
					discoveries++

					if req.Method != http.MethodGet || req.URL.RawQuery != "" || req.Body != nil || hasIrisSigningHeaders(req.Header) {
						t.Error("epoch discovery must be an unsigned bodyless GET without query")
					}

					return epochResponse(tc.status, tc.body), nil
				}

				protected++

				return epochResponse(http.StatusOK, `{}`), nil
			}))

			_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello")

			if !tc.valid {
				if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) || protected != 0 || discoveries != 1 {
					t.Fatalf("bootstrap failure classification/count: error=%v protected=%d discoveries=%d", err, protected, discoveries)
				}

				return
			}

			if err != nil {
				t.Fatal(err)
			}

			if _, err := client.GetRooms(t.Context()); err != nil {
				t.Fatal(err)
			}

			if discoveries != 1 || protected != 2 {
				t.Fatalf("cache counts: discoveries=%d protected=%d", discoveries, protected)
			}
		})
	}
}

func TestAuthEpochRefreshReplaysEveryProtectedBody(t *testing.T) {
	for _, tc := range []struct {
		name string
		call func(context.Context, *APIClient) error
	}{
		{name: "GET", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.GetRooms(ctx)
			return err
		}},
		{name: "JSON without id", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendMessageAccepted(ctx, testRoom, "hello")
			return err
		}},
		{name: "JSON with id", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendMessageAccepted(ctx, testRoom, "hello", WithClientRequestID("epoch-reply-id"))
			return err
		}},
		{name: "query POST", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.postJSON[struct{}](ctx, PathQueryRecentMessages, map[string]string{"chatId": "42"}, true)
			return err
		}},
		{name: "strict media", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.postStrictJSON[struct{}](ctx, PathMediaChunk, map[string]string{"messageId": "42"})
			return err
		}},
		{name: "image", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendImage(ctx, testRoom, []byte("\x89PNG\r\n\x1a\npayload"), WithClientRequestID("epoch-image-id"))
			return err
		}},
		{name: "multiple images", call: func(ctx context.Context, c *APIClient) error {
			_, err := c.SendMultipleImages(ctx, testRoom, [][]byte{[]byte("\x89PNGfirst"), []byte("\x89PNGsecond")}, WithClientRequestID("epoch-images-id"))
			return err
		}},
		{name: "file ReaderAt", call: func(ctx context.Context, c *APIClient) error {
			data := []byte("file payload")
			file := NewReplyFile("fixture.txt", "text/plain", int64(len(data)), bytes.NewReader(data))
			_, err := c.SendFile(ctx, testRoom, file, WithClientRequestID("epoch-file-id"))

			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertEpochRefreshPreservesRequest(t, tc.call)
		})
		t.Run(tc.name+"/bootstrap failure", func(t *testing.T) {
			var protected int

			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != authEpochPath {
					protected++
				}

				return epochResponse(http.StatusOK, `{}`), nil
			}))
			err := tc.call(t.Context(), client)

			if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) || protected != 0 {
				t.Fatalf("bootstrap failure: error=%v protected=%d", err, protected)
			}
		})
	}
}

func assertEpochRefreshPreservesRequest(t *testing.T, call func(context.Context, *APIClient) error) {
	t.Helper()

	var (
		discoveries, protected      int
		originalBody                []byte
		originalNonce, originalType string
	)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)

	defer cancel()

	deadline, _ := ctx.Deadline()
	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got, ok := req.Context().Deadline(); !ok || !got.Equal(deadline) {
			t.Error("original deadline was changed")
		}

		if req.URL.Path == authEpochPath {
			discoveries++

			epoch := testAuthEpoch

			if discoveries > 1 {
				epoch = testNextAuthEpoch
			}

			return epochResponse(http.StatusOK, epochDocument(epoch)), nil
		}

		protected++

		body := readEpochRequestBody(t, req)
		epoch := testAuthEpoch

		if protected > 1 {
			epoch = testNextAuthEpoch
		}

		assertEpochSignature(t, req, body, epoch)

		if protected == 1 {
			originalBody = body
			originalNonce = req.Header.Get(HeaderIrisNonce)
			originalType = req.Header.Get("Content-Type")

			return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
		}

		if !bytes.Equal(body, originalBody) || req.Header.Get("Content-Type") != originalType {
			t.Error("epoch refresh changed payload, clientRequestId or multipart boundary")
		}

		if req.Header.Get(HeaderIrisNonce) == originalNonce {
			t.Error("epoch refresh reused nonce")
		}

		return epochResponse(http.StatusOK, `{}`), nil
	}))

	if err := call(ctx, client); err != nil {
		t.Fatal(err)
	}

	if protected != 2 || discoveries != 2 {
		t.Fatalf("refresh counts: protected=%d discoveries=%d", protected, discoveries)
	}
}

func readEpochRequestBody(t *testing.T, req *http.Request) []byte {
	t.Helper()

	if req.Body == nil {
		return nil
	}

	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Error(err)
	}

	return body
}

func assertEpochSignature(t *testing.T, req *http.Request, body []byte, epoch string) {
	t.Helper()

	nonce := req.Header.Get(HeaderIrisNonce)
	if len(nonce) != 97 || !strings.HasPrefix(nonce, epoch+".") || !isLowerSHA256Hex(nonce[33:]) {
		t.Error("protected nonce does not satisfy epoch/random contract")
	}

	bodyHash := signing.SHA256HexBytes(body)
	if req.Header.Get(HeaderIrisBodySHA256) != bodyHash {
		t.Error("signed body hash changed")
	}

	want, err := signing.SignIrisCanonicalWithSigner(signing.NewHMACSigner(testEpochSecret), req.Method, req.URL.RequestURI(), req.Header.Get(HeaderIrisTimestamp), nonce, bodyHash)
	if err != nil || want != req.Header.Get(HeaderIrisSignature) {
		t.Error("epoch nonce is not bound to canonical HMAC")
	}
}

func TestAuthEpochRefreshRequiresVerifiedExactRejection(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   int
		body     string
		attempts int
	}{
		{name: "generic 401", status: http.StatusUnauthorized, body: `{"message":"unauthorized"}`, attempts: 1},
		{name: "other code", status: http.StatusUnauthorized, body: `{"code":"OTHER"}`, attempts: 1},
		{name: "padded code", status: http.StatusUnauthorized, body: `{"code":" AUTH_EPOCH_MISMATCH "}`, attempts: 1},
		{name: "wrong status", status: http.StatusServiceUnavailable, body: testEpochMismatch, attempts: 1},
		{name: "duplicate code", status: http.StatusUnauthorized, body: `{"code":"AUTH_EPOCH_MISMATCH","code":"AUTH_EPOCH_MISMATCH"}`, attempts: 1},
		{name: "second JSON", status: http.StatusUnauthorized, body: testEpochMismatch + `{}`, attempts: 1},
		{name: "oversized code document", status: http.StatusUnauthorized, body: testEpochMismatch + strings.Repeat(" ", httpErrorBodyParseMaxLen), attempts: 1},
		{name: "repeated restart", status: http.StatusUnauthorized, body: testEpochMismatch, attempts: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var discoveries, attempts int

			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == authEpochPath {
					discoveries++
					return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
				}

				attempts++

				readEpochRequestBody(t, req)

				return epochResponse(tc.status, tc.body), nil
			}), WithReplyRetry(5))
			_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("epoch-id"))

			if err == nil || attempts != tc.attempts || discoveries != tc.attempts {
				t.Fatalf("error=%v attempts=%d discoveries=%d", err, attempts, discoveries)
			}

			if tc.attempts == 2 && client.epoch.value != "" {
				t.Error("terminal epoch mismatch left a stale cache entry")
			}
		})
	}
}

func TestAuthEpochRefreshBudgetSurvivesOuterRetry(t *testing.T) {
	var discoveries, attempts int

	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			discoveries++
			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}

		attempts++

		readEpochRequestBody(t, req)

		if attempts == 2 {
			return epochResponse(http.StatusTooManyRequests, `{}`), nil
		}

		return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
	}), WithReplyRetry(3))
	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("epoch-id"))

	if !errors.Is(err, ErrNotSent) || !errors.Is(err, ErrRetryable) || errors.Is(err, ErrAuthFailed) || attempts != 3 || discoveries != 2 {
		t.Fatalf("error=%v attempts=%d discoveries=%d", err, attempts, discoveries)
	}
}

func TestAuthEpochRefreshPreservesUnknownOutcome(t *testing.T) {
	for _, bootstrapFails := range []bool{false, true} {
		t.Run(strconv.FormatBool(bootstrapFails), func(t *testing.T) {
			var discoveries, attempts int

			network := errors.New("synthetic connection loss")
			client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == authEpochPath {
					discoveries++
					if discoveries == 2 && bootstrapFails {
						return epochResponse(http.StatusOK, `{}`), nil
					}

					return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
				}

				attempts++

				readEpochRequestBody(t, req)

				if attempts == 1 {
					return nil, network
				}

				return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
			}), WithReplyRetry(2))
			_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("epoch-id"))
			assertUnknownReplyOutcome(t, err, network)

			if errors.Is(err, ErrNotSent) || errors.Is(err, ErrAuthFailed) || errors.Is(err, ErrPermanent) {
				t.Fatalf("epoch failure downgraded outcome unknown: %v", err)
			}
		})
	}
}

func TestAuthEpochConcurrentStaleRequestsShareRefresh(t *testing.T) {
	const callers = 12

	var oldRequests, discoveries atomic.Int32

	allOld := make(chan struct{})
	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			discoveries.Add(1)

			return epochResponse(http.StatusOK, epochDocument(testNextAuthEpoch)), nil
		}

		if strings.HasPrefix(req.Header.Get(HeaderIrisNonce), testAuthEpoch+".") {
			if oldRequests.Add(1) == callers {
				close(allOld)
			}

			select {
			case <-allOld:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}

			return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
		}

		return epochResponse(http.StatusOK, `{}`), nil
	}))

	client.epoch.value = testAuthEpoch

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)

	defer cancel()

	var wg sync.WaitGroup

	for range callers {
		wg.Go(func() {
			if _, err := client.GetRooms(ctx); err != nil {
				t.Error(err)
			}
		})
	}

	wg.Wait()

	if discoveries.Load() != 1 || oldRequests.Load() != callers {
		t.Fatalf("concurrent refresh counts: discoveries=%d old=%d", discoveries.Load(), oldRequests.Load())
	}
}

func TestAuthEpochWaiterCancellationDoesNotWaitForNetwork(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			close(started)

			select {
			case <-release:
			case <-req.Context().Done():
				return nil, req.Context().Err()
			}

			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}

		return epochResponse(http.StatusOK, `{}`), nil
	}))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)

	defer cancel()

	owner := make(chan error, 1)

	go func() {
		_, err := client.GetRooms(ctx)
		owner <- err
	}()

	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("bootstrap did not start")
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Millisecond)

	defer cancelWait()

	_, err := client.GetRooms(waitCtx)
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) {
		t.Errorf("waiter error=%v", err)
	}

	close(release)

	if err := <-owner; err != nil {
		t.Fatal(err)
	}
}

func TestAuthEpochRedirectCannotCrossTrustBoundary(t *testing.T) {
	var redirected atomic.Int32

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		testsupport.WriteResponse(t, w, epochDocument(testAuthEpoch))
	}))

	defer other.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+authEpochPath, http.StatusFound)
	}))

	defer origin.Close()

	client, err := NewClient(WithBaseURL(origin.URL), WithBotToken(testEpochSecret), WithHTTPClient(origin.Client()))
	if err != nil {
		t.Fatal(err)
	}

	_, err = client.GetRooms(t.Context())
	if !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) || redirected.Load() != 0 {
		t.Fatalf("redirect error=%v redirected=%d", err, redirected.Load())
	}
}

func TestAuthEpochHTTP3DiscoveryAndResign(t *testing.T) {
	var discoveries, protected atomic.Int32

	server := startLocalHTTP3ServerWithHandler(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 3 {
			t.Error("epoch/protected request did not use HTTP/3")
		}

		if r.URL.Path == authEpochPath {
			epoch := testAuthEpoch

			if discoveries.Add(1) > 1 {
				epoch = testNextAuthEpoch
			}

			testsupport.WriteResponse(t, w, epochDocument(epoch))

			return
		}

		body := readEpochRequestBody(t, r)

		if protected.Add(1) == 1 {
			assertEpochSignature(t, r, body, testAuthEpoch)
			w.WriteHeader(http.StatusUnauthorized)
			testsupport.WriteResponse(t, w, testEpochMismatch)

			return
		}

		assertEpochSignature(t, r, body, testNextAuthEpoch)

		var reply ReplyRequest

		if err := jsonv2.Unmarshal(body, &reply); err != nil || reply.ClientRequestID == nil || *reply.ClientRequestID != "h3-epoch-id" {
			t.Error("HTTP/3 replay lost original request")
		}

		testsupport.WriteResponse(t, w, `{"requestId":"h3-request"}`)
	}))

	client, err := NewClient(
		WithBaseURL("https://localhost:"+strconv.Itoa(server.port)), WithBotToken(testEpochSecret),
		WithTransport(TransportH3), WithH3CACertFile(server.certFile), WithH3ServerName("localhost"),
	)
	if err != nil {
		t.Fatal(err)
	}

	testsupport.CloseOnCleanup(t, "client.Close", client.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)

	defer cancel()

	response, err := client.SendMessageAccepted(ctx, testRoom, "hello", WithClientRequestID("h3-epoch-id"))
	if err != nil || response == nil || discoveries.Load() != 2 || protected.Load() != 2 {
		t.Fatalf("HTTP/3 epoch flow: error=%v discoveries=%d protected=%d", err, discoveries.Load(), protected.Load())
	}
}

func TestAuthEpochAttemptTimeoutIsShared(t *testing.T) {
	var (
		deadline               time.Time
		discoveries, protected int
	)

	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		got, ok := req.Context().Deadline()
		if !ok {
			t.Error("HTTP client timeout was not bound to the original attempt")
		}

		if deadline.IsZero() {
			deadline = got
		} else if !deadline.Equal(got) {
			t.Error("discovery or resign reset the HTTP client timeout")
		}

		if req.URL.Path == authEpochPath {
			discoveries++
			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}

		protected++
		if protected == 1 {
			return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
		}

		return epochResponse(http.StatusOK, `{}`), nil
	}))

	client.client.Timeout = time.Second

	if _, err := client.GetRooms(t.Context()); err != nil {
		t.Fatal(err)
	}

	if discoveries != 2 || protected != 2 {
		t.Fatalf("discoveries=%d protected=%d", discoveries, protected)
	}
}

func TestAuthEpochRefreshHonorsOriginalDeadline(t *testing.T) {
	var discoveries, protected int

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)

	defer cancel()

	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == authEpochPath {
			discoveries++
			if discoveries == 2 {
				<-req.Context().Done()

				return nil, req.Context().Err()
			}

			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}

		protected++

		return epochResponse(http.StatusUnauthorized, testEpochMismatch), nil
	}))
	_, err := client.GetRooms(ctx)

	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) ||
		discoveries != 2 || protected != 1 {
		t.Fatalf("deadline error=%v discoveries=%d protected=%d", err, discoveries, protected)
	}
}

func TestAuthEpochBootstrapTransportFailureIsNotSent(t *testing.T) {
	network := errors.New("synthetic bootstrap network failure")

	var requests int

	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++

		if req.URL.Path != authEpochPath {
			t.Error("protected request escaped failed bootstrap")
		}

		return nil, network
	}), WithReplyRetry(3))
	_, err := client.SendMessageAccepted(t.Context(), testRoom, "hello", WithClientRequestID("epoch-id"))

	if !errors.Is(err, network) || !errors.Is(err, ErrNotSent) || errors.Is(err, ErrTransport) || requests != 1 {
		t.Fatalf("bootstrap transport error=%v requests=%d", err, requests)
	}
}

func TestAuthEpochDiscoveryRetainsDeploymentPrefix(t *testing.T) {
	var discoveries int

	client := newEpochTestClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/tenant/iris"+authEpochPath {
			discoveries++
			return epochResponse(http.StatusOK, epochDocument(testAuthEpoch)), nil
		}

		if req.URL.Path != "/tenant/iris"+PathRooms {
			t.Error("request escaped deployment prefix")
		}

		return epochResponse(http.StatusOK, `{}`), nil
	}), WithBaseURL("https://iris.test/tenant/iris/"))

	if _, err := client.GetRooms(t.Context()); err != nil {
		t.Fatal(err)
	}

	if discoveries != 1 {
		t.Fatalf("discoveries=%d", discoveries)
	}
}
