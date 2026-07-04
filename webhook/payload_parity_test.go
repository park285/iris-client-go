package webhook_test

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sort"
	"testing"

	"github.com/park285/iris-client-go/webhook"
)

type webhookPayloadVector struct {
	Name        string `json:"name"`
	PayloadJSON string `json:"payloadJson"`
}

func TestWebhookPayloadVectorsMatchStrictSchema(t *testing.T) {
	vectors := readWebhookPayloadVectors(t)
	if len(vectors) == 0 {
		t.Fatal("no webhook payload vectors")
	}

	for _, vector := range vectors {
		t.Run(vector.Name, func(t *testing.T) {
			sourceObject := decodeJSONObject(t, []byte(vector.PayloadJSON))

			req := strictDecodeWebhookRequest(t, []byte(vector.PayloadJSON))
			emittedJSON, err := json.Marshal(req)
			if err != nil {
				t.Fatalf("json.Marshal(WebhookRequest) error = %v", err)
			}
			emittedObject := decodeJSONObject(t, emittedJSON)

			assertKeySetEqual(t, sourceObject, emittedObject, "top-level payload")
			assertMentionKeySetsEqual(t, vector.Name, sourceObject, emittedObject)
		})
	}
}

func readWebhookPayloadVectors(t *testing.T) []webhookPayloadVector {
	t.Helper()

	raw, err := os.ReadFile("testdata/webhook_payload_vectors.json")
	if err != nil {
		t.Fatalf("ReadFile(webhook_payload_vectors.json) error = %v", err)
	}

	var vectors []webhookPayloadVector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("json.Unmarshal(webhook payload vectors) error = %v", err)
	}

	return vectors
}

func strictDecodeWebhookRequest(t *testing.T, raw []byte) webhook.WebhookRequest {
	t.Helper()

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()

	var req webhook.WebhookRequest
	if err := decoder.Decode(&req); err != nil {
		t.Fatalf("strict decode WebhookRequest error = %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("strict decode WebhookRequest trailing value error = %v", err)
	}

	return req
}

func decodeJSONObject(t *testing.T, raw []byte) map[string]json.RawMessage {
	t.Helper()

	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("json.Unmarshal(object) error = %v", err)
	}
	if object == nil {
		t.Fatal("JSON value is not an object")
	}

	return object
}

func assertKeySetEqual(
	t *testing.T,
	want map[string]json.RawMessage,
	got map[string]json.RawMessage,
	label string,
) {
	t.Helper()

	wantKeys := sortedJSONKeys(want)
	gotKeys := sortedJSONKeys(got)
	if !stringSlicesEqual(wantKeys, gotKeys) {
		t.Fatalf("%s keys = %v, want %v", label, gotKeys, wantKeys)
	}
}

func assertMentionKeySetsEqual(
	t *testing.T,
	vectorName string,
	sourceObject map[string]json.RawMessage,
	emittedObject map[string]json.RawMessage,
) {
	t.Helper()

	sourceRaw, sourceOK := sourceObject["mentions"]
	emittedRaw, emittedOK := emittedObject["mentions"]
	if sourceOK != emittedOK {
		t.Fatalf("%s mentions presence = %t, want %t", vectorName, emittedOK, sourceOK)
	}
	if !sourceOK {
		return
	}

	sourceMentions := decodeMentionObjects(t, sourceRaw)
	emittedMentions := decodeMentionObjects(t, emittedRaw)
	if len(emittedMentions) != len(sourceMentions) {
		t.Fatalf(
			"%s mention count = %d, want %d",
			vectorName,
			len(emittedMentions),
			len(sourceMentions),
		)
	}

	for i := range sourceMentions {
		assertKeySetEqual(t, sourceMentions[i], emittedMentions[i], vectorName)
	}
}

func decodeMentionObjects(t *testing.T, raw json.RawMessage) []map[string]json.RawMessage {
	t.Helper()

	var mentions []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &mentions); err != nil {
		t.Fatalf("json.Unmarshal(mentions) error = %v", err)
	}

	return mentions
}

func sortedJSONKeys(object map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	return keys
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}

	return true
}
