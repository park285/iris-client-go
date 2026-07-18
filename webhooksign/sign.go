package webhooksign

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/park285/iris-client-go/internal/client/randomhex"
	"github.com/park285/iris-client-go/internal/irishmac"
)

func SignRequest(req *http.Request, secret string, body []byte) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := randomhex.Generate("iris-webhook")
	return signRequest(req, secret, body, timestamp, nonce)
}

func signRequest(req *http.Request, secret string, body []byte, timestamp, nonce string) error {
	if req == nil {
		return errors.New("webhooksign: request is nil")
	}
	if req.URL == nil {
		return errors.New("webhooksign: request URL is nil")
	}
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return errors.New("webhooksign: secret is required")
	}
	messageIDs := req.Header.Values(irishmac.HeaderIrisMessageID)
	if len(messageIDs) != 1 {
		return fmt.Errorf("webhooksign: exactly one %s header is required", irishmac.HeaderIrisMessageID)
	}
	messageID := strings.TrimSpace(messageIDs[0])
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
	canonical := irishmac.CanonicalWebhookRequestV2(req.Method, target, timestamp, nonce, messageID, bodySHA256)
	signature := irishmac.NewSigner(secret).Sign(canonical)
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	req.Header.Set(irishmac.HeaderIrisSignatureVersion, irishmac.SignatureVersionV2)
	req.Header.Set(irishmac.HeaderIrisTimestamp, timestamp)
	req.Header.Set(irishmac.HeaderIrisNonce, nonce)
	req.Header.Set(irishmac.HeaderIrisBodySHA256, bodySHA256)
	req.Header.Set(irishmac.HeaderIrisSignature, signature)
	return nil
}
