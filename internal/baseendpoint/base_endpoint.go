package baseendpoint

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func Parse(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, errors.New("base endpoint is empty")
	}

	parsed, err := url.ParseRequestURI(trimmed)
	if err != nil {
		if strings.Contains(err.Error(), "invalid port") {
			return nil, errors.New("base endpoint port must be numeric")
		}

		return nil, errors.New("base endpoint is malformed")
	}

	if !parsed.IsAbs() || parsed.Opaque != "" {
		return nil, errors.New("base endpoint must be an absolute hierarchical URL")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("base endpoint scheme must be http or https; got %q", parsed.Scheme)
	}

	if parsed.Host == "" || parsed.Hostname() == "" {
		return nil, errors.New("base endpoint must include a host")
	}

	if parsed.User != nil {
		return nil, errors.New("base endpoint must not include userinfo")
	}

	if parsed.RawQuery != "" || parsed.ForceQuery {
		return nil, errors.New("base endpoint must not include a query")
	}

	if parsed.Fragment != "" {
		return nil, errors.New("base endpoint must not include a fragment")
	}

	normalized := strings.TrimRight(trimmed, "/")

	parsed, err = url.ParseRequestURI(normalized)
	if err != nil {
		return nil, errors.New("normalized base endpoint is malformed")
	}

	return parsed, nil
}
