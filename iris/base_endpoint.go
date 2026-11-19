package iris

import (
	"fmt"
	"net/url"

	"github.com/park285/iris-client-go/v5/internal/baseendpoint"
)

func ParseBaseEndpoint(raw string) (*url.URL, error) {
	endpoint, err := baseendpoint.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse Iris base endpoint: %w", err)
	}

	return endpoint, nil
}
