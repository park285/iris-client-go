package iris_test

import (
	"context"
	"time"

	"github.com/park285/iris-client-go/iris"
)

var (
	_ iris.BotClient = (*iris.H2CClient)(nil)
	_ iris.Client    = (*iris.H2CClient)(nil)

	_ iris.BotClient    = (*iris.RebindingClient)(nil)
	_ iris.Client       = (*iris.RebindingClient)(nil)
	_ iris.KaringClient = (*iris.RebindingClient)(nil)

	_ = iris.RebindingClientConfig{ResolveInterval: time.Second}
)

// GetRoomInfo/GetMemberActivity의 결과를 소비자가 선언할 수 있어야 한다.
var (
	_ = func(c *iris.H2CClient, ctx context.Context, chatID int64) (*iris.RoomInfoResponse, error) {
		return c.GetRoomInfo(ctx, chatID)
	}
	_ = func(c *iris.H2CClient, ctx context.Context, chatID, userID int64) (*iris.MemberActivityResponse, error) {
		return c.GetMemberActivity(ctx, chatID, userID, "week")
	}

	_ []iris.NoticeInfo     = nil
	_ []iris.BotCommandInfo = nil
	_ *iris.OpenLinkInfo    = nil
	_                       = iris.PeriodRange{}
)
