package randomhex

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var fallbackCounter atomic.Uint64

func Generate(prefix string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}

	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), fallbackCounter.Add(1))
}
