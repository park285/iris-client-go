package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sync"
)

type hmacSigner struct {
	key  []byte
	pool sync.Pool
}

func newHMACSigner(secret string) *hmacSigner {
	s := &hmacSigner{key: []byte(secret)}
	s.pool.New = func() any {
		return hmac.New(sha256.New, s.key)
	}
	return s
}

func (s *hmacSigner) sign(canonical string) string {
	mac, ok := s.pool.Get().(hash.Hash)
	if !ok {
		mac = hmac.New(sha256.New, s.key)
	}
	defer s.pool.Put(mac)
	mac.Reset()
	mac.Write([]byte(canonical))
	var sumBuf [sha256.Size]byte
	sum := mac.Sum(sumBuf[:0])
	return hex.EncodeToString(sum)
}
