package iris

import client "github.com/park285/iris-client-go/v2/internal/client/transport"

type (
	Reaction         = client.Reaction
	ReactionStatus   = client.ReactionStatus
	ReactionRequest  = client.ReactionRequest
	ReactionResponse = client.ReactionResponse
)

type ReactionClient = client.ReactionClient

const PathRoomReactions = client.PathRoomReactions

const (
	ReactionHeart    = client.ReactionHeart
	ReactionLike     = client.ReactionLike
	ReactionCheck    = client.ReactionCheck
	ReactionLaugh    = client.ReactionLaugh
	ReactionSurprise = client.ReactionSurprise
	ReactionSad      = client.ReactionSad

	ReactionStatusSent           = client.ReactionStatusSent
	ReactionStatusFailed         = client.ReactionStatusFailed
	ReactionStatusOutcomeUnknown = client.ReactionStatusOutcomeUnknown
)

var (
	_ ReactionClient = (*APIClient)(nil)
	_ ReactionClient = (*RebindingClient)(nil)
)
