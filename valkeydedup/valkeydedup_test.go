package valkeydedup_test

import (
	"testing"

	"github.com/park285/iris-client-go/valkeydedup"
	"github.com/park285/iris-client-go/webhook"
)

func TestNewReturnsDeduplicator(t *testing.T) {
	t.Parallel()

	var d webhook.Deduplicator = valkeydedup.New(nil)
	if d == nil {
		t.Fatal("New() returned nil")
	}
}

func TestOptionReturnsHandlerOption(t *testing.T) {
	t.Parallel()

	var opt webhook.HandlerOption = valkeydedup.Option(nil)
	if opt == nil {
		t.Fatal("Option() returned nil")
	}
}
