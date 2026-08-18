package transport

import "github.com/park285/iris-client-go/v2/internal/client/common"

type CertReloadResponse = common.CertReloadResponse
type BridgeHealthCheck = common.BridgeHealthCheck
type BridgeDiscoveryHook = common.BridgeDiscoveryHook
type BridgeDiagnosticsCapability = common.BridgeDiagnosticsCapability
type BridgeDiagnosticsCapabilities = common.BridgeDiagnosticsCapabilities
type BridgeHealthResult = common.BridgeHealthResult
type KeyCacheStats = common.KeyCacheStats
type NativeCoreDiagnostics = common.NativeCoreDiagnostics
type TextPingWarmResponse = common.TextPingWarmResponse
type ConfigState = common.ConfigState
type ConfigDiscoveredState = common.ConfigDiscoveredState
type ConfigPendingRestart = common.ConfigPendingRestart
type ConfigResponse = common.ConfigResponse
type ConfigUpdateRequest = common.ConfigUpdateRequest
type ConfigUpdateResponse = common.ConfigUpdateResponse
type ReplyAcceptedResponse = common.ReplyAcceptedResponse
type ReplyStatusSnapshot = common.ReplyStatusSnapshot
type RoomListResponse = common.RoomListResponse
type RoomSummary = common.RoomSummary
type MemberListResponse = common.MemberListResponse
type MemberInfo = common.MemberInfo
type RoomInfoResponse = common.RoomInfoResponse
type NoticeInfo = common.NoticeInfo
type BotCommandInfo = common.BotCommandInfo
type OpenLinkInfo = common.OpenLinkInfo
type StatsResponse = common.StatsResponse
type PeriodRange = common.PeriodRange
type MemberStats = common.MemberStats
type MemberActivityResponse = common.MemberActivityResponse
type ReplyRequest = common.ReplyRequest
type ReplyMention = common.ReplyMention
type ReplyMentionUserID = common.ReplyMentionUserID
type Reaction = common.Reaction
type ReactionStatus = common.ReactionStatus
type ReactionRequest = common.ReactionRequest
type ReactionResponse = common.ReactionResponse
type MediaChunkRequest = common.MediaChunkRequest
type MediaChunkResponse = common.MediaChunkResponse

const (
	ReactionHeart    = common.ReactionHeart
	ReactionLike     = common.ReactionLike
	ReactionCheck    = common.ReactionCheck
	ReactionLaugh    = common.ReactionLaugh
	ReactionSurprise = common.ReactionSurprise
	ReactionSad      = common.ReactionSad

	ReactionStatusSent           = common.ReactionStatusSent
	ReactionStatusFailed         = common.ReactionStatusFailed
	ReactionStatusOutcomeUnknown = common.ReactionStatusOutcomeUnknown
)

func normalizeReplyMentionUserID(value ReplyMentionUserID) (ReplyMentionUserID, error) {
	return common.NormalizeReplyMentionUserID(value)
}
