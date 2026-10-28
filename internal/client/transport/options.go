package transport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"
)

type SendOption func(*sendOptions)

type sendOptions struct {
	ClientRequestID  *string
	ThreadID         *string
	ThreadScope      *int
	ImageContentType *string
	Mentions         []ReplyMention
}

func WithThreadID(id string) SendOption {
	return func(o *sendOptions) {
		o.ThreadID = &id
	}
}

func WithClientRequestID(id string) SendOption {
	return func(o *sendOptions) {
		o.ClientRequestID = &id
	}
}

// WithThreadScope는 reply 요청의 threadScope 값을 지정합니다.
//
// Iris는 threadId가 있는 reply를 메시지 종류와 이 값에 관계없이 항상 thread-only scope 2로
// 보냅니다. 요청 threadScope에 맞춰 전달 범위를 조정하지 않는 것이 의도한 계약입니다
// (DEC-20260926-iris-reply-thread-scope-fixed). 따라서 이 옵션은 생략하거나 WithThreadID와
// 함께 2를 전달합니다. Iris는 threadId와 함께 온 다른 양수 값도 2로 처리하며, threadId 없이
// 온 threadScope는 값과 관계없이 reply admission에서 거절합니다. SDK는 같은 규칙으로 요청 전에
// 0 이하 값과 WithThreadID 없는 모든 값을 오류로 반환합니다.
func WithThreadScope(scope int) SendOption {
	return func(o *sendOptions) {
		o.ThreadScope = &scope
	}
}

func WithImageContentType(contentType string) SendOption {
	return func(o *sendOptions) {
		o.ImageContentType = &contentType
	}
}

func WithMention(mention ReplyMention) SendOption {
	return func(o *sendOptions) {
		o.Mentions = append(o.Mentions, cloneReplyMention(mention))
	}
}

func applySendOptions(opts []SendOption) sendOptions {
	var result sendOptions

	for _, opt := range opts {
		if opt != nil {
			opt(&result)
		}
	}

	return result
}

func validateSendOptions(o sendOptions) error {
	if o.ClientRequestID != nil {
		if err := validateClientRequestID(*o.ClientRequestID); err != nil {
			return err
		}
	}

	if o.ThreadID != nil {
		if _, err := normalizeReplyThreadIDValue(*o.ThreadID); err != nil {
			return err
		}
	}

	if o.ThreadScope != nil && *o.ThreadScope <= 0 {
		return fmt.Errorf("iris: threadScope must be positive, got %d", *o.ThreadScope)
	}

	// Iris reply admission(JSON·multipart)은 threadId 없이 온 threadScope를 값과 관계없이 거절한다
	// (DEC-20260926-iris-reply-thread-scope-fixed). 서버에서만 실패하지 않도록 같은 규칙으로 먼저 거절한다.
	if o.ThreadScope != nil && o.ThreadID == nil {
		return errors.New("iris: threadScope requires threadId")
	}

	return validateReplyMentions(o.Mentions)
}

// ValidateClientRequestID는 WithClientRequestID가 전송 시점에 적용하는 것과 같은 규칙으로
// id를 검증합니다. 소비자가 id를 만드는 곳(재발급 generation suffix 포함)에서 미리 부르면
// 같은 규칙을 재구현하지 않고도 전송 전에 실패시킬 수 있습니다.
func ValidateClientRequestID(id string) error {
	return validateClientRequestID(id)
}

func validateClientRequestID(id string) error {
	id = strings.TrimSpace(id)
	if len(id) < 8 || len(id) > 160 {
		return errors.New("iris: clientRequestId must be 8..160 ASCII bytes using [A-Za-z0-9._:-]")
	}

	for _, r := range id {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			continue
		}

		switch r {
		case '.', '_', ':', '-':
			continue
		default:
			return errors.New("iris: clientRequestId must be 8..160 ASCII bytes using [A-Za-z0-9._:-]")
		}
	}

	return nil
}

func validateReplyMentions(mentions []ReplyMention) error {
	for _, mention := range mentions {
		if _, err := normalizeReplyMentionUserID(mention.UserID); err != nil {
			return err
		}

		if mention.Len < 0 {
			return fmt.Errorf("iris: mention len must be positive, got %d", mention.Len)
		}

		hasNickname := strings.TrimSpace(mention.Nickname) != ""
		hasRange := len(mention.At) > 0 && mention.Len > 0

		if !hasNickname && !hasRange {
			return errors.New("iris: mention requires nickname or at/len")
		}

		for _, position := range mention.At {
			if position <= 0 {
				return fmt.Errorf("iris: mention at positions must be positive, got %d", position)
			}
		}
	}

	return nil
}

func validateImageReplyMentions(mentions []ReplyMention) error {
	if len(mentions) == 0 {
		return nil
	}

	return errors.New("iris: mentions are supported only for text, markdown, and richtext replies")
}

func validateImageReplyOptions(o sendOptions) error {
	if o.ImageContentType != nil {
		if _, err := normalizeReplyMediaContentType(*o.ImageContentType); err != nil {
			return err
		}
	}

	return validateImageReplyMentions(o.Mentions)
}

func cloneReplyMentions(mentions []ReplyMention) []ReplyMention {
	if len(mentions) == 0 {
		return nil
	}

	out := make([]ReplyMention, 0, len(mentions))
	for _, mention := range mentions {
		out = append(out, cloneReplyMention(mention))
	}

	return out
}

func cloneReplyMention(mention ReplyMention) ReplyMention {
	if len(mention.At) > 0 {
		mention.At = append([]int(nil), mention.At...)
	}

	return mention
}

// TLS handshake·연결 수·ping 시도 상한은 소비자 조정 수요가 없어 옵션 없이 고정한다.
const (
	defaultTLSHandshakeTimeout = 5 * time.Second
	defaultMaxConnsPerHost     = 32
	defaultPingProbeTimeout    = 5 * time.Second
)

type clientOptions struct {
	Transport             string
	Timeout               time.Duration
	DialTimeout           time.Duration
	ResponseHeaderTimeout time.Duration
	IdleConnTimeout       time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	Logger                *slog.Logger
	HTTPClient            *http.Client
	TransportMetrics      TransportMetrics
	ReplyRetryMax         int // 0=비활성화(기본값), >0=/reply 최대 시도 횟수(재시도 대상은 WithReplyRetry 참조)
	h3ServerName          string
	h3CACertFile          string
	h3DialGuard           func(context.Context, net.IP) error
	baseURL               string
	botToken              string
}

type ClientOption func(*clientOptions)

// WithTransport는 정본 transport 값 h3 또는 http1을 고른다.
func WithTransport(transport string) ClientOption {
	return func(o *clientOptions) {
		o.Transport = transport
	}
}

func WithTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.Timeout = d
	}
}

func WithDialTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.DialTimeout = d
	}
}

func WithResponseHeaderTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.ResponseHeaderTimeout = d
	}
}

func WithIdleConnTimeout(d time.Duration) ClientOption {
	return func(o *clientOptions) {
		o.IdleConnTimeout = d
	}
}

func WithMaxIdleConns(n int) ClientOption {
	return func(o *clientOptions) {
		o.MaxIdleConns = n
	}
}

func WithMaxIdleConnsPerHost(n int) ClientOption {
	return func(o *clientOptions) {
		o.MaxIdleConnsPerHost = n
	}
}

func WithLogger(logger *slog.Logger) ClientOption {
	return func(o *clientOptions) {
		o.Logger = logger
	}
}

func WithHTTPClient(c *http.Client) ClientOption {
	return func(o *clientOptions) {
		if c != nil {
			o.HTTPClient = c
		}
	}
}

func WithTransportMetrics(metrics TransportMetrics) ClientOption {
	return func(o *clientOptions) {
		if metrics != nil {
			o.TransportMetrics = metrics
		}
	}
}

func WithH3ServerName(serverName string) ClientOption {
	return func(o *clientOptions) {
		o.h3ServerName = strings.TrimSpace(serverName)
	}
}

func WithH3CACertFile(path string) ClientOption {
	return func(o *clientOptions) {
		o.h3CACertFile = strings.TrimSpace(path)
	}
}

// WithH3DialGuard는 H3 연결 대상 IP를 dial context와 함께 검사하는 guard를 설정합니다.
func WithH3DialGuard(guard func(context.Context, net.IP) error) ClientOption {
	return func(o *clientOptions) {
		o.h3DialGuard = guard
	}
}

// WithReplyRetry는 /reply 경로의 최대 시도 횟수(최초 요청 포함)를 지정합니다.
//
// 재시도 대상은 둘뿐입니다. HTTP 429는 Iris 서버 계약상 미처리 응답이라 항상 재시도하고,
// transport 오류는 WithClientRequestID가 있을 때만 같은 clientRequestId로 재시도해 Iris admission이
// 중복을 흡수하게 합니다. 5xx·그 밖의 4xx·H3 egress 거부·응답 상한 초과는 재시도하지 않습니다.
// 앞선 attempt가 transport 오류로 끝나 결과를 알 수 없으면, 이후 attempt가 Iris의 clientRequestId
// 판정(CLIENT_REQUEST_ID_* code를 가진 409)을 받지 못한 채 끝난 오류는 ErrTransport로 반환합니다
// (DEC-20260731-reply-outcome-unknown-fail-closed).
func WithReplyRetry(maxAttempts int) ClientOption {
	return func(o *clientOptions) {
		o.ReplyRetryMax = maxAttempts
	}
}

func WithBaseURL(url string) ClientOption {
	return func(o *clientOptions) {
		o.baseURL = url
	}
}

// WithBotToken은 NewClientFromOptions가 읽는 bot-control 자격이다. NewAPIClient는 이 옵션을
// 읽지 않고 botToken 인자를 쓴다.
func WithBotToken(token string) ClientOption {
	return func(o *clientOptions) {
		o.botToken = token
	}
}

func applyClientOptions(opts []ClientOption) clientOptions {
	var out clientOptions

	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}

	out.Timeout = defaultPositiveDuration(out.Timeout, 10*time.Second)
	out.DialTimeout = defaultPositiveDuration(out.DialTimeout, 3*time.Second)
	out.ResponseHeaderTimeout = defaultPositiveDuration(out.ResponseHeaderTimeout, 5*time.Second)
	out.IdleConnTimeout = defaultPositiveDuration(out.IdleConnTimeout, 90*time.Second)
	out.MaxIdleConns = defaultPositiveInt(out.MaxIdleConns, 10)
	out.MaxIdleConnsPerHost = defaultPositiveInt(out.MaxIdleConnsPerHost, 10)

	if out.TransportMetrics == nil {
		out.TransportMetrics = NoopTransportMetrics{}
	}

	if out.ReplyRetryMax < 0 {
		out.ReplyRetryMax = 0
	}

	return out
}

func defaultPositiveDuration(v, fallback time.Duration) time.Duration {
	if v > 0 {
		return v
	}

	return fallback
}

func defaultPositiveInt(v, fallback int) int {
	if v > 0 {
		return v
	}

	return fallback
}
