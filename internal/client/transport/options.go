package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v5/internal/client/common"
)

type SendOption func(*sendOptions)

type sendOptions struct {
	ClientRequestID  *string
	ThreadID         *string
	ImageContentType *string
	Mentions         []common.ReplyMention
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

func WithMention(mention common.ReplyMention) SendOption {
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

func validateReplyMentions(mentions []common.ReplyMention) error {
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

func validateImageReplyMentions(mentions []common.ReplyMention) error {
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

func cloneReplyMentions(mentions []common.ReplyMention) []common.ReplyMention {
	if len(mentions) == 0 {
		return nil
	}

	out := make([]common.ReplyMention, 0, len(mentions))
	for _, mention := range mentions {
		out = append(out, cloneReplyMention(mention))
	}

	return out
}

func cloneReplyMention(mention common.ReplyMention) common.ReplyMention {
	if len(mention.At) > 0 {
		mention.At = append([]int(nil), mention.At...)
	}

	return mention
}

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
	ReplyRetryMax    int
	h3ServerName     string
	h3CACertFile     string
	h3DialGuard      func(context.Context, net.IP) error
	baseURL          string
	botToken         string
}

type ClientOption func(*clientOptions)

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

func WithH3DialGuard(guard func(context.Context, net.IP) error) ClientOption {
	return func(o *clientOptions) {
		o.h3DialGuard = guard
	}
}

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
