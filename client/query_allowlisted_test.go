package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestH2CClientQueryRoomSummary(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		resp := RoomSummary{ChatID: 123, ActiveMembersCount: intPtr(10)}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.QueryRoomSummary(t.Context(), 123)
	if err != nil {
		t.Fatalf("QueryRoomSummary() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != PathQueryRoomSummary {
		t.Fatalf("path = %q, want %q", gotPath, PathQueryRoomSummary)
	}
	if resp.ChatID != 123 {
		t.Errorf("ChatID = %d, want 123", resp.ChatID)
	}
}

func TestH2CClientQueryMemberStats(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		resp := StatsResponse{
			ChatID:        1,
			Period:        PeriodRange{From: 0, To: 1},
			TotalMessages: 5,
			ActiveMembers: 2,
			TopMembers:    []MemberStats{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	period := "7d"
	resp, err := c.QueryMemberStats(t.Context(), QueryMemberStatsRequest{ChatID: 1, Period: &period, Limit: 20})
	if err != nil {
		t.Fatalf("QueryMemberStats() error = %v", err)
	}

	if gotPath != PathQueryMemberStats {
		t.Fatalf("path = %q, want %q", gotPath, PathQueryMemberStats)
	}
	if resp.TotalMessages != 5 {
		t.Errorf("TotalMessages = %d, want 5", resp.TotalMessages)
	}
}

func TestH2CClientQueryRecentThreads(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		origin := "hello"
		active := int64(1000)
		resp := ThreadListResponse{
			ChatID: 1,
			Threads: []ThreadSummary{
				{ThreadID: "100", OriginMessage: &origin, MessageCount: 3, LastActiveAt: &active},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.QueryRecentThreads(t.Context(), 1)
	if err != nil {
		t.Fatalf("QueryRecentThreads() error = %v", err)
	}

	if gotPath != PathQueryRecentThreads {
		t.Fatalf("path = %q, want %q", gotPath, PathQueryRecentThreads)
	}
	if len(resp.Threads) != 1 {
		t.Fatalf("len(Threads) = %d, want 1", len(resp.Threads))
	}
	if resp.Threads[0].ThreadID != "100" {
		t.Errorf("ThreadID = %s, want 100", resp.Threads[0].ThreadID)
	}
}

func TestH2CClientQueryRecentMessages(t *testing.T) {
	t.Parallel()

	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		resp := RecentMessagesResponse{
			ChatID: 1,
			Messages: []RecentMessage{
				{ID: 1, ChatID: 1, UserID: 2, Message: "hi", Type: 1, CreatedAt: 1000},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.QueryRecentMessages(t.Context(), QueryRecentMessagesRequest{ChatID: 1, Limit: 50})
	if err != nil {
		t.Fatalf("QueryRecentMessages() error = %v", err)
	}

	if gotPath != PathQueryRecentMessages {
		t.Fatalf("path = %q, want %q", gotPath, PathQueryRecentMessages)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("len(Messages) = %d, want 1", len(resp.Messages))
	}
	if resp.Messages[0].ID != 1 {
		t.Errorf("ID = %d, want 1", resp.Messages[0].ID)
	}
}

func TestH2CClientGetThreads(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		resp := ThreadListResponse{ChatID: 42, Threads: []ThreadSummary{}}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetThreads(t.Context(), 42)
	if err != nil {
		t.Fatalf("GetThreads() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/rooms/42/threads" {
		t.Fatalf("path = %q, want /rooms/42/threads", gotPath)
	}
	if resp.ChatID != 42 {
		t.Errorf("ChatID = %d, want 42", resp.ChatID)
	}
}

func TestH2CClientGetRoomEvents(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit = %s, want 10", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("after") != "5" {
			t.Errorf("after = %s, want 5", r.URL.Query().Get("after"))
		}

		// 서버는 raw JSON array를 반환합니다.
		resp := []RoomEventRecord{
			{ID: 6, ChatID: 42, EventType: "member_event", UserID: 1, Payload: "{}", CreatedAt: 1000},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetRoomEvents(t.Context(), 42, 10, 5)
	if err != nil {
		t.Fatalf("GetRoomEvents() error = %v", err)
	}

	if gotMethod != http.MethodGet {
		t.Fatalf("method = %s, want GET", gotMethod)
	}
	if gotPath != "/rooms/42/events" {
		t.Fatalf("path = %q, want /rooms/42/events", gotPath)
	}
	if len(resp) != 1 {
		t.Fatalf("len = %d, want 1", len(resp))
	}
	if resp[0].ID != 6 {
		t.Errorf("ID = %d, want 6", resp[0].ID)
	}
}

func TestH2CClientGetRoomEventsNoParams(t *testing.T) {
	t.Parallel()

	var gotQuery string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery

		if err := json.NewEncoder(w).Encode([]RoomEventRecord{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetRoomEvents(t.Context(), 42, 0, 0)
	if err != nil {
		t.Fatalf("GetRoomEvents() error = %v", err)
	}

	if gotQuery != "" {
		t.Fatalf("query = %q, want empty", gotQuery)
	}
	if len(resp) != 0 {
		t.Fatalf("len = %d, want 0", len(resp))
	}
}

func TestH2CClientQueryRoomSummaryError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	_, err := c.QueryRoomSummary(t.Context(), 1)
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %q, want 403 mention", err.Error())
	}
}

func intPtr(v int) *int { return &v }
