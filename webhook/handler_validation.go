package webhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/park285/iris-client-go/v6/internal/irishmac"
)

func (h *Handler) acceptTransport(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if !isPOST(r.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)

		return nil, false
	}

	body, ok := h.authorizeRequest(w, r)
	if !ok {
		return nil, false
	}

	if !isJSONContentType(r.Header.Get("Content-Type")) {
		w.WriteHeader(http.StatusUnsupportedMediaType)

		return nil, false
	}

	return body, true
}

type hmacAuthOutcome uint8

const (
	hmacAuthReject hmacAuthOutcome = iota
	hmacAuthAccept
	hmacAuthUnavailable
)

func (h *Handler) authorizeRequest(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	if hasSignatureHeaders(r.Header) {
		if headers, ok := h.authorizeHMACHeaders(r); ok {
			body, ok := h.bufferBodyForHMAC(w, r)
			if !ok {
				return nil, false
			}

			switch h.authorizeHMACBody(r, headers, body) {
			case hmacAuthAccept:
				return body, true
			case hmacAuthUnavailable:
				w.WriteHeader(http.StatusServiceUnavailable)

				return nil, false
			case hmacAuthReject:
			}
		}
	}

	h.metrics.ObserveUnauthorized()
	w.WriteHeader(http.StatusUnauthorized)

	return nil, false
}

func (h *Handler) bufferBodyForHMAC(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body := http.MaxBytesReader(w, r.Body, h.options.maxBodyBytes)
	raw, err := io.ReadAll(body)
	closeErr := body.Close()

	if err == nil {
		err = closeErr
	}

	if err != nil {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(statusForDecodeError(err))

		return nil, false
	}

	r.Body = io.NopCloser(bytes.NewReader(raw))

	return raw, true
}

type hmacHeaders struct {
	timestamp  string
	nonce      string
	signature  string
	bodySHA256 string
	target     string
	messageID  string
}

func (h *Handler) authorizeHMACHeaders(r *http.Request) (hmacHeaders, bool) {
	_, versionStatus := webhookSignatureVersion(r.Header)
	switch versionStatus {
	case signatureVersionUnknown:
		h.signatureUnknown.Add(1)

		return hmacHeaders{}, false
	case signatureVersionMalformed:
		h.signatureMalformed.Add(1)

		return hmacHeaders{}, false
	case signatureVersionAccepted:
	}

	timestamp, nonce, signature, bodySHA256, ok := signatureHeaderValues(r.Header)
	if !ok || !timestampWithinReplayWindow(timestamp, h.replayWindow, time.Now()) {
		return hmacHeaders{}, false
	}

	target, err := irishmac.CanonicalTarget(r.URL.RequestURI())
	if err != nil {
		return hmacHeaders{}, false
	}

	messageID, present, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !present {
		return hmacHeaders{}, false
	}

	return hmacHeaders{
		timestamp:  timestamp,
		nonce:      nonce,
		signature:  signature,
		bodySHA256: bodySHA256,
		target:     target,
		messageID:  messageID,
	}, true
}

func (h *Handler) authorizeHMACBody(r *http.Request, headers hmacHeaders, body []byte) hmacAuthOutcome {
	gotBodySHA256 := irishmac.SHA256HexBytes(body)
	if !constantTimeEqualString(headers.bodySHA256, gotBodySHA256) {
		return hmacAuthReject
	}

	canonical, err := irishmac.CanonicalWebhookRequestV3(
		r.Host,
		r.Method,
		headers.target,
		headers.timestamp,
		headers.nonce,
		headers.messageID,
		gotBodySHA256,
	)
	if err != nil {
		return hmacAuthReject
	}

	expected := h.webhookSigner.Sign(canonical)
	if !constantTimeEqualString(headers.signature, expected) {
		return hmacAuthReject
	}

	h.signatureV3Validated.Add(1)

	return h.checkNonce(r.Context(), r.Method, headers.target, headers.timestamp, headers.nonce)
}

func hasSignatureHeaders(header http.Header) bool {
	return header.Get(HeaderIrisTimestamp) != "" ||
		header.Get(HeaderIrisNonce) != "" ||
		header.Get(HeaderIrisSignature) != "" ||
		header.Get(HeaderIrisBodySHA256) != "" ||
		header.Get(HeaderIrisSignatureVersion) != ""
}

type signatureVersionStatus uint8

const (
	signatureVersionAccepted signatureVersionStatus = iota
	signatureVersionUnknown
	signatureVersionMalformed
)

func webhookSignatureVersion(header http.Header) (string, signatureVersionStatus) {
	values := header.Values(HeaderIrisSignatureVersion)
	if len(values) != 1 {
		return "", signatureVersionMalformed
	}

	if values[0] == "" || strings.TrimSpace(values[0]) != values[0] {
		return "", signatureVersionMalformed
	}

	switch strings.ToLower(values[0]) {
	case SignatureVersionV3:
		return SignatureVersionV3, signatureVersionAccepted
	default:
		return "", signatureVersionUnknown
	}
}

func signatureHeaderValues(header http.Header) (string, string, string, string, bool) {
	timestamp := strings.TrimSpace(header.Get(HeaderIrisTimestamp))
	nonce := strings.TrimSpace(header.Get(HeaderIrisNonce))
	signature := strings.TrimSpace(header.Get(HeaderIrisSignature))
	bodySHA256 := strings.TrimSpace(header.Get(HeaderIrisBodySHA256))

	return timestamp, nonce, signature, bodySHA256, timestamp != "" && nonce != "" && signature != "" && bodySHA256 != ""
}

func timestampWithinReplayWindow(timestamp string, window time.Duration, now time.Time) bool {
	timestampMs, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}

	signedAt := time.UnixMilli(timestampMs)

	return !signedAt.Before(now.Add(-window)) && !signedAt.After(now.Add(window))
}

func (h *Handler) checkNonce(ctx context.Context, method, target, timestamp, nonce string) hmacAuthOutcome {
	if h.nonceStore == nil {
		return hmacAuthReject
	}

	key := strings.Join([]string{strings.ToUpper(method), target, timestamp, nonce}, "\n")
	duplicate, err := h.isNonceDuplicate(ctx, key)

	switch {
	case err != nil:
		h.logger.Warn(
			"webhook hmac nonce check failed; the request is rejected with 503 so the sender retransmits instead of dropping it",
			slog.Any("error", err),
		)

		return hmacAuthUnavailable
	case duplicate:
		return hmacAuthReject
	default:
		return hmacAuthAccept
	}
}

func (h *Handler) isNonceDuplicate(ctx context.Context, key string) (bool, error) {
	nonceCtx, cancel := context.WithTimeout(ctx, h.options.nonceTimeout)
	defer cancel()

	duplicate, err := h.nonceStore.IsDuplicate(nonceCtx, key, h.nonceReplayTTL())
	if err != nil {
		return duplicate, fmt.Errorf("check nonce replay: %w", err)
	}

	return duplicate, nil
}

func (h *Handler) nonceReplayTTL() time.Duration {
	return 2 * h.replayWindow
}

func constantTimeEqualString(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

var errInvalidWebhookRequest = errors.New("iris webhook: invalid webhook request")

func DecodeMessage(body []byte) (*Message, error) {
	req, err := parseWebhookRequest(body)
	if err != nil {
		return nil, err
	}

	return buildMessage(req), nil
}

func parseWebhookRequest(body []byte) (*WebhookRequest, error) {
	req, err := decodeWebhookRequest(body)
	if err != nil {
		return nil, err
	}

	if !validWebhookRequest(req) {
		return nil, errInvalidWebhookRequest
	}

	messageID, valid := irishmac.NormalizeMessageID(req.MessageID)
	if !valid || messageID == "" {
		return nil, fmt.Errorf("%w: messageId", errInvalidWebhookRequest)
	}

	req.MessageID = messageID

	return req, nil
}

func (h *Handler) decodeAndValidate(w http.ResponseWriter, body []byte) (*WebhookRequest, bool) {
	start := time.Now()
	req, err := parseWebhookRequest(body)

	h.metrics.ObserveDecodeLatency(time.Since(start))

	if err == nil {
		return req, true
	}

	if !errors.Is(err, errInvalidWebhookRequest) {
		h.logger.Warn("webhook decode failed", slog.Any("error", err))
	}

	h.metrics.ObserveBadRequest()
	w.WriteHeader(statusForDecodeError(err))

	return nil, false
}

func (h *Handler) reconcileMessageID(w http.ResponseWriter, r *http.Request, req *WebhookRequest) bool {
	headerID, headerPresent, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !headerPresent || req.MessageID != headerID {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return false
	}

	return true
}

func normalizedMessageIDHeader(header http.Header) (string, bool, bool) {
	values := header.Values(HeaderIrisMessageID)
	if len(values) > 1 {
		return "", false, false
	}

	if len(values) == 0 {
		return "", false, true
	}

	messageID, valid := irishmac.NormalizeMessageID(values[0])

	return messageID, messageID != "", valid
}

func decodeWebhookRequest(body []byte) (*WebhookRequest, error) {
	var req WebhookRequest

	if err := jsonv2.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("decode webhook request: %w", err)
	}

	return &req, nil
}

func statusForDecodeError(err error) int {
	if isBodyTooLarge(err) {
		return http.StatusRequestEntityTooLarge
	}

	timeout, ok := errors.AsType[net.Error](err)
	if errors.Is(err, context.DeadlineExceeded) || (ok && timeout.Timeout()) {
		return http.StatusRequestTimeout
	}

	return http.StatusBadRequest
}

func isBodyTooLarge(err error) bool {
	_, ok := errors.AsType[*http.MaxBytesError](err)

	return ok
}

func buildMessage(req *WebhookRequest) *Message {
	trimmed := normalizeWebhookRequest(req)
	msg := &Message{
		Msg:  trimmed.Text,
		Room: trimmed.Room,
		JSON: buildMessageJSON(trimmed),
	}

	if trimmed.Sender != "" {
		sender := trimmed.Sender

		msg.Sender = &sender
	}

	return msg
}

func buildMessageJSON(req WebhookRequest) *MessageJSON {
	result := &MessageJSON{
		UserID:             req.UserID,
		Type:               req.Type,
		Route:              req.Route,
		MessageID:          req.MessageID,
		ChatLogID:          req.ChatLogID,
		RoomType:           req.RoomType,
		RoomLinkID:         req.RoomLinkID,
		SourceCreatedAtMS:  req.SourceCreatedAtMS,
		RawSourceLogID:     req.RawSourceLogID,
		SourceGenerationID: req.SourceGenerationID,
		SourceAccountID:    req.SourceAccountID,
		IsMine:             req.IsMine,
		Origin:             req.Origin,
		Attachment:         req.Attachment,
		Mentions:           req.Mentions,
		EventPayload:       req.EventPayload,
	}

	if req.SourceLogID != 0 {
		sourceLogID := req.SourceLogID

		result.SourceLogID = &sourceLogID
	}

	if threadID := resolveThreadID(&req); threadID != "" {
		result.ThreadID = &threadID
	}

	if req.ThreadScope != nil {
		scope := *req.ThreadScope

		result.ThreadScope = &scope
	}

	return result
}

func normalizeWebhookRequest(req *WebhookRequest) WebhookRequest {
	if req == nil {
		return WebhookRequest{}
	}

	result := *req

	result.Route = strings.TrimSpace(result.Route)
	result.MessageID = strings.TrimSpace(result.MessageID)
	result.SourceAccountID = strings.TrimSpace(result.SourceAccountID)
	result.Sender = strings.TrimSpace(result.Sender)
	result.ChatLogID = strings.TrimSpace(result.ChatLogID)
	result.RoomType = strings.TrimSpace(result.RoomType)
	result.RoomLinkID = strings.TrimSpace(result.RoomLinkID)
	result.ThreadID = strings.TrimSpace(result.ThreadID)
	result.Type = strings.TrimSpace(result.Type)
	result.Origin = strings.TrimSpace(result.Origin)
	result.Mentions = cloneWebhookMentions(result.Mentions)

	return result
}

func cloneWebhookMentions(mentions []WebhookMention) []WebhookMention {
	if len(mentions) == 0 {
		return nil
	}

	out := make([]WebhookMention, 0, len(mentions))
	for _, mention := range mentions {
		mention.UserID = strings.TrimSpace(mention.UserID)
		mention.Nickname = strings.TrimSpace(mention.Nickname)
		mention.At = append([]int(nil), mention.At...)
		out = append(out, mention)
	}

	return out
}

func validWebhookRequest(req *WebhookRequest) bool {
	return validWebhookText(req) &&
		validRequiredMax(req.Room, 256) &&
		validRequiredMax(req.UserID, 256) &&
		validOptionalMax(req.Sender, 256) &&
		validOptionalMax(req.Route, 256) &&
		validOptionalMessageID(req.MessageID) &&
		validOptionalMax(req.SourceAccountID, 256) &&
		validOptionalMax(req.ChatLogID, 256) &&
		validOptionalMax(req.RoomType, 256) &&
		validOptionalMax(req.RoomLinkID, 256) &&
		validOptionalMax(req.ThreadID, 256) &&
		validOptionalMax(req.Type, 256) &&
		validOptionalMax(req.Origin, 64) &&
		(req.Attachment == "" || utf8.RuneCountInString(req.Attachment) <= 65536) &&
		len(req.EventPayload) <= maxEventPayloadBytes &&
		validWebhookMentions(req.Mentions)
}

func validWebhookMentions(mentions []WebhookMention) bool {
	for _, mention := range mentions {
		if strings.TrimSpace(mention.UserID) == "" {
			return false
		}
	}

	return true
}

func validWebhookText(req *WebhookRequest) bool {
	if utf8.RuneCountInString(req.Text) > 16000 {
		return false
	}

	if strings.TrimSpace(req.Text) != "" {
		return true
	}

	return strings.TrimSpace(req.Type) != ""
}

func validRequiredMax(value string, limit int) bool {
	if utf8.RuneCountInString(value) > limit {
		return false
	}

	return strings.TrimSpace(value) != ""
}

func validOptionalMax(value string, limit int) bool {
	return utf8.RuneCountInString(value) <= limit
}

func validOptionalMessageID(value string) bool {
	_, valid := irishmac.NormalizeMessageID(value)

	return valid
}

const contentTypeJSON = "application/json"

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, contentTypeJSON)
}

func isPOST(method string) bool {
	return method == http.MethodPost
}
