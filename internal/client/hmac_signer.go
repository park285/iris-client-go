package client

import (
	"hash"
	"sync"

	"github.com/park285/iris-client-go/internal/irishmac"
)

type hmacSigner struct {
	key  []byte
	pool sync.Pool
}

func newHMACSigner(secret string) *hmacSigner {
	s := &hmacSigner{key: []byte(secret)}
	s.pool.New = func() any {
		return irishmac.NewMAC(s.key)
	}
	return s
}

func (s *hmacSigner) sign(canonical string) string {
	mac, ok := s.pool.Get().(hash.Hash)
	if !ok {
		mac = irishmac.NewMAC(s.key)
	}
	defer s.pool.Put(mac)
	return irishmac.SignHash(mac, canonical)
}
