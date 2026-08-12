package transport

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const PathRoomReactions = "/rooms/%d/reactions"

// ReactionClient는 Iris client가 제공하는 optional typed reaction capability입니다.
// 기존 custom client의 source compatibility를 위해 Sender와 분리합니다.
type ReactionClient interface {
	SendReaction(ctx context.Context, room int64, req ReactionRequest) (*ReactionResponse, error)
}

var _ ReactionClient = (*H2CClient)(nil)

// SendReaction은 room에 typed idempotent reaction 요청을 제출합니다.
func (c *H2CClient) SendReaction(ctx context.Context, room int64, req ReactionRequest) (*ReactionResponse, error) {
	if room <= 0 {
		return nil, fmt.Errorf("validate reaction request: room must be positive, got %d", room)
	}
	if err := validateReactionRequest(req); err != nil {
		return nil, fmt.Errorf("validate reaction request: %w", err)
	}

	req.RequestID = strings.TrimSpace(req.RequestID)
	req.ChatLogID = strings.TrimSpace(req.ChatLogID)

	path := fmt.Sprintf(PathRoomReactions, room)
	var resp ReactionResponse
	if err := c.postStrictJSON(ctx, path, req, &resp, SecretRoleBotControl); err != nil {
		return nil, fmt.Errorf("send iris reaction: %w", err)
	}
	if resp.RequestID != req.RequestID {
		return nil, errors.New("send iris reaction: response requestId does not match request")
	}

	return &resp, nil
}

func validateReactionRequest(req ReactionRequest) error {
	if err := validateReactionIdentity(req); err != nil {
		return err
	}
	if err := validateReactionOperationShape(req); err != nil {
		return err
	}

	groups := []struct {
		name      string
		reactions []Reaction
	}{
		{name: "add", reactions: req.Add},
		{name: "follow", reactions: req.Follow},
		{name: "remove", reactions: req.Remove},
	}
	for _, group := range groups {
		if err := validateReactionGroup(group.name, group.reactions); err != nil {
			return err
		}
	}
	return validateReactionOverlap(req.Follow, req.Remove)
}

func validateReactionIdentity(req ReactionRequest) error {
	if err := ValidateClientRequestID(req.RequestID); err != nil {
		return fmt.Errorf("requestId: %w", err)
	}
	chatLogID := req.ChatLogID
	if chatLogID == "" {
		return fmt.Errorf("chatLogId must not be blank")
	}
	if chatLogID[0] == '0' {
		return fmt.Errorf("chatLogId must be a canonical positive integer")
	}
	for _, character := range chatLogID {
		if character < '0' || character > '9' {
			return fmt.Errorf("chatLogId must be a canonical positive integer")
		}
	}
	if parsed, err := strconv.ParseInt(chatLogID, 10, 64); err != nil || parsed <= 0 {
		return fmt.Errorf("chatLogId must be a canonical positive integer")
	}
	if req.LinkID != nil && *req.LinkID <= 0 {
		return fmt.Errorf("linkId must be positive, got %d", *req.LinkID)
	}
	if req.Revision < 0 {
		return fmt.Errorf("revision must be non-negative, got %d", req.Revision)
	}
	return nil
}

func validateReactionOperationShape(req ReactionRequest) error {
	operationCount := len(req.Add) + len(req.Follow) + len(req.Remove)
	if operationCount == 0 {
		return fmt.Errorf("at least one reaction operation is required")
	}

	if len(req.Add) > 0 && (len(req.Follow) > 0 || len(req.Remove) > 0) {
		return fmt.Errorf("add cannot be combined with follow or remove")
	}
	return nil
}

func validateReactionGroup(name string, reactions []Reaction) error {
	if len(reactions) > 6 {
		return fmt.Errorf("%s contains too many reactions", name)
	}
	seen := make(map[Reaction]struct{}, len(reactions))
	for _, reaction := range reactions {
		if !isSupportedReaction(reaction) {
			return fmt.Errorf("%s contains unsupported reaction %q", name, reaction)
		}
		if _, exists := seen[reaction]; exists {
			return fmt.Errorf("%s contains duplicate reaction %q", name, reaction)
		}
		seen[reaction] = struct{}{}
	}
	return nil
}

func validateReactionOverlap(follow, remove []Reaction) error {
	removed := make(map[Reaction]struct{}, len(remove))
	for _, reaction := range remove {
		removed[reaction] = struct{}{}
	}
	for _, reaction := range follow {
		if _, overlaps := removed[reaction]; overlaps {
			return fmt.Errorf("follow and remove overlap on reaction %q", reaction)
		}
	}

	return nil
}

func isSupportedReaction(reaction Reaction) bool {
	switch reaction {
	case ReactionHeart, ReactionLike, ReactionCheck, ReactionLaugh, ReactionSurprise, ReactionSad:
		return true
	default:
		return false
	}
}
