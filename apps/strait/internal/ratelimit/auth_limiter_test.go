package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestAuthLimiter(t *testing.T) (*AuthLimiter, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return NewAuthLimiter(client, true), mr
}

func TestAuthLimiter_NotBlocked_BelowThreshold(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 9 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, _ := limiter.IsBlocked(ctx, "1.2.3.4")
	if blocked {
		t.Error("should not be blocked with 9 failures (threshold is 10)")
	}
}

func TestAuthLimiter_Blocked_At10(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 10 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 10 failures")
	}
	if lockout != 1*time.Minute {
		t.Errorf("lockout = %v, want 1m", lockout)
	}
}

func TestAuthLimiter_Blocked_At25(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 25 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 25 failures")
	}
	if lockout != 5*time.Minute {
		t.Errorf("lockout = %v, want 5m", lockout)
	}
}

func TestAuthLimiter_Blocked_At50(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 50 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 50 failures")
	}
	if lockout != 15*time.Minute {
		t.Errorf("lockout = %v, want 15m", lockout)
	}
}

func TestAuthLimiter_TTL_Expires(t *testing.T) {
	t.Parallel()
	limiter, mr := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 15 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, _ := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked")
	}

	mr.FastForward(authFailWindowTTL + time.Second)

	blocked, _ = limiter.IsBlocked(ctx, "1.2.3.4")
	if blocked {
		t.Error("should not be blocked after TTL expires")
	}
}

func TestAuthLimiter_Reset(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 15 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}
	limiter.Reset(ctx, "1.2.3.4")

	blocked, _ := limiter.IsBlocked(ctx, "1.2.3.4")
	if blocked {
		t.Error("should not be blocked after reset")
	}
}

func TestAuthLimiter_IndependentIPs(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx := context.Background()

	for range 15 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, _ := limiter.IsBlocked(ctx, "5.6.7.8")
	if blocked {
		t.Error("different IP should not be blocked")
	}
}

func TestAuthLimiter_Nil_FailsOpen(t *testing.T) {
	t.Parallel()

	var limiter *AuthLimiter
	blocked, _ := limiter.IsBlocked(context.Background(), "1.2.3.4")
	if blocked {
		t.Error("nil limiter should not block")
	}
	// Should not panic.
	limiter.RecordFailure(context.Background(), "1.2.3.4")
	limiter.Reset(context.Background(), "1.2.3.4")
}

func TestAuthLimiter_Disabled_FailsOpen(t *testing.T) {
	t.Parallel()

	limiter := NewAuthLimiter(nil, false)
	blocked, _ := limiter.IsBlocked(context.Background(), "1.2.3.4")
	if blocked {
		t.Error("disabled limiter should not block")
	}
}

func FuzzAuthLimiter_IP(f *testing.F) {
	f.Add("192.168.1.1")
	f.Add("::1")
	f.Add("")
	f.Add("10.0.0.1:8080")
	f.Add("2001:db8::1")

	mr := miniredis.RunT(f)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	f.Cleanup(func() { client.Close() })
	limiter := NewAuthLimiter(client, true)

	f.Fuzz(func(t *testing.T, ip string) {
		ctx := context.Background()
		// Should never panic regardless of IP format.
		limiter.RecordFailure(ctx, ip)
		limiter.IsBlocked(ctx, ip)
		limiter.Reset(ctx, ip)
	})
}
