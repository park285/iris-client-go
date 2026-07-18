package irishmac

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
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
	SignatureVersionV2         = "v2"
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
		return "", err
	}
	canonical := CanonicalRequest(method, canonicalTarget, timestamp, nonce, bodySHA256)
	return signer.Sign(canonical), nil
}

func CanonicalWebhookRequestV2(method, target, timestamp, nonce, messageID, bodySHA256 string) string {
	return strings.Join([]string{
		SignatureVersionV2,
		strings.ToUpper(method),
		target,
		timestamp,
		nonce,
		messageID,
		strings.ToLower(bodySHA256),
	}, "\n")
}

func SHA256HexBytes(body []byte) string {
	if len(body) == 0 {
		return EmptyBodySHA256Hex
	}

	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
