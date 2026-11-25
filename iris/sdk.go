package iris

import client "github.com/park285/iris-client-go/v6/internal/client/transport"

func NewClient(opts ...ClientOption) (*APIClient, error) {
	return client.NewClient(opts...)
}
