package iris

import "testing"

var (
	_ FileSender = (*H2CClient)(nil)
	_ FileSender = (*RebindingClient)(nil)
)

func TestNewReplyFileBytesExposesStableMetadata(t *testing.T) {
	t.Parallel()

	file := NewReplyFileBytes("report.txt", "text/plain", []byte("payload"))
	if file.FileName != "report.txt" || file.ContentType != "text/plain" || file.ByteLength != 7 {
		t.Fatalf("file = %+v", file)
	}
}
