package client

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

var randomHexFallbackCounter atomic.Uint64

// signIrisCanonicalWithSigner는 Iris 요청 인증을 위한 HMAC-SHA256 서명을 계산합니다.
// 정규화 형식: "METHOD\nPATH\nTIMESTAMP\nNONCE\nSHA256(body)"
func signIrisCanonicalWithSigner(signer *hmacSigner, method, path, timestamp, nonce, bodySHA256 string) (string, error) {
	target, err := canonicalIrisTarget(path)
	if err != nil {
		return "", err
	}

	canonical := canonicalIrisRequest(
		method,
		target,
		timestamp,
		nonce,
		bodySHA256,
	)

	return signer.sign(canonical), nil
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

func canonicalIrisTarget(target string) (string, error) {
	path, rawQuery, hasQuery := strings.Cut(target, "?")
	if !hasQuery || rawQuery == "" {
		return path, nil
	}

	pairs, ok := parseCanonicalIrisQuery(rawQuery)
	if !ok {
		return "", fmt.Errorf("iris: request target has malformed percent-encoding in query")
	}
	if len(pairs) == 0 {
		return path, nil
	}

	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return compareOptionalCanonicalQueryValue(pairs[i].value, pairs[j].value) < 0
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
		if pair.value != nil {
			builder.WriteByte('=')
			builder.WriteString(*pair.value)
		}
	}
	return builder.String(), nil
}

type canonicalQueryPair struct {
	key   string
	value *string
}

func parseCanonicalIrisQuery(rawQuery string) ([]canonicalQueryPair, bool) {
	pairs := make([]canonicalQueryPair, 0, strings.Count(rawQuery, "&")+1)
	for rawPair := range strings.SplitSeq(rawQuery, "&") {
		if rawPair == "" {
			continue
		}

		rawKey, rawValue, hasValue := strings.Cut(rawPair, "=")
		key, ok := decodeIrisQueryComponentStrict(rawKey)
		if !ok {
			return nil, false
		}
		pair := canonicalQueryPair{key: encodeIrisQueryComponent(key)}
		if hasValue {
			value, ok := decodeIrisQueryComponentStrict(rawValue)
			if !ok {
				return nil, false
			}
			encodedValue := encodeIrisQueryComponent(value)
			pair.value = &encodedValue
		}
		pairs = append(pairs, pair)
	}
	return pairs, true
}

func compareOptionalCanonicalQueryValue(left, right *string) int {
	switch {
	case left == nil && right == nil:
		return 0
	case left == nil:
		return -1
	case right == nil:
		return 1
	case *left < *right:
		return -1
	case *left > *right:
		return 1
	default:
		return 0
	}
}

func decodeIrisQueryComponentStrict(value string) (string, bool) {
	var out strings.Builder
	out.Grow(len(value))
	for i := 0; i < len(value); {
		if value[i] != '%' {
			out.WriteByte(value[i])
			i++
			continue
		}
		if i+2 >= len(value) || !isHexByte(value[i+1]) || !isHexByte(value[i+2]) {
			return "", false
		}
		out.WriteByte(fromHexPair(value[i+1], value[i+2]))
		i += 3
	}
	decoded := out.String()
	if !utf8.ValidString(decoded) {
		return "", false
	}
	return decoded, true
}

func encodeIrisQueryComponent(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for i := range len(value) {
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
			fmt.Fprintf(&builder, "%%%02X", b)
		}
	}
	return builder.String()
}

func isHexByte(b byte) bool {
	return b >= '0' && b <= '9' || b >= 'a' && b <= 'f' || b >= 'A' && b <= 'F'
}

func fromHexPair(high, low byte) byte {
	return fromHexNibble(high)<<4 | fromHexNibble(low)
}

func fromHexNibble(b byte) byte {
	switch {
	case b >= '0' && b <= '9':
		return b - '0'
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10
	default:
		return b - 'A' + 10
	}
}

func generateNonce() string {
	return generateRandomHex16("iris-nonce")
}

func generateRandomHex16(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}

	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), randomHexFallbackCounter.Add(1))
}
