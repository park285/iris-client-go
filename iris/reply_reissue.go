package iris

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// ReplyReissueMaxGenerations는 스택 공통 reissue 세대 상한이다. Iris admission 유지 기간과
// 소비자 replay 지평이 이 값을 전제로 게이트(check-stack-reissue-contract.sh)에 고정되어 있으므로,
// 값 변경은 반드시 게이트·세 소비자 봇과 함께 움직여야 한다.
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

// ReissuedClientRequestID는 base clientRequestId의 generation차 재발급 id를 만들고
// Iris 계약(8..160 ASCII, [A-Za-z0-9._:-])으로 검증한다.
func ReissuedClientRequestID(base string, generation int) (string, error) {
	base = strings.TrimSpace(base)
	if base == "" {
		return "", errors.New("iris: reply reissue base clientRequestId is empty")
	}

	if generation <= 0 || generation > ReplyReissueMaxGenerations {
		return "", fmt.Errorf("%w: generation=%d max=%d", ErrReplyReissueGenerationOutOfRange, generation, ReplyReissueMaxGenerations)
	}

	// 직전 candidate를 base로 되먹이면 ":r1:r2"처럼 ladder가 중첩되어, 게이트가 전제하는
	// 단일 ":rN" 세대 결정성과 crash-replay 시 저장된 id 매칭이 깨진다. 원본 id만 base로 허용한다.
	if replyReissueSuffixPattern.MatchString(base) {
		return "", fmt.Errorf("%w: base=%q", ErrReplyReissueBaseAlreadyReissued, base)
	}

	candidate := base + replyReissueSuffix(generation)
	if err := ValidateClientRequestID(candidate); err != nil {
		return "", fmt.Errorf("iris: validate reissued clientRequestId: %w", err)
	}

	return candidate, nil
}

// IsPreHandoffClientRequestIDConflict는 Iris가 "durable queue handoff 전에 실패했으니 새
// clientRequestId를 쓰라"고 답한 409(CLIENT_REQUEST_ID_FAILED)에서만 참이다. 이때만 세대를
// 올린 재발급이 안전하다 — 다른 409는 전송 결과 미상이거나 payload 불일치라 재발급하면 중복 발화가 된다.
func IsPreHandoffClientRequestIDConflict(err error) bool {
	if !isClientRequestIDConflictStatus(err) {
		return false
	}

	return HTTPErrorCode(err) == HTTPErrorCodeClientRequestIDFailed
}

// IsTerminalClientRequestIDConflict는 재발급으로도 해소되지 않는 409 계열
// (payload mismatch, outcome unknown, already exists)에서 참이다.
func IsTerminalClientRequestIDConflict(err error) bool {
	if !isClientRequestIDConflictStatus(err) {
		return false
	}

	switch HTTPErrorCode(err) {
	case HTTPErrorCodeClientRequestIDPayloadMismatch,
		HTTPErrorCodeClientRequestIDOutcomeUnknown,
		HTTPErrorCodeClientRequestIDAlreadyExists:
		return true
	default:
		return false
	}
}

// IsUnrecoverableClientRequestIDConflict는 재전송을 반복해도 같은 결과가 나오는 409 전체다.
// pre-handoff conflict도 포함한다 — 호출자의 reissue ladder가 세대를 소진한 뒤에 남은 오류이기 때문이다.
func IsUnrecoverableClientRequestIDConflict(err error) bool {
	return IsPreHandoffClientRequestIDConflict(err) || IsTerminalClientRequestIDConflict(err)
}

func isClientRequestIDConflictStatus(err error) bool {
	var httpErr *HTTPError

	return errors.As(err, &httpErr) && httpErr != nil && httpErr.StatusCode == http.StatusConflict
}
