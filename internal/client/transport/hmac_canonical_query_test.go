package transport

import (
	"net/http"
	"testing"
)

func TestCanonicalIrisTargetTreatsPlusAsLiteralPlus(t *testing.T) {
	t.Parallel()

	got := mustCanonicalIrisTarget(t, "/query?term=a+b")
	if got != "/query?term=a%2Bb" {
		t.Fatalf("canonicalIrisTarget() = %q", got)
	}
}

func TestCanonicalIrisTargetDistinguishesFlagAndEmptyValue(t *testing.T) {
	t.Parallel()

	got := mustCanonicalIrisTarget(t, "/query?flag=&flag")
	if got != "/query?flag&flag=" {
		t.Fatalf("canonicalIrisTarget() = %q", got)
	}
}

func TestCanonicalIrisTargetReencodesUTF8AndSorts(t *testing.T) {
	t.Parallel()

	got := mustCanonicalIrisTarget(t, "/query?symbols=a%26b%3Dc%25&room%20name=%ED%95%9C%EA%B8%80%20%EC%B1%84%ED%8C%85")
	want := "/query?room%20name=%ED%95%9C%EA%B8%80%20%EC%B1%84%ED%8C%85&symbols=a%26b%3Dc%25"
	if got != want {
		t.Fatalf("canonicalIrisTarget() = %q, want %q", got, want)
	}
}

func TestCanonicalIrisTargetRejectsMalformedPercentEncoding(t *testing.T) {
	t.Parallel()

	for _, target := range []string{"/query?term=%", "/query?term=%GG"} {
		if _, err := canonicalIrisTarget(target); err == nil {
			t.Fatalf("canonicalIrisTarget(%q) error = nil, want fail-closed error", target)
		}
	}
}

func TestNewRequestFailsClosedOnMalformedTargetQuery(t *testing.T) {
	t.Parallel()

	c := NewH2CClient("http://localhost", "",
		WithTransport("http1"),
		WithHMACSecret("secret"),
	)

	if _, err := c.newSignedRequest(t.Context(), http.MethodGet, "/query?term=%", nil, SecretRoleBotControl); err == nil {
		t.Fatal("newSignedRequest() error = nil, want fail-closed error for malformed target query")
	}
}
