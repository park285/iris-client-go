package valkeydedup_test

import (
	"testing"

	"github.com/park285/iris-client-go/valkeydedup"
	"github.com/park285/iris-client-go/webhook"
)

func TestNewReturnsDeduplicator(t *testing.T) {
	t.Parallel()

	d := valkeydedup.New(nil)
	if d == nil {
		t.Fatal("New() returned nil")
	}
	//lint:ignore SA1019 소비자 drain 전 legacy interface 호환을 검증한다.
	var _ webhook.Deduplicator = d
	//lint:ignore SA1019 소비자 drain 전 legacy interface 호환을 검증한다.
	var _ webhook.DedupReleaser = d
	var _ webhook.StatefulDeduplicator = d
}

func TestOptionReturnsHandlerOption(t *testing.T) {
	t.Parallel()

	var opt webhook.HandlerOption = valkeydedup.Option(nil)
	if opt == nil {
		t.Fatal("Option() returned nil")
	}
}
