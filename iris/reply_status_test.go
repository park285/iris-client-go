package iris_test

import (
	"testing"

	"github.com/park285/iris-client-go/v4/iris"
)

func TestClassifyReplyStatus(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		state string
		want  iris.ReplyStatusClass
	}{
		{state: "queued", want: iris.ReplyStatusPending},
		{state: "preparing", want: iris.ReplyStatusPending},
		{state: "prepared", want: iris.ReplyStatusPending},
		{state: "sending", want: iris.ReplyStatusPending},
		{state: "handoff_completed", want: iris.ReplyStatusHandoffCompleted},
		{state: "failed", want: iris.ReplyStatusFailed},
		{state: "outcome_unknown", want: iris.ReplyStatusUnknown},
		{state: "", want: iris.ReplyStatusUnknown},
		{state: "future_state", want: iris.ReplyStatusUnknown},
		{state: "HANDOFF_COMPLETED", want: iris.ReplyStatusUnknown},
		{state: " handoff_completed", want: iris.ReplyStatusUnknown},
		{state: "handoff_completed\n", want: iris.ReplyStatusUnknown},
		{state: "FAILED", want: iris.ReplyStatusUnknown},
		{state: "failed ", want: iris.ReplyStatusUnknown},
		{state: "sent", want: iris.ReplyStatusUnknown},
		{state: "success", want: iris.ReplyStatusUnknown},
		{state: "completed", want: iris.ReplyStatusUnknown},
		{state: "delivered", want: iris.ReplyStatusUnknown},
		{state: "accepted", want: iris.ReplyStatusUnknown},
		{state: "pending", want: iris.ReplyStatusUnknown},
		{state: "error", want: iris.ReplyStatusUnknown},
		{state: "canceled", want: iris.ReplyStatusUnknown},
	} {
		t.Run(tc.state, func(t *testing.T) {
			t.Parallel()

			if got := iris.ClassifyReplyStatus(tc.state); got != tc.want {
				t.Fatalf("ClassifyReplyStatus(%q) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}
