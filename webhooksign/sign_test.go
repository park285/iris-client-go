package webhooksign

import (
	"net/http"
	"testing"

	"github.com/park285/iris-client-go/internal/irishmac"
)

func TestSignRequestMatchesWebhookV2Contract(t *testing.T) {
	body := []byte(`{"messageId":"kakao-log-g7-123456-default","text":"hello","room":"room-1","userId":"user-1"}`)
	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set(irishmac.HeaderIrisMessageID, "kakao-log-g7-123456-default")

	if err := signRequest(req, "webhook-secret", body, "9003", "webhook-v2-n1"); err != nil {
		t.Fatalf("signRequest() error = %v", err)
	}

	want := map[string]string{
		irishmac.HeaderIrisSignatureVersion: irishmac.SignatureVersionV2,
		irishmac.HeaderIrisTimestamp:        "9003",
		irishmac.HeaderIrisNonce:            "webhook-v2-n1",
		irishmac.HeaderIrisBodySHA256:       "996ab617569cab40a0826be05713794c853df741efd0813b6a61a95c77698404",
		irishmac.HeaderIrisSignature:        "563ed7dbb16c0044d1d3bd529e9c5bb4f8f0779ceb0a2457edc3da503762e3fb",
	}
	for name, value := range want {
		if got := req.Header.Get(name); got != value {
			t.Fatalf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestSignRequestRejectsMissingMessageID(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if err := SignRequest(req, "webhook-secret", nil); err == nil {
		t.Fatal("SignRequest() error = nil, want missing message ID error")
	}
}

func TestSignRequestProducesValidWebhookV2Signature(t *testing.T) {
	body := []byte(`{"messageId":"message-123","text":"hello"}`)
	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris?z=last&a=first", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	messageID := "message-123"
	secret := "webhook-secret"
	req.Header.Set(irishmac.HeaderIrisMessageID, messageID)

	if err := SignRequest(req, secret, body); err != nil {
		t.Fatalf("SignRequest() error = %v", err)
	}

	wantHeaders := []string{
		irishmac.HeaderIrisSignatureVersion,
		irishmac.HeaderIrisTimestamp,
		irishmac.HeaderIrisNonce,
		irishmac.HeaderIrisBodySHA256,
		irishmac.HeaderIrisSignature,
	}
	for _, name := range wantHeaders {
		if got := req.Header.Get(name); got == "" {
			t.Fatalf("%s is empty", name)
		}
	}
	if got := req.Header.Get(irishmac.HeaderIrisSignatureVersion); got != irishmac.SignatureVersionV2 {
		t.Fatalf("%s = %q, want %q", irishmac.HeaderIrisSignatureVersion, got, irishmac.SignatureVersionV2)
	}
	bodySHA256 := irishmac.SHA256HexBytes(body)
	if got := req.Header.Get(irishmac.HeaderIrisBodySHA256); got != bodySHA256 {
		t.Fatalf("%s = %q, want %q", irishmac.HeaderIrisBodySHA256, got, bodySHA256)
	}
	target, err := irishmac.CanonicalTarget(req.URL.RequestURI())
	if err != nil {
		t.Fatalf("CanonicalTarget() error = %v", err)
	}
	canonical := irishmac.CanonicalWebhookRequestV2(
		req.Method,
		target,
		req.Header.Get(irishmac.HeaderIrisTimestamp),
		req.Header.Get(irishmac.HeaderIrisNonce),
		messageID,
		bodySHA256,
	)
	wantSignature := irishmac.NewSigner(secret).Sign(canonical)
	if got := req.Header.Get(irishmac.HeaderIrisSignature); got != wantSignature {
		t.Fatalf("%s = %q, want valid signature %q", irishmac.HeaderIrisSignature, got, wantSignature)
	}
}
