package iris_test

import (
	jsonv2 "encoding/json/v2"
	"os"
	"testing"
	"time"

	"github.com/park285/iris-client-go/v6/iris"
)

func TestReplyAdmissionConstantsMatchIrisContract(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("testdata/reply_admission_retention.json")
	if err != nil {
		t.Fatalf("계약 파일 읽기 실패: %v", err)
	}

	var contract struct {
		RetentionMs int64 `json:"replyAdmissionRetentionMs"`
		MarginMs    int64 `json:"replyReplayHorizonMarginMs"`
		HorizonMs   int64 `json:"replyAutomaticReplayHorizonMs"`
	}

	if err := jsonv2.Unmarshal(data, &contract, jsonv2.RejectUnknownMembers(true)); err != nil {
		t.Fatalf("계약 파일 파싱 실패: %v", err)
	}

	for _, tc := range []struct {
		name string
		got  time.Duration
		ms   int64
	}{
		{"ReplyAdmissionRetention", iris.ReplyAdmissionRetention, contract.RetentionMs},
		{"ReplyReplayHorizonMargin", iris.ReplyReplayHorizonMargin, contract.MarginMs},
		{"ReplyAutomaticReplayHorizon", iris.ReplyAutomaticReplayHorizon, contract.HorizonMs},
	} {
		if want := time.Duration(tc.ms) * time.Millisecond; tc.got != want {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, want)
		}
	}
}
