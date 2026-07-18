package webhook

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/park285/iris-client-go/internal/irishmac"
	"github.com/park285/iris-client-go/internal/jsonx"
)

func (h *Handler) acceptTransport(w http.ResponseWriter, r *http.Request) bool {
	if !isPOST(r.Method) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return false
	}
	if h.rejectMissingToken(w) {
		return false
	}
	if !h.rejectUnauthorized(w, r) {
		return false
	}
	if !isJSONContentType(r.Header.Get("Content-Type")) {
		w.WriteHeader(http.StatusUnsupportedMediaType)
		return false
	}
	return true
}

func (h *Handler) rejectMissingToken(w http.ResponseWriter) bool {
	if h.token != "" || h.webhookSecret != "" {
		return false
	}

	w.WriteHeader(http.StatusInternalServerError)

	return true
}

func (h *Handler) rejectUnauthorized(w http.ResponseWriter, r *http.Request) bool {
	if hasSignatureHeaders(r.Header) {
		body, ok := h.bufferBodyForHMAC(w, r)
		if !ok {
			return false
		}
		if h.authorizeHMAC(r, body) {
			return true
		}
		h.metrics.ObserveUnauthorized()
		w.WriteHeader(http.StatusUnauthorized)

		return false
	}

	h.metrics.ObserveUnauthorized()
	w.WriteHeader(http.StatusUnauthorized)

	return false
}

func (h *Handler) bufferBodyForHMAC(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	body := http.MaxBytesReader(w, r.Body, h.options.MaxBodyBytes)
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

func (h *Handler) authorizeHMAC(r *http.Request, body []byte) bool {
	if !hasWebhookSignatureVersionV2(r.Header) {
		return false
	}
	timestamp, nonce, signature, bodySHA256, ok := signatureHeaderValues(r.Header)
	if !ok || !timestampWithinReplayWindow(timestamp, h.replayWindow, time.Now()) {
		return false
	}

	gotBodySHA256 := irishmac.SHA256HexBytes(body)
	if !constantTimeEqualString(bodySHA256, gotBodySHA256) {
		return false
	}

	target, err := irishmac.CanonicalTarget(r.URL.RequestURI())
	if err != nil {
		return false
	}
	messageID, present, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !present {
		return false
	}
	canonical := canonicalWebhookRequestV2(r.Method, target, timestamp, nonce, messageID, gotBodySHA256)
	expected := h.webhookSigner.Sign(canonical)
	if !constantTimeEqualString(signature, expected) {
		return false
	}

	return !h.isReplay(r.Context(), r.Method, target, timestamp, nonce)
}

func canonicalWebhookRequestV2(method, target, timestamp, nonce, messageID, bodySHA256 string) string {
	return irishmac.CanonicalWebhookRequestV2(method, target, timestamp, nonce, messageID, bodySHA256)
}

func hasSignatureHeaders(header http.Header) bool {
	return header.Get(HeaderIrisTimestamp) != "" ||
		header.Get(HeaderIrisNonce) != "" ||
		header.Get(HeaderIrisSignature) != "" ||
		header.Get(HeaderIrisBodySHA256) != "" ||
		header.Get(HeaderIrisSignatureVersion) != ""
}

func hasWebhookSignatureVersionV2(header http.Header) bool {
	values := header.Values(HeaderIrisSignatureVersion)
	return len(values) == 1 && strings.EqualFold(strings.TrimSpace(values[0]), SignatureVersionV2)
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
	delta := now.Sub(time.UnixMilli(timestampMs))
	if delta < 0 {
		delta = -delta
	}
	return delta <= window
}

func (h *Handler) isReplay(ctx context.Context, method, target, timestamp, nonce string) bool {
	if h.nonceCache == nil {
		return true
	}
	key := strings.Join([]string{strings.ToUpper(method), target, timestamp, nonce}, "\n")
	duplicate, err := h.isNonceDuplicate(ctx, key)
	if err != nil {
		h.logger.Warn("webhook hmac nonce check failed", slog.Any("error", err))

		return true
	}
	return duplicate
}

func (h *Handler) isNonceDuplicate(ctx context.Context, key string) (bool, error) {
	dedupCtx := ctx
	cancel := func() {}
	if h.options.DedupTimeout > 0 {
		dedupCtx, cancel = context.WithTimeout(ctx, h.options.DedupTimeout)
	}
	defer cancel()
	return h.nonceCache.IsDuplicate(dedupCtx, key, h.nonceReplayTTL())
}

// timestamp를 미래 방향으로 window까지(now+window) 수용하므로, 서명자 시계가 앞선 nonce가
// 만료 후 (now+window, ts+window] 구간에서 재사용되지 않으려면 최초 수신(now-window)부터
// 최종 수용(ts+window)까지 최대 2*window를 덮어야 한다.
func (h *Handler) nonceReplayTTL() time.Duration {
	return 2 * h.replayWindow
}

func constantTimeEqualString(left, right string) bool {
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (h *Handler) decodeAndValidate(w http.ResponseWriter, r *http.Request) (*WebhookRequest, bool) {
	start := time.Now()
	req, err := decodeWebhookRequest(w, r, h.options.MaxBodyBytes)
	h.metrics.ObserveDecodeLatency(time.Since(start))
	status := 0
	if err != nil {
		h.logger.Warn("webhook decode failed", slog.Any("error", err))
		status = statusForDecodeError(err)
	} else if !validWebhookRequest(req) {
		status = http.StatusBadRequest
	}
	if status != 0 {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(status)

		return nil, false
	}

	return req, true
}

func canonicalDedupID(req *WebhookRequest) string {
	if req == nil {
		return ""
	}

	return req.MessageID
}

func (h *Handler) reconcileMessageID(w http.ResponseWriter, r *http.Request, req *WebhookRequest) bool {
	bodyID, valid := normalizeMessageID(req.MessageID)
	if !valid {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return false
	}
	headerID, headerPresent, valid := normalizedMessageIDHeader(r.Header)
	if !valid || !headerPresent || (bodyID != "" && bodyID != headerID) {
		h.metrics.ObserveBadRequest()
		w.WriteHeader(http.StatusBadRequest)

		return false
	}

	if bodyID == "" {
		req.MessageID = headerID
	} else {
		req.MessageID = bodyID
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

	messageID, valid := normalizeMessageID(values[0])

	return messageID, messageID != "", valid
}

func normalizeMessageID(raw string) (string, bool) {
	messageID := strings.TrimSpace(raw)
	if messageID == "" {
		return "", true
	}
	if len(messageID) > maxMessageIDBytes {
		return "", false
	}
	for i := range len(messageID) {
		if validMessageIDByte(messageID[i]) {
			continue
		}

		return "", false
	}

	return messageID, true
}

func validMessageIDByte(character byte) bool {
	return character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' ||
		strings.ContainsRune("-_.:/", rune(character))
}

func decodeWebhookRequest(
	w http.ResponseWriter,
	r *http.Request,
	maxBodyBytes int64,
) (*WebhookRequest, error) {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)

	defer func() {
		_ = body.Close() //nolint:errcheck // 디코딩 후 request body를 닫는 것은 best-effort다.
	}()

	decoder := jsonx.NewDecoder(body)

	var req WebhookRequest
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("decode webhook request: %w", err)
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, fmt.Errorf("ensure single JSON value: %w", err)
	}

	return &req, nil
}

func ensureSingleJSONValue(decoder jsonx.Decoder) error {
	var extra struct{}
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("webhook request contains multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode trailing JSON value: %w", err)
	}

	return nil
}

func statusForDecodeError(err error) int {
	if isBodyTooLarge(err) {
		return http.StatusRequestEntityTooLarge
	}

	return http.StatusBadRequest
}

func isBodyTooLarge(err error) bool {
	var maxBytesErr *http.MaxBytesError

	return errors.As(err, &maxBytesErr)
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
		Message:            req.Text,
		ChatID:             req.Room,
		Type:               req.Type,
		Route:              req.Route,
		MessageID:          req.MessageID,
		ChatLogID:          req.ChatLogID,
		RoomType:           req.RoomType,
		RoomLinkID:         req.RoomLinkID,
		RawSourceLogID:     req.RawSourceLogID,
		SourceGenerationID: req.SourceGenerationID,
		SourceAccountID:    req.SourceAccountID,
		IsMine:             req.IsMine,
		Origin:             req.Origin,
		Attachment:         req.Attachment,
		Mentions:           cloneWebhookMentions(req.Mentions),
		EventPayload:       req.EventPayload,
	}

	if req.SourceLogID != 0 {
		sourceLogID := req.SourceLogID

		result.SourceLogID = &sourceLogID
	}

	if threadID := ResolveThreadID(&req); threadID != "" {
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
		len(req.EventPayload) <= maxEventPayloadBytes
}

func validWebhookText(req *WebhookRequest) bool {
	if utf8.RuneCountInString(req.Text) > 16000 {
		return false
	}

	if strings.TrimSpace(req.Text) != "" {
		return true
	}

	return strings.TrimSpace(req.Type) != "" && strings.TrimSpace(string(req.EventPayload)) != ""
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
	_, valid := normalizeMessageID(value)

	return valid
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.EqualFold(mediaType, "application/json")
}

func isPOST(method string) bool {
	return method == http.MethodPost
}
