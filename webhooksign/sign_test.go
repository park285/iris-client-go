package webhooksign

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/internal/irishmac"
)

func TestSignRequestMatchesWebhookV2Contract(t *testing.T) {
	body := []byte(`{"messageId":"kakao-log-g7-123456-default","text":"hello","room":"room-1","userId":"user-1"}`)
	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", bytes.NewReader(body))
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
	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris?z=last&a=first", bytes.NewReader(body))
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

func TestSignRequestRejectsNonPostMethod(t *testing.T) {
	body := []byte(`{"messageId":"message-123","text":"hello"}`)

	for _, method := range []string{"", "post", http.MethodGet} {
		req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}

		req.Header.Set(irishmac.HeaderIrisMessageID, "message-123")
		req.Method = method

		if err := SignRequest(req, "webhook-secret", body); err == nil || !strings.Contains(err.Error(), "request method must be POST") {
			t.Fatalf("SignRequest(method=%q) error = %v, want the POST rejection", method, err)
		}
	}
}

func TestSignRequestMakesTheRequestCarryExactlyTheSignedBytes(t *testing.T) {
	body := []byte(`{"messageId":"message-123","text":"hello"}`)

	for _, test := range []struct {
		name   string
		mutate func(*http.Request)
	}{
		{name: "body absent", mutate: func(req *http.Request) { req.Body, req.ContentLength = nil, 0 }},
		{name: "body disagrees with the signed bytes", mutate: func(req *http.Request) {
			req.Body = io.NopCloser(bytes.NewReader([]byte("attacker-visible")))
			req.ContentLength = 0
		}},
		{name: "length unknown", mutate: func(req *http.Request) {
			req.Body = io.NopCloser(bytes.NewReader(body))
			req.ContentLength = 0
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}

			req.Header.Set(irishmac.HeaderIrisMessageID, "message-123")
			test.mutate(req)

			if err := SignRequest(req, "webhook-secret", body); err != nil {
				t.Fatalf("SignRequest() error = %v", err)
			}

			if req.ContentLength != int64(len(body)) {
				t.Fatalf("ContentLength = %d, want %d", req.ContentLength, len(body))
			}

			carried, err := io.ReadAll(req.Body)
			if err != nil {
				t.Fatalf("ReadAll(req.Body) error = %v", err)
			}

			if !bytes.Equal(carried, body) {
				t.Fatalf("request carries %q, want the signed %q", carried, body)
			}

			replay, err := req.GetBody()
			if err != nil {
				t.Fatalf("GetBody() error = %v", err)
			}

			replayed, err := io.ReadAll(replay)
			if err != nil {
				t.Fatalf("ReadAll(GetBody()) error = %v", err)
			}

			if !bytes.Equal(replayed, body) {
				t.Fatalf("GetBody() replays %q, want the signed %q", replayed, body)
			}
		})
	}
}

func TestSignRequestRejectsShadowMessageIDHeaderKeys(t *testing.T) {
	body := []byte(`{"messageId":"message-123"}`)

	req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	req.Header.Set(irishmac.HeaderIrisMessageID, "message-123")
	req.Header["x-iris-message-id"] = []string{"forged"}

	err = SignRequest(req, "webhook-secret", body)
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("SignRequest() error = %v, want the duplicate-header rejection", err)
	}
}

func TestSignRequestRejectsNonCanonicalMessageID(t *testing.T) {
	body := []byte(`{"messageId":"message-123"}`)

	for _, messageID := range []string{"mid 1", "mid#1", "메시지-1", strings.Repeat("a", irishmac.MaxMessageIDBytes+1)} {
		req, err := http.NewRequest(http.MethodPost, "https://iris.example/webhook/iris", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("NewRequest() error = %v", err)
		}

		req.Header.Set(irishmac.HeaderIrisMessageID, messageID)

		err = SignRequest(req, "webhook-secret", body)
		if err == nil || !strings.Contains(err.Error(), "non-canonical byte") {
			t.Fatalf("SignRequest(messageID=%q) error = %v, want the verifier's constraint named at sign time", messageID, err)
		}
	}
}
