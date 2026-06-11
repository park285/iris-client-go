package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"sync"
)

type hmacSigner struct {
	pool sync.Pool
}

func newHMACSigner(secret string) *hmacSigner {
	key := []byte(secret)
	s := &hmacSigner{}
	s.pool.New = func() any {
		return hmac.New(sha256.New, key)
	}
	return s
}

func (s *hmacSigner) sign(canonical string) string {
	mac := s.pool.Get().(hash.Hash)
	mac.Reset()
	mac.Write([]byte(canonical))
	var sumBuf [sha256.Size]byte
	sum := mac.Sum(sumBuf[:0])
	s.pool.Put(mac)
	return hex.EncodeToString(sum)
}
