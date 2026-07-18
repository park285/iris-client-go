package transport

import (
	"net/url"
	"strings"
	"testing"
)

func TestAppendSafePathSegmentAcceptsTrimmedToken(t *testing.T) {
	t.Parallel()

	got, err := appendSafePathSegment(PathReplyStatus, "request ID", " reply-123_abc.def:456 ")
	if err != nil {
		t.Fatalf("appendSafePathSegment() error = %v, want nil", err)
	}
	if got != "/reply-status/reply-123_abc.def:456" {
		t.Fatalf("appendSafePathSegment() = %q", got)
	}
}

func TestAppendSafePathSegmentRejectsStructuralCharacters(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"", " ", ".", "..", "reply/123", "reply?x=1", "한글", strings.Repeat("a", maxPathSegmentTokenBytes+1)} {
		if _, err := appendSafePathSegment(PathReplyStatus, "request ID", value); err == nil {
			t.Fatalf("appendSafePathSegment(%q) error = nil, want error", value)
		}
	}
}

func TestCanonicalQueryStringMatchesIrisRuntimeContract(t *testing.T) {
	t.Parallel()

	params := url.Values{}
	params.Add("symbols", "a&b=c%")
	params.Add("room name", "한글 채팅")
	params.Add("term", "a+b")

	got := canonicalQueryString(params)
	want := "room%20name=%ED%95%9C%EA%B8%80%20%EC%B1%84%ED%8C%85&symbols=a%26b%3Dc%25&term=a%2Bb"
	if got != want {
		t.Fatalf("canonicalQueryString() = %q, want %q", got, want)
	}
}

func TestCanonicalQueryStringSupportsFlagParameter(t *testing.T) {
	t.Parallel()

	params := url.Values{"flag": nil}
	if got := canonicalQueryString(params); got != "flag" {
		t.Fatalf("canonicalQueryString(flag) = %q", got)
	}
}
