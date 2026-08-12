package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestH2CClientSendReactionPostsTypedRequest(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotMethod string
	var gotBody ReactionRequest
	var gotSignature string
	var gotContentType string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotContentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if err := json.NewEncoder(w).Encode(ReactionResponse{
			Success:   true,
			Status:    ReactionStatusSent,
			RequestID: "reaction:req-1",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	linkID := int64(77)
	client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
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

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/rooms/42/reactions" {
		t.Fatalf("path = %q, want /rooms/42/reactions", gotPath)
	}
	if gotSignature == "" {
		t.Fatal("signature header missing")
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if gotBody.RequestID != "reaction:req-1" || gotBody.ChatLogID != "123" || gotBody.Revision != 9 {
		t.Fatalf("request identity = %+v", gotBody)
	}
	if gotBody.LinkID == nil || *gotBody.LinkID != linkID {
		t.Fatalf("LinkID = %v, want %d", gotBody.LinkID, linkID)
	}
	if len(gotBody.Add) != 2 || gotBody.Add[0] != ReactionLike || gotBody.Add[1] != ReactionHeart {
		t.Fatalf("Add = %+v", gotBody.Add)
	}
	if len(gotBody.Follow) != 0 || len(gotBody.Remove) != 0 {
		t.Fatalf("unexpected non-add operations: %+v", gotBody)
	}
	if resp == nil || !resp.Success || resp.Status != ReactionStatusSent || resp.RequestID != "reaction:req-1" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestH2CClientSendReactionAcceptsFollowAndRemove(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
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
		_ = json.NewEncoder(w).Encode(ReactionResponse{Success: true, Status: ReactionStatusSent, RequestID: "reaction:req-2"})
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
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

func TestH2CClientSendReactionRejectsInvalidRequestsBeforeTransport(t *testing.T) {
	t.Parallel()

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()
	client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

	tests := []struct {
		name string
		req  ReactionRequest
		want string
	}{
		{
			name: "empty operations",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123"},
			want: "at least one reaction operation",
		},
		{
			name: "mixed add and remove",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123", Add: []Reaction{ReactionLike}, Remove: []Reaction{ReactionHeart}},
			want: "add cannot be combined",
		},
		{
			name: "unknown reaction",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123", Add: []Reaction{Reaction("party")}},
			want: "unsupported reaction",
		},
		{
			name: "blank request id",
			req:  ReactionRequest{ChatLogID: "123", Add: []Reaction{ReactionLike}},
			want: "requestId",
		},
		{
			name: "blank chat log id",
			req:  ReactionRequest{RequestID: "reaction:req-3", Add: []Reaction{ReactionLike}},
			want: "chatLogId",
		},
		{
			name: "nonnumeric chat log id",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "log-123", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "leading zero chat log id",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "0123", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "whitespace chat log id",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: " 123 ", Add: []Reaction{ReactionLike}},
			want: "canonical positive integer",
		},
		{
			name: "negative revision",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123", Revision: -1, Add: []Reaction{ReactionLike}},
			want: "revision must be non-negative",
		},
		{
			name: "duplicate add",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123", Add: []Reaction{ReactionLike, ReactionLike}},
			want: "duplicate reaction",
		},
		{
			name: "overlapping follow remove",
			req:  ReactionRequest{RequestID: "reaction:req-3", ChatLogID: "123", Follow: []Reaction{ReactionLike}, Remove: []Reaction{ReactionLike}},
			want: "follow and remove overlap",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.SendReaction(t.Context(), 42, test.req)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
	if called {
		t.Fatal("invalid request reached transport")
	}

	if _, err := client.SendReaction(t.Context(), 0, ReactionRequest{
		RequestID: "reaction:req-3",
		ChatLogID: "123",
		Add:       []Reaction{ReactionLike},
	}); err == nil || !strings.Contains(err.Error(), "room must be positive") {
		t.Fatalf("invalid room error = %v", err)
	}
}

func TestH2CClientSendReactionPropagatesContextAndTransportErrors(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); !errors.Is(err, context.Canceled) {
			return nil, errors.New("request context was not canceled")
		}
		return nil, context.Canceled
	})
	client := NewH2CClient("http://localhost", "unused-bot-token", WithRoundTripper(rt))
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

func TestH2CClientSendReactionRejectsMismatchedResponseRequestID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(ReactionResponse{
			Success:   true,
			Status:    ReactionStatusSent,
			RequestID: "reaction:req-other",
		})
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
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

func TestH2CClientSendReactionRejectsNonCanonicalResponses(t *testing.T) {
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
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
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
