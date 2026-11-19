package webhooksign

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/park285/iris-client-go/v5/internal/client/randomhex"
	"github.com/park285/iris-client-go/v5/internal/irishmac"
)

func SignRequest(req *http.Request, secret string, body []byte) error {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	nonce := randomhex.Generate()

	return signRequest(req, secret, body, timestamp, nonce)
}

func signRequest(req *http.Request, secret string, body []byte, timestamp, nonce string) error {
	if err := validateSigningRequest(req); err != nil {
		return err
	}

	secret, err := normalizedSigningSecret(secret)
	if err != nil {
		return err
	}

	messageID, err := normalizedSigningMessageID(req.Header)
	if err != nil {
		return err
	}

	timestamp, nonce, err = normalizedSigningTime(timestamp, nonce)
	if err != nil {
		return err
	}

	target, err := irishmac.CanonicalTarget(req.URL.RequestURI())
	if err != nil {
		return fmt.Errorf("webhooksign: canonicalize request target: %w", err)
	}

	bodySHA256 := irishmac.SHA256HexBytes(body)

	canonical, err := canonicalRequestV3(req, target, timestamp, nonce, messageID, bodySHA256)
	if err != nil {
		return fmt.Errorf("canonical request: %w", err)
	}

	signature := irishmac.SignHash(irishmac.NewMAC([]byte(secret)), canonical)

	if req.Header == nil {
		req.Header = make(http.Header)
	}

	req.Header.Set(irishmac.HeaderIrisSignatureVersion, irishmac.SignatureVersionV3)
	req.Header.Set(irishmac.HeaderIrisTimestamp, timestamp)
	req.Header.Set(irishmac.HeaderIrisNonce, nonce)
	req.Header.Set(irishmac.HeaderIrisBodySHA256, bodySHA256)
	req.Header.Set(irishmac.HeaderIrisSignature, signature)
	maps.DeleteFunc(req.Header, func(key string, _ []string) bool {
		return strings.EqualFold(key, irishmac.HeaderIrisMessageID)
	})
	req.Header.Set(irishmac.HeaderIrisMessageID, messageID)
	setSignedBody(req, body)

	return nil
}

func validateSigningRequest(req *http.Request) error {
	if req == nil {
		return errors.New("webhooksign: request is nil")
	}

	if req.URL == nil {
		return errors.New("webhooksign: request URL is nil")
	}

	if req.Method != http.MethodPost {
		return fmt.Errorf("webhooksign: request method must be %s, got %q", http.MethodPost, req.Method)
	}

	return nil
}

func normalizedSigningSecret(secret string) (string, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "", errors.New("webhooksign: secret is required")
	}

	return secret, nil
}

func normalizedSigningMessageID(header http.Header) (string, error) {
	messageIDs := messageIDHeaderValues(header)
	if len(messageIDs) != 1 {
		return "", fmt.Errorf("webhooksign: exactly one %s header is required", irishmac.HeaderIrisMessageID)
	}

	messageID, canonicalID := irishmac.NormalizeMessageID(messageIDs[0])
	if !canonicalID {
		return "", fmt.Errorf(
			"webhooksign: %s header exceeds %d bytes or carries a non-canonical byte",
			irishmac.HeaderIrisMessageID, irishmac.MaxMessageIDBytes,
		)
	}

	if messageID == "" {
		return "", fmt.Errorf("webhooksign: %s header is blank", irishmac.HeaderIrisMessageID)
	}

	return messageID, nil
}

func normalizedSigningTime(timestamp, nonce string) (string, string, error) {
	timestamp = strings.TrimSpace(timestamp)
	nonce = strings.TrimSpace(nonce)

	if timestamp == "" || nonce == "" {
		return "", "", errors.New("webhooksign: timestamp and nonce are required")
	}

	return timestamp, nonce, nil
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

func messageIDHeaderValues(header http.Header) []string {
	var values []string

	for key, headerValues := range header {
		if strings.EqualFold(key, irishmac.HeaderIrisMessageID) {
			values = append(values, headerValues...)
		}
	}

	return values
}

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
