package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/park285/iris-client-go/internal/jsonx"
)

func TestSearchNicknameHistoryExactUsesCanonicalQueryEncoding(t *testing.T) {
	t.Parallel()

	var gotRequestURI string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.URL.RequestURI()
		if err := jsonx.NewEncoder(w).Encode(NicknameHistorySearchResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	c := NewH2CClient(server.URL, "bot-token", WithTransport("http1"), WithHTTPClient(server.Client()))
	if _, err := c.SearchNicknameHistoryExact(t.Context(), 42, " 카푸치노 ", 50); err != nil {
		t.Fatalf("SearchNicknameHistoryExact() error = %v", err)
	}

	want := "/rooms/42/nickname-history/search?limit=50&match=exact&name=%EC%B9%B4%ED%91%B8%EC%B9%98%EB%85%B8"
	if gotRequestURI != want {
		t.Fatalf("RequestURI = %q, want %q", gotRequestURI, want)
	}
}

func TestNicknameHistorySearchResponseJSON(t *testing.T) {
	raw := `{
		"complete": true,
		"asOfSourceLogId": 165595,
		"durableHeadSourceLogId": 165595,
		"matches": [
			{
				"userId": 8691114094424718810,
				"latestNickname": "카푸카푸",
				"history": [
					{
						"previousDisplayName": "카푸치노",
						"currentDisplayName": "카푸카푸",
						"sourceLogId": 165595,
						"createdAtMs": 1778226335000
					}
				]
			}
		]
	}`

	var got NicknameHistorySearchResponse
	if err := jsonx.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Complete {
		t.Fatalf("Complete = %v, want true", got.Complete)
	}
	if got.AsOfSourceLogID != 165595 {
		t.Fatalf("AsOfSourceLogID = %d, want 165595", got.AsOfSourceLogID)
	}
	if got.DurableHeadSourceLogID != 165595 {
		t.Fatalf("DurableHeadSourceLogID = %d, want 165595", got.DurableHeadSourceLogID)
	}
	if len(got.Matches) != 1 {
		t.Fatalf("len(Matches) = %d, want 1", len(got.Matches))
	}

	m := got.Matches[0]
	if m.UserID != 8691114094424718810 {
		t.Fatalf("Matches[0].UserID = %d, want 8691114094424718810", m.UserID)
	}
	if m.LatestNickname != "카푸카푸" {
		t.Fatalf("Matches[0].LatestNickname = %q, want 카푸카푸", m.LatestNickname)
	}
	if len(m.History) != 1 {
		t.Fatalf("len(Matches[0].History) = %d, want 1", len(m.History))
	}

	h := m.History[0]
	if h.PreviousDisplayName != "카푸치노" {
		t.Fatalf("History[0].PreviousDisplayName = %q, want 카푸치노", h.PreviousDisplayName)
	}
	if h.CurrentDisplayName != "카푸카푸" {
		t.Fatalf("History[0].CurrentDisplayName = %q, want 카푸카푸", h.CurrentDisplayName)
	}
	if h.SourceLogID != 165595 {
		t.Fatalf("History[0].SourceLogID = %d, want 165595", h.SourceLogID)
	}
	if h.CreatedAtMs != 1778226335000 {
		t.Fatalf("History[0].CreatedAtMs = %d, want 1778226335000", h.CreatedAtMs)
	}
}
