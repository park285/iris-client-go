package webhook

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

func FuzzMessageIDHeader(f *testing.F) {
	grammar := regexp.MustCompile(`^[-A-Za-z0-9_.:/]{0,256}$`)

	for _, raw := range []string{"", " \t", "\u2003room:42/message_8.1-3\t", strings.Repeat("a", 256), strings.Repeat("a", 257), "a,b", "a\r\nb", "\xff"} {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		header := make(http.Header)
		header.Set(HeaderIrisMessageID, raw)

		want := strings.TrimSpace(raw)
		valid := grammar.MatchString(want)

		if !valid {
			want = ""
		}

		got, present, ok := normalizedMessageIDHeader(header)
		if got != want || present != (want != "") || ok != valid {
			t.Fatalf("normalized ID = (%q, %t, %t), want (%q, %t, %t)", got, present, ok, want, want != "", valid)
		}

		header.Add(HeaderIrisMessageID, raw)

		if got, present, ok := normalizedMessageIDHeader(header); got != "" || present || ok {
			t.Fatalf("duplicate ID = (%q, %t, %t), want rejection", got, present, ok)
		}
	})
}
