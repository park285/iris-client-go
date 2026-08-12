package iris_test

import (
	"testing"

	"github.com/park285/iris-client-go/iris"
)

func TestReactionPublicAliases(t *testing.T) {
	t.Parallel()

	var _ iris.ReactionClient = (*iris.H2CClient)(nil)
	var _ iris.ReactionClient = (*iris.RebindingClient)(nil)

	req := iris.ReactionRequest{
		RequestID: "reaction:req-1",
		ChatLogID: "1",
		Add:       []iris.Reaction{iris.ReactionLike},
	}
	if req.Add[0] != iris.ReactionLike {
		t.Fatalf("Reaction alias = %q, want %q", req.Add[0], iris.ReactionLike)
	}
	if iris.PathRoomReactions != "/rooms/%d/reactions" {
		t.Fatalf("PathRoomReactions = %q", iris.PathRoomReactions)
	}

	var response iris.ReactionResponse
	response.Status = iris.ReactionStatusSent
	if response.Status != iris.ReactionStatusSent {
		t.Fatalf("ReactionStatus alias = %q", response.Status)
	}
}
