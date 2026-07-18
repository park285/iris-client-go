package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKaringClientSendContentListPostsSignedBotControlRequest(t *testing.T) {
	t.Parallel()

	var got KaringContentListRequest
	var gotPath string
	var gotMethod string
	var gotSignature string
	var gotBodyHash string
	var gotContentType string
	clientRequestID := "karing:content-list-42:v1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotMethod = r.Method
		gotSignature = r.Header.Get(HeaderIrisSignature)
		gotBodyHash = r.Header.Get(HeaderIrisBodySHA256)
		gotContentType = r.Header.Get("Content-Type")

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if err := json.NewEncoder(w).Encode(KaringDryRunResponse{
			OK:           true,
			DryRun:       false,
			ReceiverName: "기본방",
			TemplateID:   133218,
			ItemCount:    intPtr(1),
			TemplateArgs: KaringTemplateArgs{"item1_title": "테스트 방송"},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-bot-token",
		WithBotControlToken("bot-control-secret"),
		WithHTTPClient(server.Client()),
	)

	resp, err := client.SendKaringContentList(t.Context(), KaringContentListRequest{
		ClientRequestID: &clientRequestID,
		Items: []KaringContentItem{{
			Title:        "테스트 방송",
			URL:          "https://www.youtube.com/watch?v=video000001",
			MemberName:   "Test Member",
			ChannelName:  "Test Channel",
			Status:       KaringStreamStatusLive,
			StartAt:      "2026-05-16T12:00:00Z",
			ThumbnailURL: "https://i.ytimg.com/vi/video000001/maxresdefault.jpg",
			Platform:     "youtube",
		}},
		ReceiverName:   "기본방",
		ReceiverRoomID: 464252100463241,
		TemplateID:     133218,
		ExtraArgs:      KaringTemplateArgs{"batch_id": "alarm-1"},
	})
	if err != nil {
		t.Fatalf("SendKaringContentList() error = %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Fatalf("method = %s, want POST", gotMethod)
	}
	if gotPath != PathKaringContentList {
		t.Fatalf("path = %q, want %q", gotPath, PathKaringContentList)
	}
	if gotSignature == "" {
		t.Fatal("signature header missing")
	}
	if gotBodyHash == "" {
		t.Fatal("body hash header missing")
	}
	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", gotContentType)
	}
	if got.ClientRequestID == nil || *got.ClientRequestID != clientRequestID {
		t.Fatalf("ClientRequestID = %v, want %q", got.ClientRequestID, clientRequestID)
	}
	if got.TemplateID != 133218 || got.ReceiverName != "기본방" || got.ReceiverRoomID != 464252100463241 {
		t.Fatalf("request = %+v, want template and receiver", got)
	}
	if len(got.Items) != 1 {
		t.Fatalf("len(Items) = %d, want 1", len(got.Items))
	}
	if got.Items[0].URL != "https://www.youtube.com/watch?v=video000001" {
		t.Fatalf("Items[0].URL = %q", got.Items[0].URL)
	}
	if got.ExtraArgs["batch_id"] != "alarm-1" {
		t.Fatalf("ExtraArgs[batch_id] = %q, want alarm-1", got.ExtraArgs["batch_id"])
	}
	if resp == nil || !resp.OK || resp.TemplateID != 133218 || resp.ItemCount == nil || *resp.ItemCount != 1 {
		t.Fatalf("SendKaringContentList() response = %+v", resp)
	}
}

func TestKaringClientSendHololivePostsSignedBotControlRequest(t *testing.T) {
	t.Parallel()

	var got KaringHololiveRequest
	var gotPath string
	var gotSignature string
	clientRequestID := "karing:hololive-42:v1"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotSignature = r.Header.Get(HeaderIrisSignature)

		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if err := json.NewEncoder(w).Encode(KaringDryRunResponse{
			OK:           true,
			DryRun:       true,
			ReceiverName: "기본방",
			TemplateID:   133220,
			StreamCount:  intPtr(1),
			TemplateArgs: KaringTemplateArgs{"time_left": "10분 후 시작"},
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-bot-token",
		WithBotControlToken("bot-control-secret"),
		WithHTTPClient(server.Client()),
	)

	resp, err := client.SendKaringHololive(t.Context(), KaringHololiveRequest{
		ClientRequestID: &clientRequestID,
		Streams: []KaringContentItem{{
			Title:  "테스트 방송",
			URL:    "https://www.youtube.com/watch?v=video000001",
			Status: KaringStreamStatusUpcoming,
		}},
		ExtraArgs:      KaringTemplateArgs{"time_left": "10분 후 시작"},
		ReceiverRoomID: 464252100463241,
		DryRun:         true,
	})
	if err != nil {
		t.Fatalf("SendKaringHololive() error = %v", err)
	}

	if gotPath != PathKaringHololive {
		t.Fatalf("path = %q, want %q", gotPath, PathKaringHololive)
	}
	if gotSignature == "" {
		t.Fatal("signature header missing")
	}
	if got.ClientRequestID == nil || *got.ClientRequestID != clientRequestID {
		t.Fatalf("ClientRequestID = %v, want %q", got.ClientRequestID, clientRequestID)
	}
	if len(got.Streams) != 1 || got.Streams[0].Status != KaringStreamStatusUpcoming {
		t.Fatalf("Streams = %+v", got.Streams)
	}
	if got.ReceiverRoomID != 464252100463241 {
		t.Fatalf("ReceiverRoomID = %d, want 464252100463241", got.ReceiverRoomID)
	}
	if got.ExtraArgs["time_left"] != "10분 후 시작" {
		t.Fatalf("ExtraArgs[time_left] = %q, want 10분 후 시작", got.ExtraArgs["time_left"])
	}
	if resp == nil || !resp.OK || resp.StreamCount == nil || *resp.StreamCount != 1 {
		t.Fatalf("SendKaringHololive() response = %+v", resp)
	}
}

func TestKaringClientDecodesAcceptedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := json.NewEncoder(w).Encode(KaringDryRunResponse{
			Success:   true,
			Delivery:  "queued",
			RequestID: "karing-req-1",
			Kind:      "karing.content_list",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewH2CClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))
	resp, err := client.SendKaringContentList(t.Context(), KaringContentListRequest{})
	if err != nil {
		t.Fatalf("SendKaringContentList() error = %v", err)
	}

	if resp == nil || !resp.Success || resp.RequestID != "karing-req-1" || resp.Kind != "karing.content_list" {
		t.Fatalf("SendKaringContentList() response = %+v", resp)
	}
}

func TestKaringDryRunResponseUnmarshalAcceptedCamelCaseWire(t *testing.T) {
	t.Parallel()

	raw := `{
		"success": true,
		"delivery": "queued",
		"requestId": "karing-req-2",
		"kind": "karing.send",
		"receiverName": "기본방",
		"templateId": 133218,
		"itemCount": 2,
		"streamCount": 3,
		"duplicate": true
	}`

	var got KaringDryRunResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Success || got.Delivery != "queued" || got.RequestID != "karing-req-2" || got.Kind != "karing.send" {
		t.Fatalf("accepted core fields = %+v", got)
	}
	if got.ReceiverName != "기본방" {
		t.Fatalf("ReceiverName = %q, want 기본방", got.ReceiverName)
	}
	if got.TemplateID != 133218 {
		t.Fatalf("TemplateID = %d, want 133218", got.TemplateID)
	}
	if got.ItemCount == nil || *got.ItemCount != 2 {
		t.Fatalf("ItemCount = %v, want 2", got.ItemCount)
	}
	if got.StreamCount == nil || *got.StreamCount != 3 {
		t.Fatalf("StreamCount = %v, want 3", got.StreamCount)
	}
	if got.Duplicate == nil || !*got.Duplicate {
		t.Fatalf("Duplicate = %v, want true", got.Duplicate)
	}
}

func TestKaringDryRunResponseUnmarshalSnakeCaseWire(t *testing.T) {
	t.Parallel()

	raw := `{
		"ok": true,
		"dry_run": true,
		"receiver_name": "기본방",
		"template_id": 133218,
		"item_count": 1,
		"template_args": {"item1_title": "테스트 방송"}
	}`

	var got KaringDryRunResponse
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.OK || !got.DryRun {
		t.Fatalf("dry-run core fields = %+v", got)
	}
	if got.ReceiverName != "기본방" || got.TemplateID != 133218 {
		t.Fatalf("identity fields = %+v", got)
	}
	if got.ItemCount == nil || *got.ItemCount != 1 {
		t.Fatalf("ItemCount = %v, want 1", got.ItemCount)
	}
	if got.TemplateArgs["item1_title"] != "테스트 방송" {
		t.Fatalf("TemplateArgs = %v", got.TemplateArgs)
	}
}
