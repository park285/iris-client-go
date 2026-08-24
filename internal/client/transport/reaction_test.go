package transport

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/testsupport"
)

type capturedReactionRequest struct {
	method      string
	path        string
	signature   string
	contentType string
	body        ReactionRequest
}

func TestAPIClientSendReactionPostsTypedRequest(t *testing.T) {
	t.Parallel()

	var captured capturedReactionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = capturedReactionRequest{
			method:      r.Method,
			path:        r.URL.Path,
			signature:   r.Header.Get(HeaderIrisSignature),
			contentType: r.Header.Get("Content-Type"),
		}

		if err := jsonv2.UnmarshalRead(r.Body, &captured.body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if err := jsonv2.MarshalWrite(w, ReactionResponse{
			Success:   true,
			Status:    ReactionStatusSent,
			RequestID: "reaction:req-1",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	linkID := int64(77)
	client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

	resp, err := client.SendReaction(t.Context(), 42, ReactionRequest{
		RequestID: "reaction:req-1",
		ChatLogID: "123",
		LinkID:    &linkID,
		Revision:  9,
		Add:       []Reaction{ReactionLike, ReactionHeart},
	})
	if err != nil {
		t.Fatalf("SendReaction() error = %v", err)
	}

	assertTypedReactionRequestEnvelope(t, &captured)
	assertTypedReactionRequestBody(t, captured.body, linkID)
	assertTypedReactionResponse(t, resp)
}

func assertTypedReactionRequestEnvelope(t *testing.T, got *capturedReactionRequest) {
	t.Helper()

	if got.method != http.MethodPost {
		t.Fatalf("method = %q, want POST", got.method)
	}

	if got.path != "/rooms/42/reactions" {
		t.Fatalf("path = %q, want /rooms/42/reactions", got.path)
	}

	if got.signature == "" {
		t.Fatal("signature header missing")
	}

	if got.contentType != contentTypeJSON {
		t.Fatalf("Content-Type = %q, want application/json", got.contentType)
	}
}

func assertTypedReactionRequestBody(t *testing.T, got ReactionRequest, wantLinkID int64) {
	t.Helper()

	if got.RequestID != "reaction:req-1" || got.ChatLogID != "123" || got.Revision != 9 {
		t.Fatalf("request identity = %+v", got)
	}

	if got.LinkID == nil || *got.LinkID != wantLinkID {
		t.Fatalf("LinkID = %v, want %d", got.LinkID, wantLinkID)
	}

	if len(got.Add) != 2 || got.Add[0] != ReactionLike || got.Add[1] != ReactionHeart {
		t.Fatalf("Add = %+v", got.Add)
	}

	if len(got.Follow) != 0 || len(got.Remove) != 0 {
		t.Fatalf("unexpected non-add operations: %+v", got)
	}
}

func assertTypedReactionResponse(t *testing.T, resp *ReactionResponse) {
	t.Helper()

	if resp == nil || !resp.Success || resp.Status != ReactionStatusSent || resp.RequestID != "reaction:req-1" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAPIClientSendReactionAcceptsFollowAndRemove(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any

		if err := jsonv2.UnmarshalRead(r.Body, &body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if _, ok := body["add"]; ok {
			t.Fatalf("follow/remove request unexpectedly included add: %v", body)
		}

		if _, ok := body["follow"]; !ok {
			t.Fatalf("follow missing: %v", body)
		}

		if _, ok := body["remove"]; !ok {
			t.Fatalf("remove missing: %v", body)
		}

		testsupport.WriteJSON(t, w, ReactionResponse{Success: true, Status: ReactionStatusSent, RequestID: "reaction:req-2"})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

	resp, err := client.SendReaction(t.Context(), 42, ReactionRequest{
		RequestID: "reaction:req-2",
		ChatLogID: "123",
		Follow:    []Reaction{ReactionLike},
		Remove:    []Reaction{ReactionHeart},
	})
	if err != nil {
		t.Fatalf("SendReaction() error = %v", err)
	}

	if resp == nil || resp.Status != ReactionStatusSent {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAPIClientSendReactionRejectsInvalidRequestsBeforeTransport(t *testing.T) {
	t.Parallel()

	var called atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called.Store(true)
	}))

	t.Cleanup(server.Close)

	client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

	t.Cleanup(func() {
		if called.Load() {
			t.Error("invalid request reached transport")
		}
	})

	for _, test := range invalidReactionRequestCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.SendReaction(t.Context(), 42, test.req)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}

	if _, err := client.SendReaction(t.Context(), 0, ReactionRequest{
		RequestID: testReactionRequestID,
		ChatLogID: "123",
		Add:       []Reaction{ReactionLike},
	}); err == nil || !strings.Contains(err.Error(), "room must be positive") {
		t.Fatalf("invalid room error = %v", err)
	}
}

type invalidReactionRequestCase struct {
	name string
	req  ReactionRequest
	want string
}

func invalidReactionRequestCases() []invalidReactionRequestCase {
	return []invalidReactionRequestCase{
		{
			name: "empty operations",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123"},
			want: "at least one reaction operation",
		},
		{
			name: "mixed add and remove",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123", Add: []Reaction{ReactionLike}, Remove: []Reaction{ReactionHeart}},
			want: "add cannot be combined",
		},
		{
			name: "unknown reaction",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123", Add: []Reaction{Reaction("party")}},
			want: "unsupported reaction",
		},
		{
			name: "blank request id",
			req:  ReactionRequest{ChatLogID: "123", Add: []Reaction{ReactionLike}},
			want: "requestId",
		},
		{
			name: "blank chat log id",
			req:  ReactionRequest{RequestID: testReactionRequestID, Add: []Reaction{ReactionLike}},
			want: "chatLogId",
		},
		{
			name: "nonnumeric chat log id",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "log-123", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "leading zero chat log id",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "0123", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "whitespace chat log id",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: " 123 ", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "negative revision",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123", Revision: -1, Add: []Reaction{ReactionLike}},
			want: "revision must be non-negative",
		},
		{
			name: "duplicate add",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123", Add: []Reaction{ReactionLike, ReactionLike}},
			want: "duplicate reaction",
		},
		{
			name: "overlapping follow remove",
			req:  ReactionRequest{RequestID: testReactionRequestID, ChatLogID: "123", Follow: []Reaction{ReactionLike}, Remove: []Reaction{ReactionLike}},
			want: "follow and remove overlap",
		},
	}
}

func TestAPIClientSendReactionPropagatesContextAndTransportErrors(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); !errors.Is(err, context.Canceled) {
			return nil, errors.New("request context was not canceled")
		}

		return nil, context.Canceled
	})
	client := NewAPIClient("http://localhost", "unused-bot-token", WithRoundTripper(rt))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.SendReaction(ctx, 42, ReactionRequest{
		RequestID: "reaction:req-4",
		ChatLogID: "123",
		Add:       []Reaction{ReactionLike},
	})
	if err == nil {
		t.Fatal("SendReaction() error = nil, want context/transport error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestAPIClientSendReactionRejectsMismatchedResponseRequestID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		testsupport.WriteJSON(t, w, ReactionResponse{
			Success:   true,
			Status:    ReactionStatusSent,
			RequestID: "reaction:req-other",
		})
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
	resp, err := client.SendReaction(t.Context(), 42, ReactionRequest{
		RequestID: "reaction:req-6",
		ChatLogID: "123",
		Add:       []Reaction{ReactionLike},
	})

	if err == nil || !strings.Contains(err.Error(), "response requestId does not match request") {
		t.Fatalf("SendReaction() error = %v, want mismatched requestId error", err)
	}

	if resp != nil {
		t.Fatalf("SendReaction() response = %+v, want nil", resp)
	}
}

func TestAPIClientSendReactionRejectsNonCanonicalResponses(t *testing.T) {
	t.Parallel()

	const validRequest = `{"success":true,"status":"sent","requestId":"reaction:req-5"}`

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown field", body: validRequest[:len(validRequest)-1] + `,"extra":true}`},
		{name: "duplicate field", body: `{"success":true,"status":"sent","requestId":"reaction:req-5","requestId":"reaction:req-6"}`},
		{name: "missing request id", body: `{"success":true,"status":"sent"}`},
		{name: "trailing JSON", body: validRequest + `{}`},
		{name: "accepted status", body: `{"success":true,"status":"accepted","requestId":"reaction:req-5"}`},
		{name: "sent status with false success", body: `{"success":false,"status":"sent","requestId":"reaction:req-5"}`},
		{name: "failed status with true success", body: `{"success":true,"status":"failed","requestId":"reaction:req-5"}`},
		{name: "outcome unknown with true success", body: `{"success":true,"status":"outcome_unknown","requestId":"reaction:req-5"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				testsupport.WriteResponse(t, w, test.body)
			}))

			defer server.Close()

			client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

			_, err := client.SendReaction(t.Context(), 42, ReactionRequest{
				RequestID: "reaction:req-5",
				ChatLogID: "123",
				Add:       []Reaction{ReactionLike},
			})
			if err == nil {
				t.Fatal("SendReaction() error = nil, want strict response rejection")
			}
		})
	}
}
