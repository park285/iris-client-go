package iris

import client "github.com/park285/iris-client-go/v2/internal/client/transport"

type Reaction = client.Reaction
type ReactionStatus = client.ReactionStatus
type ReactionRequest = client.ReactionRequest
type ReactionResponse = client.ReactionResponse

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

var _ ReactionClient = (*H2CClient)(nil)
var _ ReactionClient = (*RebindingClient)(nil)
