package randomhex

import (
	"crypto/rand"
	"encoding/hex"
)

func Generate() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
