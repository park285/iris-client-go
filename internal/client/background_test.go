package client

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestRunProtectedRecoversPanic(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	runProtected(logger, "test_background_panic_recovered", func() {
		panic("sensitive panic payload")
	})

	logLine := output.String()
	for _, token := range []string{`"msg":"test_background_panic_recovered"`, `"panic_type":"string"`, `"stack":`} {
		if !strings.Contains(logLine, token) {
			t.Fatalf("recovery log missing %s: %s", token, logLine)
		}
	}
	if strings.Contains(logLine, "sensitive panic payload") {
		t.Fatalf("recovery log exposed panic payload: %s", logLine)
	}
}
