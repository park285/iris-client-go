package client

import "context"

var _ RoomClient = (*legacyRoomClientStub)(nil)

type legacyRoomClientStub struct{}

func (*legacyRoomClientStub) GetRooms(context.Context) (*RoomListResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetMembers(context.Context, int64) (*MemberListResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetRoomInfo(context.Context, int64) (*RoomInfoResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetRoomStats(context.Context, int64, RoomStatsOptions) (*StatsResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetMemberActivity(context.Context, int64, int64, string) (*MemberActivityResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetThreads(context.Context, int64) (*ThreadListResponse, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetRoomEvents(context.Context, int64, int, int64) ([]RoomEventRecord, error) {
	return nil, nil
}

func (*legacyRoomClientStub) GetRoomUserEvents(context.Context, int64, int64, int, int64) ([]RoomEventRecord, error) {
	return nil, nil
}
