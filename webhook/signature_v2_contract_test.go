package webhook

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/park285/iris-client-go/internal/irishmac"
)

type webhookSignatureV2Vector struct {
	Name             string `json:"name"`
	SignatureVersion string `json:"signatureVersion"`
	Secret           string `json:"secret"`
	Method           string `json:"method"`
	Target           string `json:"target"`
	TimestampMS      string `json:"timestampMs"`
	Nonce            string `json:"nonce"`
	MessageID        string `json:"messageId"`
	Body             string `json:"body"`
	BodySHA256Hex    string `json:"bodySha256Hex"`
	CanonicalRequest string `json:"canonicalRequest"`
	Signature        string `json:"signature"`
}

func TestWebhookSignatureV2ContractVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/webhook_signature_v2_vectors.json")
	if err != nil {
		t.Fatalf("read v2 vectors: %v", err)
	}
	var vectors []webhookSignatureV2Vector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("decode v2 vectors: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatal("v2 vectors are empty")
	}

	for _, vector := range vectors {
		if vector.SignatureVersion != SignatureVersionV2 {
			t.Fatalf("%s signature version = %q, want %q", vector.Name, vector.SignatureVersion, SignatureVersionV2)
		}
		if got := irishmac.SHA256HexBytes([]byte(vector.Body)); got != vector.BodySHA256Hex {
			t.Fatalf("%s body hash = %q, want %q", vector.Name, got, vector.BodySHA256Hex)
		}
		messageID, valid := normalizeMessageID(vector.MessageID)
		if !valid || messageID != vector.MessageID {
			t.Fatalf("%s message ID = %q, want canonical", vector.Name, vector.MessageID)
		}
		canonical := canonicalWebhookRequestV2(
			vector.Method,
			vector.Target,
			vector.TimestampMS,
			vector.Nonce,
			vector.MessageID,
			vector.BodySHA256Hex,
		)
		if canonical != vector.CanonicalRequest {
			t.Fatalf("%s canonical request = %q, want %q", vector.Name, canonical, vector.CanonicalRequest)
		}
		signer := irishmac.NewSigner(vector.Secret)
		if signature := signer.Sign(canonical); signature != vector.Signature {
			t.Fatalf("%s signature = %q, want %q", vector.Name, signature, vector.Signature)
		}
		mutated := canonicalWebhookRequestV2(
			vector.Method,
			vector.Target,
			vector.TimestampMS,
			vector.Nonce,
			vector.MessageID+"-mutated",
			vector.BodySHA256Hex,
		)
		if signer.Sign(mutated) == vector.Signature {
			t.Fatalf("%s message ID mutation preserved signature", vector.Name)
		}
	}
}
