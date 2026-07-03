package iris_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/park285/iris-client-go/iris"
)

func TestNicknameHistorySearchReExportedTypes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := iris.NicknameHistorySearchResponse{
			Complete:               true,
			AsOfSourceLogID:        165595,
			DurableHeadSourceLogID: 165595,
			Matches: []iris.NicknameHistorySearchMatch{
				{
					UserID:         8691114094424718810,
					LatestNickname: "카푸카푸",
					History: []iris.NicknameHistoryEntry{
						{
							PreviousDisplayName: "카푸치노",
							CurrentDisplayName:  "카푸카푸",
							SourceLogID:         165595,
							CreatedAtMs:         1778226335000,
						},
					},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := iris.NewH2CClient(
		server.URL, "bot-token", iris.WithTransport("http1"), iris.WithHTTPClient(server.Client()),
	)

	got, err := client.SearchNicknameHistoryExact(context.Background(), 42, "카푸치노", 50)
	if err != nil {
		t.Fatalf("SearchNicknameHistoryExact() error = %v", err)
	}
	if !got.Complete || len(got.Matches) != 1 {
		t.Fatalf("response = %+v, unexpected", got)
	}
	if got.Matches[0].UserID != 8691114094424718810 {
		t.Fatalf("Matches[0].UserID = %d, unexpected", got.Matches[0].UserID)
	}
	if got.Matches[0].History[0].PreviousDisplayName != "카푸치노" {
		t.Fatalf("History[0].PreviousDisplayName = %q, unexpected", got.Matches[0].History[0].PreviousDisplayName)
	}
}
