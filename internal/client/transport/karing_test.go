package transport

import (
	jsonv2 "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
)

type capturedKaringRequest struct {
	method      string
	path        string
	signature   string
	bodyHash    string
	contentType string
}

func newKaringCaptureServer(t *testing.T, got any, response *KaringDryRunResponse) (*httptest.Server, *capturedKaringRequest) {
	t.Helper()

	captured := &capturedKaringRequest{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*captured = capturedKaringRequest{
			method:      r.Method,
			path:        r.URL.Path,
			signature:   r.Header.Get(HeaderIrisSignature),
			bodyHash:    r.Header.Get(HeaderIrisBodySHA256),
			contentType: r.Header.Get("Content-Type"),
		}

		if err := jsonv2.UnmarshalRead(r.Body, got); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		if err := jsonv2.MarshalWrite(w, response); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))

	return server, captured
}

func assertKaringBotControlPost(t *testing.T, got *capturedKaringRequest, wantPath string) {
	t.Helper()

	if got.method != http.MethodPost {
		t.Fatalf("method = %s, want POST", got.method)
	}

	if got.path != wantPath {
		t.Fatalf("path = %q, want %q", got.path, wantPath)
	}

	if got.signature == "" {
		t.Fatal("signature header missing")
	}

	if got.bodyHash == "" {
		t.Fatal("body hash header missing")
	}

	if got.contentType != contentTypeJSON {
		t.Fatalf("Content-Type = %q, want application/json", got.contentType)
	}
}

func TestKaringClientSendContentListPostsSignedBotControlRequest(t *testing.T) {
	t.Parallel()

	var got KaringContentListRequest

	clientRequestID := "karing:content-list-42:v1"

	server, captured := newKaringCaptureServer(t, &got, &KaringDryRunResponse{
		OK:           true,
		DryRun:       false,
		ReceiverName: testKaringRoomName,
		TemplateID:   133218,
		ItemCount:    new(1),
		TemplateArgs: KaringTemplateArgs{"item1_title": "테스트 방송"},
	})
	defer server.Close()

	client := NewAPIClient(server.URL, "unused-bot-token",
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
		ReceiverName:   testKaringRoomName,
		ReceiverRoomID: 464252100463241,
		TemplateID:     133218,
		ExtraArgs:      KaringTemplateArgs{"batch_id": "alarm-1"},
	})
	if err != nil {
		t.Fatalf("SendKaringContentList() error = %v", err)
	}

	assertKaringBotControlPost(t, captured, PathKaringContentList)

	if got.ClientRequestID == nil || *got.ClientRequestID != clientRequestID {
		t.Fatalf("ClientRequestID = %v, want %q", got.ClientRequestID, clientRequestID)
	}

	if got.TemplateID != 133218 || got.ReceiverName != testKaringRoomName || got.ReceiverRoomID != 464252100463241 {
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

func TestKaringClientDecodesAcceptedResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := jsonv2.MarshalWrite(w, KaringDryRunResponse{
			Success:   true,
			Delivery:  testDeliveryQueued,
			RequestID: "karing-req-1",
			Kind:      "karing.content_list",
		}); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewAPIClient(server.URL, "unused-bot-token", WithHTTPClient(server.Client()))

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
		"duplicate": true
	}`

	var got KaringDryRunResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.Success || got.Delivery != testDeliveryQueued || got.RequestID != "karing-req-2" || got.Kind != "karing.send" {
		t.Fatalf("accepted core fields = %+v", got)
	}

	if got.ReceiverName != testKaringRoomName {
		t.Fatalf("ReceiverName = %q, want 기본방", got.ReceiverName)
	}

	if got.TemplateID != 133218 {
		t.Fatalf("TemplateID = %d, want 133218", got.TemplateID)
	}

	if got.ItemCount == nil || *got.ItemCount != 2 {
		t.Fatalf("ItemCount = %v, want 2", got.ItemCount)
	}

	if got.Duplicate == nil || !*got.Duplicate {
		t.Fatalf("Duplicate = %v, want true", got.Duplicate)
	}
}

func TestKaringDryRunResponseUnmarshalCurrentCamelCaseDryRunWire(t *testing.T) {
	t.Parallel()

	raw := `{
		"ok": true,
		"dryRun": true,
		"receiverName": "기본방",
		"templateId": 133220,
		"itemCount": 2,
		"templateArgs": {"item2_title": "현재 casing"}
	}`

	var got KaringDryRunResponse

	if err := jsonv2.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if !got.OK || !got.DryRun {
		t.Fatalf("dry-run core fields = %+v", got)
	}

	if got.ReceiverName != testKaringRoomName || got.TemplateID != 133220 {
		t.Fatalf("identity fields = %+v", got)
	}

	if got.ItemCount == nil || *got.ItemCount != 2 {
		t.Fatalf("ItemCount = %v, want 2", got.ItemCount)
	}

	if got.TemplateArgs["item2_title"] != "현재 casing" {
		t.Fatalf("TemplateArgs = %v", got.TemplateArgs)
	}
}

func TestKaringRequestsEncodeCanonicalClientRequestIDKey(t *testing.T) {
	t.Parallel()

	clientRequestID := "karing:wire-key:v1"
	cases := []struct {
		name string
		req  any
	}{
		{name: "send", req: KaringSendRequest{ClientRequestID: &clientRequestID}},
		{name: "content-list", req: KaringContentListRequest{ClientRequestID: &clientRequestID}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := jsonv2.Marshal(tc.req)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			var fields map[string]any

			if err := jsonv2.Unmarshal(raw, &fields); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if fields["client_request_id"] != clientRequestID {
				t.Fatalf("client_request_id = %v in %s", fields["client_request_id"], raw)
			}

			if _, ok := fields["clientRequestId"]; ok {
				t.Fatalf("legacy clientRequestId key present in %s", raw)
			}
		})
	}
}

func TestKaringResponseRejectsRetiredKeys(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"dry_run", "receiver_name", "template_id", "item_count", "stream_count", "streamCount", "template_args"} {
		t.Run(key, func(t *testing.T) {
			t.Parallel()

			var got KaringDryRunResponse

			raw := []byte(`{"ok":true,"` + key + `":null}`)

			if err := jsonv2.Unmarshal(raw, &got); err == nil {
				t.Fatalf("retired Karing key %q accepted", key)
			}
		})
	}
}

func TestKaringResponseCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	want := KaringDryRunResponse{OK: true, DryRun: true, ReceiverName: "room", TemplateID: 42, ItemCount: new(2), TemplateArgs: KaringTemplateArgs{"title": "video"}}

	raw, err := jsonv2.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	var got KaringDryRunResponse

	if err := jsonv2.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}

	if !got.OK || !got.DryRun || got.ReceiverName != want.ReceiverName || got.TemplateID != want.TemplateID || got.ItemCount == nil || *got.ItemCount != 2 || got.TemplateArgs["title"] != "video" {
		t.Fatalf("round trip = %+v", got)
	}
}
