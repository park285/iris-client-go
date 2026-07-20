package iris_test

import (
	"net"
	"testing"
	"time"

	iris "github.com/park285/iris-client-go/iris"
)

func TestFacadeExposesH3DialGuardForBaseURL(t *testing.T) {
	t.Parallel()

	guard, err := iris.NewH3DialGuardForBaseURL(t.Context(), "https://192.0.2.90:31001")
	if err != nil {
		t.Fatalf("NewH3DialGuardForBaseURL() error = %v", err)
	}
	if err := guard(t.Context(), net.ParseIP("192.0.2.90")); err != nil {
		t.Fatalf("guard(allowed IP) error = %v", err)
	}
	opt, err := iris.WithH3DialGuardForBaseURL(
		t.Context(),
		"https://192.0.2.90:31001",
		iris.WithH3DialGuardTTL(time.Minute),
		iris.WithH3DialGuardResolveTimeout(5*time.Second),
		iris.WithH3DialGuardLenientInit(),
		iris.WithH3DialGuardLogger(nil),
	)
	if err != nil {
		t.Fatalf("WithH3DialGuardForBaseURL() error = %v", err)
	}
	if opt == nil {
		t.Fatal("WithH3DialGuardForBaseURL() option = nil")
	}
}
