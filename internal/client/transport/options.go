package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v4/internal/client/common"
)

type SendOption func(*sendOptions)

type sendOptions struct {
	ClientRequestID  *string
	ThreadID         *string
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
		if _, err := common.NormalizeReplyMentionUserID(mention.UserID); err != nil {
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

	return errors.New("iris: mentions are supported only for text and markdown replies")
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

// http1 transport의 연결 상한과 ping 시도 상한은 소비자 조정 수요가 없어 옵션 없이 고정한다.
// Http1은 loopback /health·/ready probe 전용이고 h3는 QUIC 상수(http3_transport.go)만 쓰므로,
// dial·응답 header·idle 조정 옵션은 두지 않는다.
const (
	defaultTLSHandshakeTimeout      = 5 * time.Second
	defaultMaxConnsPerHost          = 32
	defaultPingProbeTimeout         = 5 * time.Second
	defaultHTTP1DialTimeout         = 3 * time.Second
	defaultHTTP1HeaderTimeout       = 5 * time.Second
	defaultHTTP1IdleConnTimeout     = 90 * time.Second
	defaultHTTP1MaxIdleConns        = 10
	defaultHTTP1MaxIdleConnsPerHost = 10
)

type clientOptions struct {
	Transport        string
	Timeout          time.Duration
	HTTPClient       *http.Client
	TransportMetrics TransportMetrics
	ReplyRetryMax    int // 0=비활성화(기본값), >0=/reply 최대 시도 횟수(재시도 대상은 WithReplyRetry 참조)
	h3ServerName     string
	h3CACertFile     string
	h3DialGuard      func(context.Context, net.IP) error
	baseURL          string
	botToken         string
}

type ClientOption func(*clientOptions)

// WithTransport는 정본 transport 값 h3 또는 http1을 고른다. 비우면 h3이고, 그 밖의 값은
// ParseTransport와 같은 규칙으로 client 생성 시 거절한다.
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

// WithBotToken은 NewClient가 요구하는 bot-control 서명 자격이다.
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
