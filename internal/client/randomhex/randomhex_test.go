package randomhex

import (
	"regexp"
	"testing"
)

var hex32 = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestGenerateShape(t *testing.T) {
	got := Generate()
	if !hex32.MatchString(got) {
		t.Fatalf("Generate() = %q, want 32 lowercase hex chars", got)
	}
}

func TestGenerateDiffers(t *testing.T) {
	a, b := Generate(), Generate()
	if a == b {
		t.Fatalf("Generate() returned the same value twice: %q", a)
	}
}
