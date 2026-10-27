package common

type RoomListResponse struct {
	Rooms []RoomSummary `json:"rooms"`
}

type RoomSummary struct {
	ChatID             int64   `json:"chatId"`
	Type               *string `json:"type,omitempty"`
	LinkID             *int64  `json:"linkId,omitempty"`
	ActiveMembersCount *int    `json:"activeMembersCount,omitempty"`
	LinkName           *string `json:"linkName,omitempty"`
	LinkURL            *string `json:"linkUrl,omitempty"`
	MemberLimit        *int    `json:"memberLimit,omitempty"`
	Searchable         *int    `json:"searchable,omitempty"`
	BotRole            *int    `json:"botRole,omitempty"`
}

type MemberListResponse struct {
	ChatID     int64        `json:"chatId"`
	LinkID     *int64       `json:"linkId,omitempty"`
	Members    []MemberInfo `json:"members"`
	TotalCount int          `json:"totalCount"`
}

type MemberInfo struct {
	UserID          int64   `json:"userId"`
	Nickname        *string `json:"nickname,omitempty"`
	Role            string  `json:"role"`
	RoleCode        int     `json:"roleCode"`
	ProfileImageURL *string `json:"profileImageUrl,omitempty"`
	MessageCount    int     `json:"messageCount"`
	LastActiveAt    *int64  `json:"lastActiveAt,omitempty"`
}

type StatsResponse struct {
	ChatID        int64         `json:"chatId"`
	Period        PeriodRange   `json:"period"`
	TotalMessages int           `json:"totalMessages"`
	ActiveMembers int           `json:"activeMembers"`
	TopMembers    []MemberStats `json:"topMembers"`
}

type PeriodRange struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

type MemberStats struct {
	UserID       int64          `json:"userId"`
	Nickname     *string        `json:"nickname,omitempty"`
	MessageCount int            `json:"messageCount"`
	LastActiveAt *int64         `json:"lastActiveAt,omitempty"`
	MessageTypes map[string]int `json:"messageTypes"`
}
