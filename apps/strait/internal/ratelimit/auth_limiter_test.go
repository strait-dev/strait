package ratelimit

import (
	"context"
	"strings"
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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

func TestDefaultAuthThresholds_Values(t *testing.T) {
	t.Parallel()

	if len(DefaultAuthThresholds) != 3 {
		t.Fatalf("expected 3 thresholds, got %d", len(DefaultAuthThresholds))
	}
	want := []AuthLimiterThreshold{
		{Failures: 50, Lockout: 15 * time.Minute},
		{Failures: 25, Lockout: 5 * time.Minute},
		{Failures: 10, Lockout: 1 * time.Minute},
	}
	for i, w := range want {
		got := DefaultAuthThresholds[i]
		if got.Failures != w.Failures {
			t.Errorf("threshold[%d].Failures = %d, want %d", i, got.Failures, w.Failures)
		}
		if got.Lockout != w.Lockout {
			t.Errorf("threshold[%d].Lockout = %v, want %v", i, got.Lockout, w.Lockout)
		}
	}
}

func TestDefaultAuthThresholds_DescendingOrder(t *testing.T) {
	t.Parallel()

	for i := 1; i < len(DefaultAuthThresholds); i++ {
		if DefaultAuthThresholds[i].Failures >= DefaultAuthThresholds[i-1].Failures {
			t.Errorf("thresholds not in descending order at index %d: %d >= %d",
				i, DefaultAuthThresholds[i].Failures, DefaultAuthThresholds[i-1].Failures)
		}
	}
}

func TestAuthLimiter_Blocked_At11(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 11 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 11 failures")
	}
	if lockout != 1*time.Minute {
		t.Errorf("lockout = %v, want 1m", lockout)
	}
}

func TestAuthLimiter_Blocked_At24_FirstTier(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 24 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 24 failures")
	}
	if lockout != 1*time.Minute {
		t.Errorf("lockout = %v, want 1m (first tier, not second)", lockout)
	}
}

func TestAuthLimiter_Blocked_At26(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 26 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 26 failures")
	}
	if lockout != 5*time.Minute {
		t.Errorf("lockout = %v, want 5m", lockout)
	}
}

func TestAuthLimiter_Blocked_At49_SecondTier(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 49 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 49 failures")
	}
	if lockout != 5*time.Minute {
		t.Errorf("lockout = %v, want 5m (second tier, not third)", lockout)
	}
}

func TestAuthLimiter_Blocked_At51(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 51 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, lockout := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked at 51 failures")
	}
	if lockout != 15*time.Minute {
		t.Errorf("lockout = %v, want 15m", lockout)
	}
}

func TestBlockedError_Format(t *testing.T) {
	t.Parallel()

	msg := BlockedError(5 * time.Minute)
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !strings.Contains(msg, "5m0s") {
		t.Errorf("expected message to contain lockout duration, got %q", msg)
	}
}

func TestAuthLimiter_TTL_Expires(t *testing.T) {
	t.Parallel()
	limiter, mr := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 15 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, _ := limiter.IsBlocked(ctx, "1.2.3.4")
	if !blocked {
		t.Fatal("should be blocked")
	}

	mr.FastForward(authFailWindow() + time.Second)

	blocked, _ = limiter.IsBlocked(ctx, "1.2.3.4")
	if blocked {
		t.Error("should not be blocked after TTL expires")
	}
}

// Regression: every RecordFailure must leave a TTL on the
// counter key. The previous implementation used a non-transactional
// Pipeline so a partial failure between INCR and PExpire could leave
// a TTL-less key, which would never expire and effectively lock out
// the IP forever. We now use TxPipelined (MULTI/EXEC).
func TestAuthLimiter_RecordFailure_AlwaysSetsTTL(t *testing.T) {
	t.Parallel()
	limiter, mr := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limiter.RecordFailure(ctx, "9.9.9.9")

	ttl := mr.TTL("auth:fail:9.9.9.9")
	if ttl <= 0 {
		t.Fatalf("RecordFailure left key without TTL: ttl=%s (key would never expire)", ttl)
	}
	if ttl > authFailWindow() {
		t.Fatalf("RecordFailure TTL %s longer than configured window %s", ttl, authFailWindow())
	}
}

func TestAuthLimiter_Reset(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// Should never panic regardless of IP format.
		limiter.RecordFailure(ctx, ip)
		limiter.IsBlocked(ctx, ip)
		limiter.Reset(ctx, ip)
	})
}
