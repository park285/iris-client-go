package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestH2CClientGetRooms(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := RoomListResponse{
			Rooms: []RoomSummary{
				{ChatID: 100},
				{ChatID: 200},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetRooms(t.Context())
	if err != nil {
		t.Fatalf("GetRooms() error = %v", err)
	}

	if gotPath != PathRooms {
		t.Fatalf("path = %q, want %q", gotPath, PathRooms)
	}

	if len(result.Rooms) != 2 {
		t.Fatalf("len(Rooms) = %d, want 2", len(result.Rooms))
	}

	if result.Rooms[0].ChatID != 100 || result.Rooms[1].ChatID != 200 {
		t.Fatalf("unexpected rooms: %+v", result.Rooms)
	}
}

func TestGetRooms_TransportFailureWrapsAsTransportError(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt))
	_, err := client.GetRooms(t.Context())

	assertTransportFailure(t, err)
}

func TestH2CClientGetMembers(t *testing.T) {
	t.Parallel()

	var gotPath string

	nick := "alice"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := MemberListResponse{
			ChatID:     100,
			TotalCount: 1,
			Members: []MemberInfo{
				{UserID: 1, Nickname: &nick, Role: "member", RoleCode: 0, MessageCount: 42},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetMembers(t.Context(), 100)
	if err != nil {
		t.Fatalf("GetMembers() error = %v", err)
	}

	if gotPath != "/rooms/100/members" {
		t.Fatalf("path = %q, want /rooms/100/members", gotPath)
	}

	if result.ChatID != 100 || result.TotalCount != 1 {
		t.Fatalf("unexpected response: %+v", result)
	}

	if len(result.Members) != 1 || result.Members[0].UserID != 1 {
		t.Fatalf("unexpected members: %+v", result.Members)
	}

	if result.Members[0].Nickname == nil || *result.Members[0].Nickname != "alice" {
		t.Fatalf("Nickname = %v, want alice", result.Members[0].Nickname)
	}
}

func TestH2CClientGetMembersWithProfileRefresh(t *testing.T) {
	t.Parallel()

	var gotRequestURI string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.URL.RequestURI()
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := MemberListResponse{ChatID: 100, TotalCount: 0}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.GetMembersWithProfileRefresh(t.Context(), 100, 777)
	if err != nil {
		t.Fatalf("GetMembersWithProfileRefresh() error = %v", err)
	}

	if gotRequestURI != "/rooms/100/members?profileUserId=777" {
		t.Fatalf("request URI = %q, want /rooms/100/members?profileUserId=777", gotRequestURI)
	}
}

func TestGetRoomEvents_TransportFailureWrapsAsTransportError(t *testing.T) {
	t.Parallel()

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed")
	})

	client := NewH2CClient("http://localhost", "", WithRoundTripper(rt))
	_, err := client.GetRoomEvents(t.Context(), 42, 0, 0)

	assertTransportFailure(t, err)
}

func assertTransportFailure(t *testing.T, err error) {
	t.Helper()

	if err == nil {
		t.Fatal("error = nil, want transport error")
	}

	var te *TransportError
	if !errors.As(err, &te) {
		t.Fatalf("expected *TransportError, got %T: %v", err, err)
	}
	if !errors.Is(err, ErrTransport) {
		t.Fatalf("expected errors.Is(err, ErrTransport), got false: %v", err)
	}
	if !errors.Is(err, ErrRetryable) {
		t.Fatalf("expected errors.Is(err, ErrRetryable), got false: %v", err)
	}
}

func TestH2CClientGetRoomInfo(t *testing.T) {
	t.Parallel()

	var gotPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := RoomInfoResponse{
			ChatID:           100,
			Notices:          []NoticeInfo{{Content: "hello", AuthorID: 1, UpdatedAt: 1000}},
			BlindedMemberIDs: []int64{},
			BotCommands:      []BotCommandInfo{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetRoomInfo(t.Context(), 100)
	if err != nil {
		t.Fatalf("GetRoomInfo() error = %v", err)
	}

	if gotPath != "/rooms/100/info" {
		t.Fatalf("path = %q, want /rooms/100/info", gotPath)
	}

	if result.ChatID != 100 {
		t.Fatalf("ChatID = %d, want 100", result.ChatID)
	}

	if len(result.Notices) != 1 || result.Notices[0].Content != "hello" {
		t.Fatalf("unexpected notices: %+v", result.Notices)
	}
}

func TestH2CClientGetRoomStats(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := StatsResponse{
			ChatID:        100,
			Period:        PeriodRange{From: 1000, To: 2000},
			TotalMessages: 500,
			ActiveMembers: 10,
			TopMembers:    []MemberStats{},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetRoomStats(t.Context(), 100, RoomStatsOptions{
		Period:      "7d",
		Limit:       5,
		MinMessages: 10,
	})
	if err != nil {
		t.Fatalf("GetRoomStats() error = %v", err)
	}

	if gotPath != "/rooms/100/stats" {
		t.Fatalf("path = %q, want /rooms/100/stats", gotPath)
	}

	if !strings.Contains(gotQuery, "period=7d") {
		t.Fatalf("query = %q, want period=7d", gotQuery)
	}
	if !strings.Contains(gotQuery, "limit=5") {
		t.Fatalf("query = %q, want limit=5", gotQuery)
	}
	if !strings.Contains(gotQuery, "minMessages=10") {
		t.Fatalf("query = %q, want minMessages=10", gotQuery)
	}

	if result.ChatID != 100 || result.TotalMessages != 500 || result.ActiveMembers != 10 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestH2CClientGetRoomStatsNoOptions(t *testing.T) {
	t.Parallel()

	var gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery

		resp := StatsResponse{ChatID: 100, TopMembers: []MemberStats{}}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.GetRoomStats(t.Context(), 100, RoomStatsOptions{})
	if err != nil {
		t.Fatalf("GetRoomStats() error = %v", err)
	}

	if gotQuery != "" {
		t.Fatalf("query = %q, want empty", gotQuery)
	}
}

func TestH2CClientGetMemberActivity(t *testing.T) {
	t.Parallel()

	var gotPath, gotQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}

		resp := MemberActivityResponse{
			UserID:       1,
			MessageCount: 42,
			ActiveHours:  []int{9, 10, 14},
			MessageTypes: map[string]int{"text": 40, "image": 2},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	result, err := client.GetMemberActivity(t.Context(), 100, 1, "7d")
	if err != nil {
		t.Fatalf("GetMemberActivity() error = %v", err)
	}

	if gotPath != "/rooms/100/members/1/activity" {
		t.Fatalf("path = %q, want /rooms/100/members/1/activity", gotPath)
	}

	if gotQuery != "period=7d" {
		t.Fatalf("query = %q, want period=7d", gotQuery)
	}

	if result.UserID != 1 || result.MessageCount != 42 {
		t.Fatalf("unexpected result: %+v", result)
	}

	if len(result.ActiveHours) != 3 {
		t.Fatalf("len(ActiveHours) = %d, want 3", len(result.ActiveHours))
	}
}

func TestH2CClientGetRoomsError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "", WithTransport("http1"))
	_, err := client.GetRooms(t.Context())
	if err == nil {
		t.Fatal("expected error for 403")
	}

	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("error = %q, want 403 mention", err.Error())
	}
}
