package webhook

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/irishmac"
)

type webhookSignatureV3Vector struct {
	Name             string `json:"name"`
	SignatureVersion string `json:"signatureVersion"`
	Authority        string `json:"authority"`
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

func TestWebhookSignatureV3ContractVectors(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("testdata/webhook_signature_v3_vectors.json")
	if err != nil {
		t.Fatalf("read v3 vectors: %v", err)
	}
	var vectors []webhookSignatureV3Vector
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("decode v3 vectors: %v", err)
	}
	if len(vectors) == 0 {
		t.Fatal("v3 vectors are empty")
	}

	for _, vector := range vectors {
		if vector.SignatureVersion != SignatureVersionV3 {
			t.Fatalf("%s signature version = %q, want %q", vector.Name, vector.SignatureVersion, SignatureVersionV3)
		}
		if got := irishmac.SHA256HexBytes([]byte(vector.Body)); got != vector.BodySHA256Hex {
			t.Fatalf("%s body hash = %q, want %q", vector.Name, got, vector.BodySHA256Hex)
		}
		canonical, err := irishmac.CanonicalWebhookRequestV3(
			vector.Authority,
			vector.Method,
			vector.Target,
			vector.TimestampMS,
			vector.Nonce,
			vector.MessageID,
			vector.BodySHA256Hex,
		)
		if err != nil {
			t.Fatalf("%s canonical request error = %v", vector.Name, err)
		}
		if canonical != vector.CanonicalRequest {
			t.Fatalf("%s canonical request = %q, want %q", vector.Name, canonical, vector.CanonicalRequest)
		}
		signer := irishmac.NewSigner(vector.Secret)
		if signature := signer.Sign(canonical); signature != vector.Signature {
			t.Fatalf("%s signature = %q, want %q", vector.Name, signature, vector.Signature)
		}
		mutated, err := irishmac.CanonicalWebhookRequestV3(
			"other.example:8443",
			vector.Method,
			vector.Target,
			vector.TimestampMS,
			vector.Nonce,
			vector.MessageID,
			vector.BodySHA256Hex,
		)
		if err != nil {
			t.Fatalf("%s mutated canonical request error = %v", vector.Name, err)
		}
		if signer.Sign(mutated) == vector.Signature {
			t.Fatalf("%s authority mutation preserved signature", vector.Name)
		}
	}
}
