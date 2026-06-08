package webhook

import "testing"

func TestSchedulerShardIndexStableAndBounded(t *testing.T) {
	t.Parallel()

	for _, key := range []string{"room", "room:thread", "다국어-room:123"} {
		first := schedulerShardIndex(key, 32)
		for range 100 {
			got := schedulerShardIndex(key, 32)
			if got != first {
				t.Fatalf("schedulerShardIndex(%q) changed from %d to %d", key, first, got)
			}
			if got < 0 || got >= 32 {
				t.Fatalf("schedulerShardIndex(%q) = %d, want [0,32)", key, got)
			}
		}
	}
}

func TestSchedulerShardIndexEmptyKeyUsesShardZero(t *testing.T) {
	t.Parallel()

	if got := schedulerShardIndex("", 32); got != 0 {
		t.Fatalf("schedulerShardIndex(empty) = %d, want 0", got)
	}
}

func TestSchedulerShardIndexAllocFree(t *testing.T) {
	allocs := testing.AllocsPerRun(1000, func() {
		_ = schedulerShardIndex("room:thread", 64)
	})
	if allocs != 0 {
		t.Fatalf("schedulerShardIndex allocs/run = %f, want 0", allocs)
	}
}

func BenchmarkSchedulerShardIndex(b *testing.B) {
	keys := []string{
		"room-a:thread-1",
		"room-b:thread-2",
		"room-c:thread-3",
		"room-d",
	}

	b.ReportAllocs()
	for b.Loop() {
		for _, key := range keys {
			_ = schedulerShardIndex(key, 64)
		}
	}
}
