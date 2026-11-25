package iris

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	client "github.com/park285/iris-client-go/v6/internal/client/transport"
)

const ReplyReissueMaxGenerations = 2

const replyReissueSuffixPrefix = ":r"

var (
	ErrReplyReissueGenerationOutOfRange = errors.New("iris: reply reissue generation out of range")
	ErrReplyReissueBaseAlreadyReissued  = errors.New("iris: reply reissue base already carries a reissue suffix")
)

var replyReissueSuffixPattern = regexp.MustCompile(`:r\d+$`)

func replyReissueSuffix(generation int) string {
	if generation <= 0 || generation > ReplyReissueMaxGenerations {
		return ""
	}

	return replyReissueSuffixPrefix + strconv.Itoa(generation)
}

func ReissuedClientRequestID(base string, generation int) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("iris: reply reissue base clientRequestId is empty")
	}

	if generation <= 0 || generation > ReplyReissueMaxGenerations {
		return "", fmt.Errorf("%w: generation=%d max=%d", ErrReplyReissueGenerationOutOfRange, generation, ReplyReissueMaxGenerations)
	}

	if replyReissueSuffixPattern.MatchString(base) {
		return "", fmt.Errorf("%w: base=%q", ErrReplyReissueBaseAlreadyReissued, base)
	}

	candidate := base + replyReissueSuffix(generation)
	if err := ValidateClientRequestID(candidate); err != nil {
		return "", fmt.Errorf("iris: validate reissued clientRequestId: %w", err)
	}

	return candidate, nil
}

func IsPreHandoffClientRequestIDConflict(err error) bool {
	return client.ClientRequestIDConflictCode(err) == HTTPErrorCodeClientRequestIDFailed
}

func IsUnrecoverableClientRequestIDConflict(err error) bool {
	return client.ClientRequestIDConflictCode(err) != ""
}
