package transport

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/park285/iris-client-go/v6/internal/client/common"
	"github.com/park285/iris-client-go/v6/internal/client/query"
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

func (c *APIClient) GetRooms(ctx context.Context) (*common.RoomListResponse, error) {
	return c.doGet[common.RoomListResponse](ctx, PathRooms)
}

func (c *APIClient) GetMembers(ctx context.Context, chatID int64) (*common.MemberListResponse, error) {
	return c.doGet[common.MemberListResponse](ctx, fmt.Sprintf("%s/%d/members", PathRooms, chatID))
}

func (c *APIClient) GetMembersWithProfileRefresh(ctx context.Context, chatID, profileUserID int64) (*common.MemberListResponse, error) {
	path := fmt.Sprintf("%s/%d/members", PathRooms, chatID)
	params := url.Values{}

	if profileUserID > 0 {
		params.Set("profileUserId", strconv.FormatInt(profileUserID, 10))
	}

	path = appendCanonicalQuery(path, params)

	return c.doGet[common.MemberListResponse](ctx, path)
}

func (c *APIClient) GetRoomEventsByType(ctx context.Context, chatID int64, eventType string, limit int, after int64) ([]query.RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, nil, eventType, limit, after, 0, "")
}

func (c *APIClient) GetRoomUserEventsBefore(ctx context.Context, chatID, userID int64, limit int, before int64) ([]query.RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, "", limit, 0, before, "desc")
}

func (c *APIClient) GetLatestRoomUserEventsByType(ctx context.Context, chatID, userID int64, eventType string, limit int) ([]query.RoomEventRecord, error) {
	return c.getRoomEvents(ctx, chatID, &userID, eventType, limit, 0, 0, "desc")
}

func (c *APIClient) getRoomEvents(ctx context.Context, chatID int64, userID *int64, eventType string, limit int, after, before int64, order string) ([]query.RoomEventRecord, error) {
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

	result, err := c.doGet[[]query.RoomEventRecord](ctx, path)
	if err != nil {
		return nil, err
	}

	return *result, nil
}

func (c *APIClient) SearchNicknameHistoryExact(ctx context.Context, chatID int64, name string, limit int) (*NicknameHistorySearchResponse, error) {
	path := fmt.Sprintf("%s/%d/nickname-history/search", PathRooms, chatID)
	params := url.Values{}
	params.Set("match", "exact")
	params.Set("name", strings.TrimSpace(name))

	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	path = appendCanonicalQuery(path, params)

	return c.doGet[NicknameHistorySearchResponse](ctx, path)
}
