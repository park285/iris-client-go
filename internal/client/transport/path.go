package transport

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/park285/iris-client-go/internal/irishmac"
)

const maxPathSegmentTokenBytes = 160

func appendSafePathSegment(basePath, label, value string) (string, error) {
	segment, err := safePathSegmentToken(label, value)
	if err != nil {
		return "", err
	}

	return basePath + "/" + segment, nil
}

func safePathSegmentToken(label, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("iris: %s must not be blank", label)
	}
	if len(trimmed) > maxPathSegmentTokenBytes {
		return "", fmt.Errorf("iris: %s must be <= %d ASCII bytes", label, maxPathSegmentTokenBytes)
	}
	if trimmed == "." || trimmed == ".." {
		return "", fmt.Errorf("iris: %s must not be a dot segment", label)
	}

	for i := range len(trimmed) {
		if !isSafePathSegmentTokenByte(trimmed[i]) {
			return "", fmt.Errorf("iris: %s must use only [A-Za-z0-9._:-]", label)
		}
	}

	return trimmed, nil
}

func isSafePathSegmentTokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' ||
		b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' ||
		b == '.' || b == '_' || b == ':' || b == '-'
}

func appendCanonicalQuery(path string, params url.Values) string {
	encoded := canonicalQueryString(params)
	if encoded == "" {
		return path
	}

	return path + "?" + encoded
}

func canonicalQueryString(params url.Values) string {
	if len(params) == 0 {
		return ""
	}

	pairs := make([]string, 0, len(params))
	for key, values := range params {
		encodedKey := irishmac.EncodeQueryComponent(key)
		if len(values) == 0 {
			pairs = append(pairs, encodedKey)
			continue
		}
		for _, value := range values {
			pairs = append(pairs, encodedKey+"="+irishmac.EncodeQueryComponent(value))
		}
	}
	sort.Strings(pairs)

	return strings.Join(pairs, "&")
}
