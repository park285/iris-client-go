package transport

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

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chatId":1,"messages":[
			{"sequenceId":41,"chatLogId":"chat-log-1","chatId":1,"userId":2,"message":"hi","type":1,"createdAt":1000,"threadId":"thread-alpha"},
			{"sequenceId":42,"chatLogId":"chat-log-2","chatId":1,"userId":3,"message":"hello","type":1,"createdAt":1001,"threadId":9001}
		]}`))
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
	if len(resp.Messages) != 2 {
		t.Fatalf("len(Messages) = %d, want 2", len(resp.Messages))
	}
	if resp.Messages[0].SequenceID != 41 {
		t.Errorf("SequenceID = %d, want 41", resp.Messages[0].SequenceID)
	}
	if resp.Messages[0].ChatLogID != "chat-log-1" {
		t.Errorf("ChatLogID = %#v, want chat-log-1", resp.Messages[0].ChatLogID)
	}
	if resp.Messages[0].ThreadID == nil || *resp.Messages[0].ThreadID != "thread-alpha" {
		t.Errorf("ThreadID[0] = %#v, want thread-alpha", resp.Messages[0].ThreadID)
	}
	if resp.Messages[1].ThreadID == nil || *resp.Messages[1].ThreadID != "9001" {
		t.Errorf("ThreadID[1] = %#v, want 9001", resp.Messages[1].ThreadID)
	}
}

func TestH2CClientQueryRecentMessagesRejectsInvalidThreadIDType(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chatId":1,"messages":[
			{"sequenceId":41,"chatLogId":"chat-log-1","chatId":1,"userId":2,"message":"hi","type":1,"createdAt":1000,"threadId":true}
		]}`))
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	_, err := c.QueryRecentMessages(t.Context(), QueryRecentMessagesRequest{ChatID: 1, Limit: 50})
	if err == nil {
		t.Fatal("QueryRecentMessages() error = nil, want invalid threadId error")
	}
	if !strings.Contains(err.Error(), "decode threadId") {
		t.Fatalf("QueryRecentMessages() error = %v, want decode threadId", err)
	}
}

func TestH2CClientQueryRecentMessagesSendsCursorFields(t *testing.T) {
	t.Parallel()

	var gotBody QueryRecentMessagesRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		resp := RecentMessagesResponse{ChatID: 1, Messages: []RecentMessage{}}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	afterID := int64(100)
	sinceCreatedAt := int64(1_800_000_000)
	untilCreatedAt := int64(1_800_172_800)
	threadID := "thread-alpha"

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	_, err := c.QueryRecentMessages(t.Context(), QueryRecentMessagesRequest{
		ChatID:         1,
		Limit:          300,
		AfterID:        &afterID,
		SinceCreatedAt: &sinceCreatedAt,
		UntilCreatedAt: &untilCreatedAt,
		ThreadID:       &threadID,
	})
	if err != nil {
		t.Fatalf("QueryRecentMessages() error = %v", err)
	}

	if gotBody.AfterID == nil || *gotBody.AfterID != afterID {
		t.Fatalf("AfterID = %#v, want %d", gotBody.AfterID, afterID)
	}
	if gotBody.ThreadID == nil || *gotBody.ThreadID != threadID {
		t.Fatalf("ThreadID = %#v, want %s", gotBody.ThreadID, threadID)
	}
	if gotBody.SinceCreatedAt == nil || *gotBody.SinceCreatedAt != sinceCreatedAt {
		t.Fatalf("SinceCreatedAt = %#v, want %d", gotBody.SinceCreatedAt, sinceCreatedAt)
	}
	if gotBody.UntilCreatedAt == nil || *gotBody.UntilCreatedAt != untilCreatedAt {
		t.Fatalf("UntilCreatedAt = %#v, want %d", gotBody.UntilCreatedAt, untilCreatedAt)
	}
	if gotBody.BeforeID != nil {
		t.Fatalf("BeforeID = %#v, want nil", gotBody.BeforeID)
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
			{ID: 6, ChatID: 42, EventType: EventTypeMemberNicknameUpdated, UserID: 1, Payload: "{}", CreatedAtMs: 1778226335000},
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

func TestH2CClientGetRoomEventsByTypeSendsEventType(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		if r.URL.Query().Get("eventType") != "member_nickname_updated" {
			t.Errorf("eventType = %s, want member_nickname_updated", r.URL.Query().Get("eventType"))
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("limit = %s, want 10", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("after") != "5" {
			t.Errorf("after = %s, want 5", r.URL.Query().Get("after"))
		}

		resp := []RoomEventRecord{
			{ID: 7, ChatID: 42, EventType: EventTypeMemberNicknameUpdated, UserID: 99, Payload: "{}", CreatedAtMs: 1778226335000},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetRoomEventsByType(t.Context(), 42, "member_nickname_updated", 10, 5)
	if err != nil {
		t.Fatalf("GetRoomEventsByType() error = %v", err)
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
	if resp[0].EventType != EventTypeMemberNicknameUpdated {
		t.Errorf("EventType = %s, want member_nickname_updated", resp[0].EventType)
	}
}

func TestH2CClientGetRoomEventsByTypeEmptyEventTypeOmitsEventType(t *testing.T) {
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
	resp, err := c.GetRoomEventsByType(t.Context(), 42, "", 0, 0)
	if err != nil {
		t.Fatalf("GetRoomEventsByType() error = %v", err)
	}

	if gotQuery != "" {
		t.Fatalf("query = %q, want empty", gotQuery)
	}
	if len(resp) != 0 {
		t.Fatalf("len = %d, want 0", len(resp))
	}
}

func TestH2CClientGetRoomUserEvents(t *testing.T) {
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
		if r.URL.Query().Get("userId") != "99" {
			t.Errorf("userId = %s, want 99", r.URL.Query().Get("userId"))
		}

		resp := []RoomEventRecord{
			{ID: 6, ChatID: 42, EventType: EventTypeMemberNicknameUpdated, UserID: 99, Payload: "{}", CreatedAtMs: 1778226335000},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetRoomUserEvents(t.Context(), 42, 99, 10, 5)
	if err != nil {
		t.Fatalf("GetRoomUserEvents() error = %v", err)
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
	if resp[0].UserID != 99 {
		t.Errorf("UserID = %d, want 99", resp[0].UserID)
	}
	if resp[0].CreatedAtMs != 1778226335000 {
		t.Errorf("CreatedAtMs = %d, want 1778226335000", resp[0].CreatedAtMs)
	}
}

func TestH2CClientGetRoomUserEventsByTypeSendsUserIDAndEventType(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		if r.URL.Query().Get("eventType") != "member_nickname_updated" {
			t.Errorf("eventType = %s, want member_nickname_updated", r.URL.Query().Get("eventType"))
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit = %s, want 5", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("after") != "7" {
			t.Errorf("after = %s, want 7", r.URL.Query().Get("after"))
		}
		if r.URL.Query().Get("userId") != "99" {
			t.Errorf("userId = %s, want 99", r.URL.Query().Get("userId"))
		}

		resp := []RoomEventRecord{
			{ID: 8, ChatID: 42, EventType: EventTypeMemberNicknameUpdated, UserID: 99, Payload: "{}", CreatedAtMs: 1778226335000},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetRoomUserEventsByType(t.Context(), 42, 99, "member_nickname_updated", 5, 7)
	if err != nil {
		t.Fatalf("GetRoomUserEventsByType() error = %v", err)
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
	if resp[0].UserID != 99 {
		t.Errorf("UserID = %d, want 99", resp[0].UserID)
	}
	if resp[0].EventType != EventTypeMemberNicknameUpdated {
		t.Errorf("EventType = %s, want member_nickname_updated", resp[0].EventType)
	}
}

func TestH2CClientGetLatestRoomUserEventsByTypeSendsDescOrder(t *testing.T) {
	t.Parallel()

	var gotPath, gotMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method

		if r.URL.Query().Get("eventType") != "member_nickname_updated" {
			t.Errorf("eventType = %s, want member_nickname_updated", r.URL.Query().Get("eventType"))
		}
		if r.URL.Query().Get("limit") != "5" {
			t.Errorf("limit = %s, want 5", r.URL.Query().Get("limit"))
		}
		if r.URL.Query().Get("after") != "" {
			t.Errorf("after = %s, want empty", r.URL.Query().Get("after"))
		}
		if r.URL.Query().Get("order") != "desc" {
			t.Errorf("order = %s, want desc", r.URL.Query().Get("order"))
		}
		if r.URL.Query().Get("userId") != "99" {
			t.Errorf("userId = %s, want 99", r.URL.Query().Get("userId"))
		}

		resp := []RoomEventRecord{
			{ID: 9, ChatID: 42, EventType: EventTypeMemberNicknameUpdated, UserID: 99, Payload: "{}", CreatedAtMs: 1778226335000},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	c := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
	resp, err := c.GetLatestRoomUserEventsByType(t.Context(), 42, 99, "member_nickname_updated", 5)
	if err != nil {
		t.Fatalf("GetLatestRoomUserEventsByType() error = %v", err)
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
	if resp[0].ID != 9 {
		t.Errorf("ID = %d, want 9", resp[0].ID)
	}
}

func TestH2CClientGetRoomUserEventsBeforeSendsDescCursor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		before     int64
		wantBefore string
	}{
		{name: "older page", before: 43, wantBefore: "43"},
		{name: "latest page", before: 0, wantBefore: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("limit"); got != "5" {
					t.Errorf("limit = %s, want 5", got)
				}
				if got := r.URL.Query().Get("before"); got != test.wantBefore {
					t.Errorf("before = %s, want %s", got, test.wantBefore)
				}
				if got := r.URL.Query().Get("after"); got != "" {
					t.Errorf("after = %s, want empty", got)
				}
				if got := r.URL.Query().Get("order"); got != "desc" {
					t.Errorf("order = %s, want desc", got)
				}
				if got := r.URL.Query().Get("userId"); got != "99" {
					t.Errorf("userId = %s, want 99", got)
				}

				if err := json.NewEncoder(w).Encode([]RoomEventRecord{}); err != nil {
					t.Fatalf("encode response: %v", err)
				}
			}))
			defer srv.Close()

			client := NewH2CClient(srv.URL, "", WithHTTPClient(srv.Client()))
			if _, err := client.GetRoomUserEventsBefore(t.Context(), 42, 99, 5, test.before); err != nil {
				t.Fatalf("GetRoomUserEventsBefore() error = %v", err)
			}
		})
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
