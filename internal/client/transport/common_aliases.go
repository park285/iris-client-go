package transport

import "github.com/park285/iris-client-go/v2/internal/client/common"

type (
	CertReloadResponse            = common.CertReloadResponse
	BridgeHealthCheck             = common.BridgeHealthCheck
	BridgeDiscoveryHook           = common.BridgeDiscoveryHook
	BridgeDiagnosticsCapability   = common.BridgeDiagnosticsCapability
	BridgeDiagnosticsCapabilities = common.BridgeDiagnosticsCapabilities
	BridgeHealthResult            = common.BridgeHealthResult
	KeyCacheStats                 = common.KeyCacheStats
	NativeCoreDiagnostics         = common.NativeCoreDiagnostics
	TextPingWarmResponse          = common.TextPingWarmResponse
	ConfigState                   = common.ConfigState
	ConfigDiscoveredState         = common.ConfigDiscoveredState
	ConfigPendingRestart          = common.ConfigPendingRestart
	ConfigResponse                = common.ConfigResponse
	ConfigUpdateRequest           = common.ConfigUpdateRequest
	ConfigUpdateResponse          = common.ConfigUpdateResponse
	ReplyAcceptedResponse         = common.ReplyAcceptedResponse
	ReplyStatusSnapshot           = common.ReplyStatusSnapshot
	RoomListResponse              = common.RoomListResponse
	RoomSummary                   = common.RoomSummary
	MemberListResponse            = common.MemberListResponse
	MemberInfo                    = common.MemberInfo
	RoomInfoResponse              = common.RoomInfoResponse
	NoticeInfo                    = common.NoticeInfo
	BotCommandInfo                = common.BotCommandInfo
	OpenLinkInfo                  = common.OpenLinkInfo
	StatsResponse                 = common.StatsResponse
	PeriodRange                   = common.PeriodRange
	MemberStats                   = common.MemberStats
	MemberActivityResponse        = common.MemberActivityResponse
	ReplyRequest                  = common.ReplyRequest
	ReplyMention                  = common.ReplyMention
	ReplyMentionUserID            = common.ReplyMentionUserID
	Reaction                      = common.Reaction
	ReactionStatus                = common.ReactionStatus
	ReactionRequest               = common.ReactionRequest
	ReactionResponse              = common.ReactionResponse
	MediaChunkRequest             = common.MediaChunkRequest
	MediaChunkResponse            = common.MediaChunkResponse
)

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
	return common.NormalizeReplyMentionUserID(value) //nolint:wrapcheck // common 패키지의 정규화 오류를 그대로 노출한다.
}
