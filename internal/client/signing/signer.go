package signing

import (
	"hash"
	"sync"

	"github.com/park285/iris-client-go/internal/irishmac"
)

type HMACSigner struct {
	key  []byte
	pool sync.Pool
}

func NewHMACSigner(secret string) *HMACSigner {
	s := &HMACSigner{key: []byte(secret)}
	s.pool.New = func() any {
		return irishmac.NewMAC(s.key)
	}
	return s
}

func (s *HMACSigner) Sign(canonical string) string {
	mac, ok := s.pool.Get().(hash.Hash)
	if !ok {
		mac = irishmac.NewMAC(s.key)
	}
	defer s.pool.Put(mac)
	return irishmac.SignHash(mac, canonical)
}
