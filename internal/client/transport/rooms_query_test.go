package transport

import (
	jsonv2 "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetRoomStatsUsesCanonicalQueryEncoding(t *testing.T) {
	t.Parallel()

	var gotRequestURI string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestURI = r.URL.RequestURI()

		if err := jsonv2.MarshalWrite(w, StatsResponse{}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	defer server.Close()

	client := NewAPIClient(server.URL, "bot-token", WithTransport(transportHTTP1), WithHTTPClient(server.Client()))
	if _, err := client.GetRoomStats(t.Context(), 42, RoomStatsOptions{Period: "7 days+live", Limit: 5}); err != nil {
		t.Fatalf("GetRoomStats() error = %v", err)
	}

	want := "/rooms/42/stats?limit=5&period=7%20days%2Blive"
	if gotRequestURI != want {
		t.Fatalf("RequestURI = %q, want %q", gotRequestURI, want)
	}
}
