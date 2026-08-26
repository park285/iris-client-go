package webhook_test

import (
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"os"
	"slices"
	"testing"

	"github.com/park285/iris-client-go/v2/webhook"
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

			emittedJSON, err := jsonv2.Marshal(req)
			if err != nil {
				t.Fatalf("jsonv2.Marshal(WebhookRequest) error = %v", err)
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

	if err := jsonv2.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("jsonv2.Unmarshal(webhook payload vectors) error = %v", err)
	}

	return vectors
}

func strictDecodeWebhookRequest(t *testing.T, raw []byte) webhook.WebhookRequest {
	t.Helper()

	var req webhook.WebhookRequest

	if err := jsonv2.Unmarshal(raw, &req, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("strict decode WebhookRequest error = %v", err)
	}

	return req
}

func decodeJSONObject(t *testing.T, raw []byte) map[string]jsontext.Value {
	t.Helper()

	var object map[string]jsontext.Value

	if err := jsonv2.Unmarshal(raw, &object); err != nil {
		t.Fatalf("jsonv2.Unmarshal(object) error = %v", err)
	}

	if object == nil {
		t.Fatal("JSON value is not an object")
	}

	return object
}

func assertKeySetEqual(
	t *testing.T,
	want map[string]jsontext.Value,
	got map[string]jsontext.Value,
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
	sourceObject map[string]jsontext.Value,
	emittedObject map[string]jsontext.Value,
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

func decodeMentionObjects(t *testing.T, raw jsontext.Value) []map[string]jsontext.Value {
	t.Helper()

	var mentions []map[string]jsontext.Value

	if err := jsonv2.Unmarshal(raw, &mentions); err != nil {
		t.Fatalf("jsonv2.Unmarshal(mentions) error = %v", err)
	}

	return mentions
}

func sortedJSONKeys(object map[string]jsontext.Value) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}

	slices.Sort(keys)

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
