package webhook

import (
	"net/http"
	"strings"
	"testing"
)

func FuzzSignatureVersionHeader(f *testing.F) {
	for _, raw := range []string{"v3", "V3", "v2", "", " v3", "v3\t", "v3,v3", "v3\x00"} {
		f.Add(raw)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		header := make(http.Header)
		header.Set(HeaderIrisSignatureVersion, raw)

		version, status := webhookSignatureVersion(header)
		wantVersion, wantStatus := "", signatureVersionUnknown

		switch {
		case raw == "v3" || raw == "V3":
			wantVersion, wantStatus = "v3", signatureVersionAccepted
		case strings.TrimSpace(raw) != raw || raw == "":
			wantStatus = signatureVersionMalformed
		}

		if version != wantVersion || status != wantStatus {
			t.Fatalf("single version = (%q, %d), want (%q, %d)", version, status, wantVersion, wantStatus)
		}

		header.Add(HeaderIrisSignatureVersion, "v3")

		if version, status := webhookSignatureVersion(header); version != "" || status != signatureVersionMalformed {
			t.Fatalf("duplicate version = (%q, %d), want malformed", version, status)
		}
	})
}
