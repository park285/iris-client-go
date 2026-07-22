package main

import (
	"net/url"
	"testing"
)

func TestDiagExporterDefaultIrisEndpointUsesLoopback(t *testing.T) {
	parsed, err := url.Parse(defaultIrisBaseURL)
	if err != nil {
		t.Fatalf("parse defaultIrisBaseURL: %v", err)
	}
	if parsed.Scheme != "https" {
		t.Fatalf("defaultIrisBaseURL scheme = %q, want https", parsed.Scheme)
	}
	if !isLoopbackListen(parsed.Host) {
		t.Fatalf("defaultIrisBaseURL host = %q, want loopback", parsed.Host)
	}
}

func TestDiagExporterIrisEndpointOverrideWins(t *testing.T) {
	const endpoint = "https://iris.example:3001"
	t.Setenv("IRIS_BASE_URL", endpoint)

	if got := envOr("IRIS_BASE_URL", defaultIrisBaseURL); got != endpoint {
		t.Fatalf("IRIS_BASE_URL = %q, want %q", got, endpoint)
	}
}

func TestDiagExporterEmptyIrisEndpointUsesPortableDefault(t *testing.T) {
	t.Setenv("IRIS_BASE_URL", "")

	if got := envOr("IRIS_BASE_URL", defaultIrisBaseURL); got != defaultIrisBaseURL {
		t.Fatalf("IRIS_BASE_URL = %q, want default %q", got, defaultIrisBaseURL)
	}
}
