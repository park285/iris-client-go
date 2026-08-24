package irishmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/netip"
	"strconv"
	"strings"
	"sync"
)

const EmptyBodySHA256Hex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

const (
	HeaderIrisTimestamp        = "X-Iris-Timestamp"
	HeaderIrisNonce            = "X-Iris-Nonce"
	HeaderIrisSignature        = "X-Iris-Signature"
	HeaderIrisBodySHA256       = "X-Iris-Body-Sha256"
	HeaderIrisMessageID        = "X-Iris-Message-Id"
	HeaderIrisSignatureVersion = "X-Iris-Signature-Version"
	SignatureVersionV3         = "v3"
)

type Signer struct {
	key  []byte
	pool sync.Pool
}

func NewSigner(secret string) *Signer {
	s := &Signer{key: []byte(secret)}

	s.pool.New = func() any {
		return NewMAC(s.key)
	}

	return s
}

func NewMAC(key []byte) hash.Hash {
	return hmac.New(sha256.New, key)
}

func (s *Signer) Sign(canonical string) string {
	mac, ok := s.pool.Get().(hash.Hash)
	if !ok {
		mac = NewMAC(s.key)
	}

	defer s.pool.Put(mac)

	return SignHash(mac, canonical)
}

func SignHash(mac hash.Hash, canonical string) string {
	mac.Reset()
	mac.Write([]byte(canonical))

	var sumBuf [sha256.Size]byte

	sum := mac.Sum(sumBuf[:0])

	return hex.EncodeToString(sum)
}

func SignCanonical(signer *Signer, method, target, timestamp, nonce, bodySHA256 string) (string, error) {
	canonicalTarget, err := CanonicalTarget(target)
	if err != nil {
		return "", fmt.Errorf("canonical target: %w", err)
	}

	canonical := CanonicalRequest(method, canonicalTarget, timestamp, nonce, bodySHA256)

	return signer.Sign(canonical), nil
}

// authority는 설정 URL 원문이 아니라 실제 전송될 Host authority여야 한다. 이 함수에는
// scheme 정보가 없으므로 명시 포트는 보존하며, IDN은 URL parser가 만든 ASCII A-label만 받는다.
func CanonicalWebhookRequestV3(authority, method, target, timestamp, nonce, messageID, bodySHA256 string) (string, error) {
	canonicalAuthority, err := canonicalWebhookAuthority(authority)
	if err != nil {
		return "", fmt.Errorf("canonical authority: %w", err)
	}

	return strings.Join([]string{
		SignatureVersionV3,
		canonicalAuthority,
		strings.ToUpper(method),
		target,
		timestamp,
		nonce,
		messageID,
		strings.ToLower(bodySHA256),
	}, "\n"), nil
}

func canonicalWebhookAuthority(authority string) (string, error) {
	if authority == "" || strings.TrimSpace(authority) != authority {
		return "", errors.New("iris: webhook authority is empty or carries surrounding whitespace")
	}

	host, port, hasPort, bracketed, err := splitWebhookAuthority(authority)
	if err != nil {
		return "", fmt.Errorf("split authority: %w", err)
	}

	host, err = canonicalWebhookHost(host, bracketed)
	if err != nil {
		return "", err //nolint:wrapcheck // 검증 오류가 필드 맥락을 이미 담고 있어 그대로 전달한다.
	}

	if !hasPort {
		return host, nil
	}

	portNumber, parseErr := strconv.ParseUint(port, 10, 16)
	if parseErr != nil {
		return "", errors.New("iris: webhook authority has an invalid port")
	}

	return host + ":" + strconv.FormatUint(portNumber, 10), nil
}

func splitWebhookAuthority(authority string) (host, port string, hasPort, bracketed bool, err error) {
	if strings.HasPrefix(authority, "[") {
		closingBracket := strings.IndexByte(authority, ']')
		if closingBracket <= 1 {
			return "", "", false, false, errors.New("iris: webhook authority has an invalid bracketed host")
		}

		host = authority[1:closingBracket]

		suffix := authority[closingBracket+1:]

		switch {
		case suffix == "":
			return host, "", false, true, nil
		case strings.HasPrefix(suffix, ":") && len(suffix) > 1:
			return host, suffix[1:], true, true, nil
		default:
			return "", "", false, false, errors.New("iris: webhook authority has an invalid bracketed host suffix")
		}
	}

	if strings.ContainsAny(authority, "[]") {
		return "", "", false, false, errors.New("iris: webhook authority has unmatched brackets")
	}

	switch strings.Count(authority, ":") {
	case 0:
		host = authority
	case 1:
		host, port, _ = strings.Cut(authority, ":")
		hasPort = true
	default:
		return "", "", false, false, errors.New("iris: webhook authority IPv6 host must be bracketed")
	}

	if host == "" || hasPort && port == "" {
		return "", "", false, false, errors.New("iris: webhook authority has an empty host or port")
	}

	return host, port, hasPort, false, nil
}

func canonicalWebhookHost(host string, bracketed bool) (string, error) {
	if bracketed {
		address, parseErr := netip.ParseAddr(host)
		if parseErr != nil || !address.Is6() || address.Zone() != "" {
			return "", errors.New("iris: webhook authority has an invalid IPv6 host")
		}

		return "[" + address.String() + "]", nil
	}

	if address, parseErr := netip.ParseAddr(host); parseErr == nil {
		if address.Is6() {
			return "", errors.New("iris: webhook authority IPv6 host must be bracketed")
		}

		return address.String(), nil
	}

	if !validWebhookAuthorityHostname(host) {
		return "", errors.New("iris: webhook authority host must be an ASCII hostname or IP literal")
	}

	return strings.ToLower(host), nil
}

func validWebhookAuthorityHostname(host string) bool {
	if host == "" {
		return false
	}

	for index := range len(host) {
		character := host[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '.' || character == '_' {
			continue
		}

		return false
	}

	return true
}

func SHA256HexBytes(body []byte) string {
	if len(body) == 0 {
		return EmptyBodySHA256Hex
	}

	digest := sha256.Sum256(body)

	return hex.EncodeToString(digest[:])
}
