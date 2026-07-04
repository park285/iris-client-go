package iris_test

import (
	"context"
	"net"
	"testing"

	iris "github.com/park285/iris-client-go/iris"
)

func TestFacadeExposesH3DialGuardContext(t *testing.T) {
	t.Parallel()

	var opt iris.ClientOption = iris.WithH3DialGuardContext(func(context.Context, net.IP) error {
		return nil
	})
	if opt == nil {
		t.Fatal("WithH3DialGuardContext() returned nil")
	}
}
