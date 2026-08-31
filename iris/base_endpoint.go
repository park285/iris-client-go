package iris

import (
	"fmt"
	"net/url"

	"github.com/park285/iris-client-go/v2/internal/baseendpoint"
)

// ParseBaseEndpoint validates and normalizes an Iris API deployment endpoint.
func ParseBaseEndpoint(raw string) (*url.URL, error) {
	endpoint, err := baseendpoint.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse Iris base endpoint: %w", err)
	}

	return endpoint, nil
}
