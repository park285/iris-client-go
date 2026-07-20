package webhook

import (
	"context"
	"testing"
	"time"
)

func TestMemoryNonceCacheRejectsReuseWhileSweepIsDeferred(t *testing.T) {
	cache := newMemoryNonceCache()
	now := time.Now()
	cache.lastSweep = now
	cache.entries["nonce"] = now.Add(time.Hour)
	cache.entries["expired"] = now.Add(-time.Hour)

	duplicate, err := cache.IsDuplicate(context.Background(), "nonce", 4*time.Hour)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if !duplicate {
		t.Fatal("IsDuplicate() = false, want true for an unexpired nonce")
	}
	if _, ok := cache.entries["expired"]; !ok {
		t.Fatal("expired entry was swept before the amortization interval")
	}
}

func TestMemoryNonceCacheSweepsAfterAmortizationInterval(t *testing.T) {
	cache := newMemoryNonceCache()
	now := time.Now()
	cache.lastSweep = now.Add(-2 * time.Hour)
	cache.entries["expired"] = now.Add(-time.Hour)

	duplicate, err := cache.IsDuplicate(context.Background(), "fresh", 4*time.Hour)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if duplicate {
		t.Fatal("IsDuplicate() = true, want false for a fresh nonce")
	}
	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expired entry remained after the amortization interval")
	}
}

func TestMemoryNonceCacheSweepsAtExactAmortizationInterval(t *testing.T) {
	cache := newMemoryNonceCache()
	now := time.Unix(1_000, 0)
	ttl := 4 * time.Hour
	cache.now = func() time.Time { return now }
	cache.lastSweep = now.Add(-ttl / 4)
	cache.entries["expired"] = now.Add(-time.Nanosecond)

	duplicate, err := cache.IsDuplicate(context.Background(), "fresh", ttl)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if duplicate {
		t.Fatal("IsDuplicate() = true, want false for a fresh nonce")
	}
	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expired entry remained at the exact amortization interval")
	}
}

func TestMemoryNonceCacheReplacesEntryAtExactExpiry(t *testing.T) {
	cache := newMemoryNonceCache()
	now := time.Unix(1_000, 0)
	ttl := 4 * time.Hour
	cache.now = func() time.Time { return now }
	cache.lastSweep = now
	cache.entries["nonce"] = now

	duplicate, err := cache.IsDuplicate(context.Background(), "nonce", ttl)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if duplicate {
		t.Fatal("IsDuplicate() = true, want false at exact expiry")
	}
	if got := cache.entries["nonce"]; !got.Equal(now.Add(ttl)) {
		t.Fatalf("replacement expiry = %s, want %s", got, now.Add(ttl))
	}
}

func TestMemoryNonceCacheReplacesExpiredTargetWhileSweepIsDeferred(t *testing.T) {
	cache := newMemoryNonceCache()
	now := time.Now()
	cache.lastSweep = now
	cache.entries["nonce"] = now.Add(-time.Hour)

	duplicate, err := cache.IsDuplicate(context.Background(), "nonce", 4*time.Hour)
	if err != nil {
		t.Fatalf("IsDuplicate() error = %v", err)
	}
	if duplicate {
		t.Fatal("IsDuplicate() = true, want false for an expired nonce reservation")
	}
	if !cache.entries["nonce"].After(now) {
		t.Fatalf("replacement expiry = %s, want after %s", cache.entries["nonce"], now)
	}
}
