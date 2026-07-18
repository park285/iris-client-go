package rebind

import "github.com/park285/iris-client-go/internal/client/transport"

type H2CClient = transport.H2CClient
type ClientOption = transport.ClientOption
type SendOption = transport.SendOption
type ReplyAcceptedResponse = transport.ReplyAcceptedResponse
type ReplyStatusSnapshot = transport.ReplyStatusSnapshot
type ConfigResponse = transport.ConfigResponse
type RoomListResponse = transport.RoomListResponse
type ConfigUpdateRequest = transport.ConfigUpdateRequest
type ConfigUpdateResponse = transport.ConfigUpdateResponse
type BridgeHealthResult = transport.BridgeHealthResult
type NativeCoreDiagnostics = transport.NativeCoreDiagnostics
type TextPingWarmResponse = transport.TextPingWarmResponse
type KaringSendRequest = transport.KaringSendRequest
type KaringContentListRequest = transport.KaringContentListRequest
type KaringHololiveRequest = transport.KaringHololiveRequest
type KaringDryRunResponse = transport.KaringDryRunResponse

var NewH2CClient = transport.NewH2CClient
