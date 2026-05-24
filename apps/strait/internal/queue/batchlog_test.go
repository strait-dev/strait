package queue

import (
	"testing"
	"time"
)

func TestBatchlogConfig_NormalizedDefaults(t *testing.T) {
	cfg := BatchlogConfig{}.normalized()
	if cfg.TickInterval != 100*time.Millisecond {
		t.Fatalf("TickInterval = %s, want 100ms", cfg.TickInterval)
	}
	if cfg.LeaseDuration != 30*time.Second {
		t.Fatalf("LeaseDuration = %s, want 30s", cfg.LeaseDuration)
	}
	if cfg.LeaseOwner == "" {
		t.Fatal("LeaseOwner is empty")
	}
}

func TestQueueLeaseExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	if !leaseExpired(now, &past) {
		t.Fatal("past lease should be expired")
	}
	if leaseExpired(now, &future) {
		t.Fatal("future lease should not be expired")
	}
	if leaseExpired(now, nil) {
		t.Fatal("nil lease should not be expired")
	}
}

func FuzzQueueLease(f *testing.F) {
	f.Add(int64(0), int64(-1))
	f.Add(int64(0), int64(1))
	f.Fuzz(func(t *testing.T, nowUnix, deltaMillis int64) {
		now := time.Unix(nowUnix%4102444800, 0)
		expires := now.Add(time.Duration(deltaMillis) * time.Millisecond)
		got := leaseExpired(now, &expires)
		want := deltaMillis <= 0
		if got != want {
			t.Fatalf("leaseExpired(%s, %s) = %v, want %v", now, expires, got, want)
		}
	})
}

func FuzzBatchlog(f *testing.F) {
	f.Add(int64(0), int64(100), "worker-a")
	f.Add(int64(-1), int64(-1), "")
	f.Fuzz(func(t *testing.T, tickMillis, leaseMillis int64, owner string) {
		cfg := BatchlogConfig{
			TickInterval:  time.Duration(tickMillis) * time.Millisecond,
			LeaseDuration: time.Duration(leaseMillis) * time.Millisecond,
			LeaseOwner:    owner,
		}.normalized()
		if cfg.TickInterval <= 0 {
			t.Fatalf("TickInterval = %s, want positive", cfg.TickInterval)
		}
		if cfg.LeaseDuration <= 0 {
			t.Fatalf("LeaseDuration = %s, want positive", cfg.LeaseDuration)
		}
		if cfg.LeaseOwner == "" {
			t.Fatal("LeaseOwner is empty")
		}
	})
}
