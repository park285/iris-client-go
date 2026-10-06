package iris

import (
	"fmt"
	"net/url"

	"github.com/park285/iris-client-go/v3/internal/baseendpoint"
)

// ParseBaseEndpoint는 Iris API 배포 주소를 검증하고 정규화한다.
func ParseBaseEndpoint(raw string) (*url.URL, error) {
	endpoint, err := baseendpoint.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse Iris base endpoint: %w", err)
	}

	return endpoint, nil
}
