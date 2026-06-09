package client

import "testing"

func mustCanonicalIrisTarget(tb testing.TB, target string) string {
	tb.Helper()
	got, err := canonicalIrisTarget(target)
	if err != nil {
		tb.Fatalf("canonicalIrisTarget(%q) error = %v", target, err)
	}
	return got
}

func mustSignIrisRequest(tb testing.TB, secret, method, path, timestamp, nonce, body string) string {
	tb.Helper()
	sig, err := signIrisRequest(secret, method, path, timestamp, nonce, body)
	if err != nil {
		tb.Fatalf("signIrisRequest(%q) error = %v", path, err)
	}
	return sig
}

func mustSignIrisRequestWithBodySHA256(tb testing.TB, secret, method, path, timestamp, nonce, bodySHA256 string) string {
	tb.Helper()
	sig, err := signIrisRequestWithBodySHA256(secret, method, path, timestamp, nonce, bodySHA256)
	if err != nil {
		tb.Fatalf("signIrisRequestWithBodySHA256(%q) error = %v", path, err)
	}
	return sig
}
