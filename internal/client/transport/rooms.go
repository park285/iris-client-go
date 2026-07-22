package transport

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/park285/iris-client-go/internal/jsonx"
)

type NicknameHistorySearchResponse struct {
	Complete               bool                         `json:"complete"`
	Truncated              bool                         `json:"truncated"`
	AsOfSourceLogID        int64                        `json:"asOfSourceLogId"`
	DurableHeadSourceLogID int64                        `json:"durableHeadSourceLogId"`
	Matches                []NicknameHistorySearchMatch `json:"matches"`
}

type NicknameHistorySearchMatch struct {
	UserID         int64                  `json:"userId"`
	LatestNickname string                 `json:"latestNickname"`
	History        []NicknameHistoryEntry `json:"history"`
}

type NicknameHistoryEntry struct {
	PreviousDisplayName string `json:"previousDisplayName"`
	CurrentDisplayName  string `json:"currentDisplayName"`
	SourceLogID         int64  `json:"sourceLogId"`
	CreatedAtMs         int64  `json:"createdAtMs"`
}

type RoomStatsOptions struct {
	Period      string
	Limit       int
	MinMessages int
}

func (c *H2CClient) GetRooms(ctx context.Context) (*RoomListResponse, error) {
	return doGet[RoomListResponse](c, ctx, PathRooms, SecretRoleBotControl)
}

func (c *H2CClient) GetMembers(ctx context.Context, chatID int64) (*MemberListResponse, error) {
	return doGet[MemberListResponse](c, ctx, fmt.Sprintf("%s/%d/members", PathRooms, chatID), SecretRoleBotControl)
}

func (c *H2CClient) GetMembersWithProfileRefresh(ctx context.Context, chatID, profileUserID int64) (*MemberListResponse, error) {
	path := fmt.Sprintf("%s/%d/members", PathRooms, chatID)
	params := url.Values{}
	if profileUserID > 0 {
		params.Set("profileUserId", strconv.FormatInt(profileUserID, 10))
	}
	path = appendCanonicalQuery(path, params)
	return doGet[MemberListResponse](c, ctx, path, SecretRoleBotControl)
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
	path = appendCanonicalQuery(path, params)
	return doGet[StatsResponse](c, ctx, path, SecretRoleBotControl)
}

func (c *H2CClient) GetMemberActivity(ctx context.Context, chatID, userID int64, period string) (*MemberActivityResponse, error) {
	path := fmt.Sprintf("%s/%d/members/%d/activity", PathRooms, chatID, userID)
	params := url.Values{}
	if period != "" {
		params.Set("period", period)
	}
	path = appendCanonicalQuery(path, params)
	return doGet[MemberActivityResponse](c, ctx, path, SecretRoleBotControl)
}

// GetThreads는 지정한 채팅방의 스레드 목록을 조회합니다.
func (c *H2CClient) GetThreads(ctx context.Context, chatID int64) (*ThreadListResponse, error) {
	return doGet[ThreadListResponse](c, ctx, fmt.Sprintf("%s/%d/threads", PathRooms, chatID), SecretRoleBotControl)
}

// GetRoomEvents는 지정한 채팅방의 이벤트 목록을 조회합니다.
func (c *H2CClient) GetRoomEvents(ctx context.Context, chatID int64, limit int, after int64) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, nil, "", limit, after, 0, "")
}

// GetRoomEventsByType는 지정한 채팅방의 이벤트 목록을 이벤트 타입으로 필터링해 조회합니다.
func (c *H2CClient) GetRoomEventsByType(ctx context.Context, chatID int64, eventType string, limit int, after int64) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, nil, eventType, limit, after, 0, "")
}

// GetRoomUserEvents는 지정한 사용자의 채팅방 이벤트 목록을 조회합니다.
func (c *H2CClient) GetRoomUserEvents(ctx context.Context, chatID, userID int64, limit int, after int64) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, "", limit, after, 0, "")
}

// GetRoomUserEventsBefore는 지정한 사용자의 이벤트를 before 이전부터 최신순으로 조회합니다.
// before가 0 이하면 최신 페이지를 조회합니다.
func (c *H2CClient) GetRoomUserEventsBefore(ctx context.Context, chatID, userID int64, limit int, before int64) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, "", limit, 0, before, "desc")
}

// GetRoomUserEventsByType는 지정한 사용자의 채팅방 이벤트 목록을 이벤트 타입으로 필터링해 조회합니다.
func (c *H2CClient) GetRoomUserEventsByType(ctx context.Context, chatID, userID int64, eventType string, limit int, after int64) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, eventType, limit, after, 0, "")
}

// GetLatestRoomUserEventsByType는 지정한 사용자의 최신 채팅방 이벤트 목록을 이벤트 타입으로 필터링해 조회합니다.
func (c *H2CClient) GetLatestRoomUserEventsByType(ctx context.Context, chatID, userID int64, eventType string, limit int) ([]RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, eventType, limit, 0, 0, "desc")
}

func (c *H2CClient) getRoomEvents(ctx context.Context, chatID int64, userID *int64, eventType string, limit int, after, before int64, order string) ([]RoomEventRecord, error) {
	path := fmt.Sprintf("%s/%d/events", PathRooms, chatID)
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if after > 0 {
		params.Set("after", strconv.FormatInt(after, 10))
	}
	if before > 0 {
		params.Set("before", strconv.FormatInt(before, 10))
	}
	if eventType != "" {
		params.Set("eventType", eventType)
	}
	if userID != nil {
		params.Set("userId", strconv.FormatInt(*userID, 10))
	}
	if order != "" {
		params.Set("order", order)
	}
	path = appendCanonicalQuery(path, params)

	result, err := doGet[[]RoomEventRecord](c, ctx, path, SecretRoleBotControl)
	if err != nil {
		return nil, err
	}
	return *result, nil
}

func (c *H2CClient) SearchNicknameHistoryExact(ctx context.Context, chatID int64, name string, limit int) (*NicknameHistorySearchResponse, error) {
	path := fmt.Sprintf("%s/%d/nickname-history/search", PathRooms, chatID)
	params := url.Values{}
	params.Set("match", "exact")
	params.Set("name", strings.TrimSpace(name))
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	path = appendCanonicalQuery(path, params)

	return doGet[NicknameHistorySearchResponse](c, ctx, path, SecretRoleBotControl)
}

// doGet는 인증, 응답 디코딩, 에러 매핑을 처리하는 제네릭 GET 헬퍼입니다.
func doGet[T any](c *H2CClient, ctx context.Context, path string, role SecretRole) (*T, error) {
	resp, err := c.doSigned(ctx, http.MethodGet, path, role)
	if err != nil {
		return nil, err
	}
	defer func() {
		//nolint:errcheck,gosec
		resp.Body.Close()
	}()

	var result T
	if err := jsonx.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", path, err)
	}
	return &result, nil
}
