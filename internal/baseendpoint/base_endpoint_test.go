package baseendpoint

import (
	"strings"
	"testing"
)

func TestParsePreservesDeploymentPrefixAndRemovesOnlyTrailingSeparators(t *testing.T) {
	t.Parallel()

	endpoint, err := Parse("  https://iris.example/tenant//bridge///  ")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if got, want := endpoint.String(), "https://iris.example/tenant//bridge"; got != want {
		t.Fatalf("Parse() = %q, want %q", got, want)
	}
}

func TestParseRejectsUnsafeOrAmbiguousShapes(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"",
		"/relative",
		"mailto:iris@example.com",
		"ftp://iris.example",
		"https:///missing-host",
		"https://user:password@iris.example",
		"https://iris.example?",
		"https://iris.example?mode=h3",
		"https://iris.example#fragment",
	} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse(raw); err == nil {
				t.Fatalf("Parse(%q) error = nil, want rejection", raw)
			}
		})
	}
}

func TestParseErrorDoesNotExposeCredentialLikeInput(t *testing.T) {
	t.Parallel()

	const sensitiveValue = "do-not-log-this-value"

	_, err := Parse("https://user:" + sensitiveValue + "@iris.example/%zz")
	if err == nil {
		t.Fatal("Parse() error = nil, want rejection")
	}

	if strings.Contains(err.Error(), sensitiveValue) {
		t.Fatalf("Parse() error exposes credential-like input: %v", err)
	}
}

func TestParseReportsInvalidPortWithoutEchoingInput(t *testing.T) {
	t.Parallel()

	const sensitiveValue = "do-not-log-this-port-value"

	_, err := Parse("https://iris.example:" + sensitiveValue)
	if err == nil {
		t.Fatal("Parse() error = nil, want rejection")
	}

	if !strings.Contains(err.Error(), "port") {
		t.Fatalf("Parse() error = %v, want port classification", err)
	}

	if strings.Contains(err.Error(), sensitiveValue) {
		t.Fatalf("Parse() error exposes input: %v", err)
	}
}
