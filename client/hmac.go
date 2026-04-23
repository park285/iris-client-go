package client

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// signIrisRequest는 Iris 요청 인증을 위한 HMAC-SHA256 서명을 계산합니다.
// 정규화 형식: "METHOD\nPATH\nTIMESTAMP\nNONCE\nSHA256(body)"
func signIrisRequest(secret, method, path, timestamp, nonce, body string) string {
	bodyHash := sha256.Sum256([]byte(body))
	canonical := canonicalIrisRequest(
		method,
		canonicalIrisTarget(path),
		timestamp,
		nonce,
		hex.EncodeToString(bodyHash[:]),
	)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

func canonicalIrisRequest(method, target, timestamp, nonce, bodySHA256 string) string {
	return strings.Join([]string{
		strings.ToUpper(method),
		target,
		timestamp,
		nonce,
		bodySHA256,
	}, "\n")
}

func canonicalIrisTarget(target string) string {
	path, rawQuery, hasQuery := strings.Cut(target, "?")
	if !hasQuery || rawQuery == "" {
		return path
	}

	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return target
	}

	type pair struct {
		key   string
		value string
	}

	pairs := make([]pair, 0, len(query))
	for key, values := range query {
		encodedKey := encodeIrisQueryComponent(key)
		for _, value := range values {
			pairs = append(pairs, pair{
				key:   encodedKey,
				value: encodeIrisQueryComponent(value),
			})
		}
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})

	var builder strings.Builder
	builder.Grow(len(path) + len(rawQuery) + 1)
	builder.WriteString(path)
	builder.WriteByte('?')
	for i, pair := range pairs {
		if i > 0 {
			builder.WriteByte('&')
		}
		builder.WriteString(pair.key)
		builder.WriteByte('=')
		builder.WriteString(pair.value)
	}
	return builder.String()
}

func encodeIrisQueryComponent(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for i := 0; i < len(value); i++ {
		b := value[i]
		switch {
		case b >= 'A' && b <= 'Z':
			builder.WriteByte(b)
		case b >= 'a' && b <= 'z':
			builder.WriteByte(b)
		case b >= '0' && b <= '9':
			builder.WriteByte(b)
		case b == '-', b == '.', b == '_', b == '~':
			builder.WriteByte(b)
		default:
			builder.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return builder.String()
}

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
