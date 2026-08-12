package iris_test

import (
	"testing"

	"github.com/park285/iris-client-go/iris"
)

func TestMediaPublicAliases(t *testing.T) {
	var _ iris.MediaClient = (*iris.H2CClient)(nil)
	var _ iris.MediaClient = (*iris.RebindingClient)(nil)

	request := iris.MediaChunkRequest{
		MessageID:          "message-1",
		SourceGenerationID: 1,
		RawSourceLogID:     2,
		SourceLogID:        3,
		ChatID:             "4",
		ChatLogID:          "5",
		Type:               "2",
		MediaIndex:         0,
		Offset:             0,
		Length:             1,
	}
	if request.ChatID != "4" || request.ChatLogID != "5" {
		t.Fatalf("request aliases lost string IDs: %+v", request)
	}
	if iris.PathMediaChunk != "/media/chunk" {
		t.Fatalf("PathMediaChunk = %q", iris.PathMediaChunk)
	}
}
