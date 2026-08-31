package iris_test

import (
	"testing"

	"github.com/park285/iris-client-go/v2/iris"
)

func TestParseBaseEndpointExposesCanonicalParser(t *testing.T) {
	t.Parallel()

	endpoint, err := iris.ParseBaseEndpoint("https://iris.example/deployment///")
	if err != nil {
		t.Fatalf("ParseBaseEndpoint() error = %v", err)
	}

	if got, want := endpoint.String(), "https://iris.example/deployment"; got != want {
		t.Fatalf("ParseBaseEndpoint() = %q, want %q", got, want)
	}
}
