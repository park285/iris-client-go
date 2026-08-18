package webhooksign

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/park285/iris-client-go/v2/internal/irishmac"
)

func TestSignRequestMatchesWebhookV3Contract(t *testing.T) {
	body := []byte(`{"messageId":"kakao-log-g7-123456-default","text":"hello","room":"room-1","userId":"user-1"}`)
	req, err := http.NewRequest(http.MethodPost, "https://Webhook.Example:08443/webhook/iris", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Host = "webhook.example:8443"
	req.Header.Set(irishmac.HeaderIrisMessageID, "kakao-log-g7-123456-default")

	if err := signRequest(req, "webhook-secret", body, "9003", "webhook-v3-n1"); err != nil {
		t.Fatalf("signRequest() error = %v", err)
	}

	want := map[string]string{
		irishmac.HeaderIrisSignatureVersion: irishmac.SignatureVersionV3,
		irishmac.HeaderIrisTimestamp:        "9003",
		irishmac.HeaderIrisNonce:            "webhook-v3-n1",
		irishmac.HeaderIrisBodySHA256:       "996ab617569cab40a0826be05713794c853df741efd0813b6a61a95c77698404",
		irishmac.HeaderIrisSignature:        "13ef0a9d37b8230cf385f54189ce835191d73e9fb8e0e4f228418e080c92a5d9",
	}
	for name, value := range want {
		if got := req.Header.Get(name); got != value {
			t.Fatalf("%s = %q, want %q", name, got, value)
		}
	}
}

func TestSignRequestRequiresCanonicalHostURLAuthorityParity(t *testing.T) {
	for _, test := range []struct {
		name    string
		url     string
		host    string
		wantErr string
	}{
		{name: "URL authority", url: "https://webhook.example:8443/webhook", host: ""},
		{name: "DNS case and port normalization", url: "https://Webhook.Example:08443/webhook", host: "webhook.example:8443"},
		{name: "bracketed IPv6", url: "https://[2001:0db8::1]:08443/webhook", host: "[2001:db8::1]:8443"},
		{name: "authority mismatch", url: "https://webhook.example/webhook", host: "other.example", wantErr: "does not match"},
		{name: "Host whitespace", url: "https://webhook.example/webhook", host: " webhook.example", wantErr: "surrounding whitespace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, test.url, nil)
			if err != nil {
				t.Fatalf("NewRequest() error = %v", err)
			}
			req.Host = test.host
			req.Header.Set(irishmac.HeaderIrisMessageID, "message-123")

			err = signRequest(req, "webhook-secret", nil, "9003", "nonce-v3")
			if test.wantErr == "" && err != nil {
				t.Fatalf("signRequest() error = %v", err)
			}
			if test.wantErr != "" && (err == nil || !strings.Contains(err.Error(), test.wantErr)) {
				t.Fatalf("signRequest() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
