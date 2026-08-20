package webhooksign

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v2/internal/client/randomhex"
	"github.com/park285/iris-client-go/v2/internal/irishmac"
)

// SignRequest는 실제 request authority를 포함하는 signature v3로 서명합니다.
func SignRequest(req *http.Request, secret string, body []byte) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := randomhex.Generate()
	return signRequest(req, secret, body, timestamp, nonce)
}

func signRequest(req *http.Request, secret string, body []byte, timestamp, nonce string) error {
	if req == nil {
		return errors.New("webhooksign: request is nil")
	}
	if req.URL == nil {
		return errors.New("webhooksign: request URL is nil")
	}
	// verifier는 POST만 받고(그 외는 405), 405는 Iris가 Dead로 분류해 재전송을 포기한다.
	// 빈 Method는 net/http이 GET으로 보내는데 서명은 ""로 계산되므로 함께 걸린다.
	if req.Method != http.MethodPost {
		return fmt.Errorf("webhooksign: request method must be %s, got %q", http.MethodPost, req.Method)
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return errors.New("webhooksign: secret is required")
	}
	messageIDs := messageIDHeaderValues(req.Header)
	if len(messageIDs) != 1 {
		return fmt.Errorf("webhooksign: exactly one %s header is required", irishmac.HeaderIrisMessageID)
	}
	// webhook verifier가 같은 값에 길이·charset을 강제하므로 signer도 같은 제약을 써야 한다.
	messageID, canonicalID := irishmac.NormalizeMessageID(messageIDs[0])
	if !canonicalID {
		return fmt.Errorf(
			"webhooksign: %s header exceeds %d bytes or carries a non-canonical byte",
			irishmac.HeaderIrisMessageID, irishmac.MaxMessageIDBytes,
		)
	}
	if messageID == "" {
		return fmt.Errorf("webhooksign: %s header is blank", irishmac.HeaderIrisMessageID)
	}
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)
	if timestamp == "" || nonce == "" {
		return errors.New("webhooksign: timestamp and nonce are required")
	}
	target, err := irishmac.CanonicalTarget(req.URL.RequestURI())
	if err != nil {
		return fmt.Errorf("webhooksign: canonicalize request target: %w", err)
	}
	bodySHA256 := irishmac.SHA256HexBytes(body)
	canonical, err := canonicalRequestV3(req, target, timestamp, nonce, messageID, bodySHA256)
	if err != nil {
		return err
	}
	signature := irishmac.NewSigner(secret).Sign(canonical)
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set(irishmac.HeaderIrisSignatureVersion, irishmac.SignatureVersionV3)
	req.Header.Set(irishmac.HeaderIrisTimestamp, timestamp)
	req.Header.Set(irishmac.HeaderIrisNonce, nonce)
	req.Header.Set(irishmac.HeaderIrisBodySHA256, bodySHA256)
	req.Header.Set(irishmac.HeaderIrisSignature, signature)
	req.Header.Set(irishmac.HeaderIrisMessageID, messageID)
	setSignedBody(req, body)
	return nil
}

func canonicalRequestV3(req *http.Request, target, timestamp, nonce, messageID, bodySHA256 string) (string, error) {
	urlCanonical, err := irishmac.CanonicalWebhookRequestV3(
		req.URL.Host, req.Method, target, timestamp, nonce, messageID, bodySHA256,
	)
	if err != nil {
		return "", fmt.Errorf("webhooksign: canonicalize URL authority: %w", err)
	}
	if req.Host == "" {
		return urlCanonical, nil
	}

	hostCanonical, err := irishmac.CanonicalWebhookRequestV3(
		req.Host, req.Method, target, timestamp, nonce, messageID, bodySHA256,
	)
	if err != nil {
		return "", fmt.Errorf("webhooksign: canonicalize request Host authority: %w", err)
	}
	if hostCanonical != urlCanonical {
		return "", errors.New("webhooksign: request Host authority does not match URL authority")
	}

	return hostCanonical, nil
}

// Header.Values는 조회 키만 canonical화하므로, 맵에 직접 꽂힌 소문자 키는 보지 못한다.
// 서명이 덮지 않은 그 값이 전선에 함께 나가면 수신 측이 어느 쪽을 채택하는지에 서명 범위가
// 좌우된다.
func messageIDHeaderValues(header http.Header) []string {
	var values []string

	for key, headerValues := range header {
		if strings.EqualFold(key, irishmac.HeaderIrisMessageID) {
			values = append(values, headerValues...)
		}
	}

	return values
}

// 서명은 body 인자를 덮는데 실제로 나가는 바이트는 req.Body다. 어긋난 조합은 반드시 401이
// 되고 그 실패는 secret 불일치와 구분되지 않으므로, 검사 대신 서명한 바이트를 요청에 심는다.
func setSignedBody(req *http.Request, body []byte) {
	if len(body) == 0 {
		req.Body = http.NoBody
		req.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
		req.ContentLength = 0

		return
	}

	signed := bytes.Clone(body)

	req.Body = io.NopCloser(bytes.NewReader(signed))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(signed)), nil }
	req.ContentLength = int64(len(signed))
}
