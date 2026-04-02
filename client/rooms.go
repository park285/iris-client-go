package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/park285/iris-client-go/internal/jsonx"
)

type RoomClient interface {
	GetRooms(ctx context.Context) (*RoomListResponse, error)
	GetMembers(ctx context.Context, chatID int64) (*MemberListResponse, error)
	GetRoomInfo(ctx context.Context, chatID int64) (*RoomInfoResponse, error)
	GetRoomStats(ctx context.Context, chatID int64, opts RoomStatsOptions) (*StatsResponse, error)
	GetMemberActivity(ctx context.Context, chatID, userID int64, period string) (*MemberActivityResponse, error)
	GetThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error)
	GetRoomEvents(ctx context.Context, chatID int64, limit int, after int64) ([]RoomEventRecord, error)
}

type RoomStatsOptions struct {
	Period      string
	Limit       int
	MinMessages int
}

var _ RoomClient = (*H2CClient)(nil)

func (c *H2CClient) GetRooms(ctx context.Context) (*RoomListResponse, error) {
	return doGet[RoomListResponse](c, ctx, PathRooms, SecretRoleBotControl)
}

func (c *H2CClient) GetMembers(ctx context.Context, chatID int64) (*MemberListResponse, error) {
	return doGet[MemberListResponse](c, ctx, fmt.Sprintf("%s/%d/members", PathRooms, chatID), SecretRoleBotControl)
}

func (c *H2CClient) GetRoomInfo(ctx context.Context, chatID int64) (*RoomInfoResponse, error) {
	return doGet[RoomInfoResponse](c, ctx, fmt.Sprintf("%s/%d/info", PathRooms, chatID), SecretRoleBotControl)
}

func (c *H2CClient) GetRoomStats(ctx context.Context, chatID int64, opts RoomStatsOptions) (*StatsResponse, error) {
	path := fmt.Sprintf("%s/%d/stats", PathRooms, chatID)
	params := url.Values{}
	if opts.Period != "" {
		params.Set("period", opts.Period)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.MinMessages > 0 {
		params.Set("minMessages", strconv.Itoa(opts.MinMessages))
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return doGet[StatsResponse](c, ctx, path, SecretRoleBotControl)
}

func (c *H2CClient) GetMemberActivity(ctx context.Context, chatID, userID int64, period string) (*MemberActivityResponse, error) {
	path := fmt.Sprintf("%s/%d/members/%d/activity", PathRooms, chatID, userID)
	if period != "" {
		path += "?period=" + url.QueryEscape(period)
	}
	return doGet[MemberActivityResponse](c, ctx, path, SecretRoleBotControl)
}

// GetThreads는 지정한 채팅방의 스레드 목록을 조회합니다.
func (c *H2CClient) GetThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error) {
	return doGet[ThreadListResponse](c, ctx, fmt.Sprintf("%s/%d/threads", PathRooms, chatID), SecretRoleBotControl)
}

// GetRoomEvents는 지정한 채팅방의 이벤트 목록을 조회합니다.
func (c *H2CClient) GetRoomEvents(ctx context.Context, chatID int64, limit int, after int64) ([]RoomEventRecord, error) {
	path := fmt.Sprintf("%s/%d/events", PathRooms, chatID)
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if encoded := params.Encode(); encoded != "" {
		path += "?" + encoded
	}

	req, err := c.newSignedRequest(ctx, http.MethodGet, path, nil, SecretRoleBotControl)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}

	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s: %w", path, readErrorResponse(path, resp))
	}

	var result []RoomEventRecord
	if err := jsonx.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	return result, nil
}

// doGet는 인증, 응답 디코딩, 에러 매핑을 처리하는 제네릭 GET 헬퍼입니다.
func doGet[T any](c *H2CClient, ctx context.Context, path string, role SecretRole) (*T, error) {
	req, err := c.newSignedRequest(ctx, http.MethodGet, path, nil, role)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get %s: %w", path, err)
	}

	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get %s: %w", path, readErrorResponse(path, resp))
	}

	var result T
	if err := jsonx.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}

	return &result, nil
}
