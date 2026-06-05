package ratelimit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	assert.False(t, blocked)

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
	require.True(t, blocked)
	assert.Equal(t, 1*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 5*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 15*
		time.Minute,
		lockout,
	)

}

func TestDefaultAuthThresholds_Values(t *testing.T) {
	t.Parallel()
	require.Len(t, DefaultAuthThresholds,

		3)

	want := []AuthLimiterThreshold{
		{Failures: 50, Lockout: 15 * time.Minute},
		{Failures: 25, Lockout: 5 * time.Minute},
		{Failures: 10, Lockout: 1 * time.Minute},
	}
	for i, w := range want {
		got := DefaultAuthThresholds[i]
		assert.Equal(t, w.Failures,
			got.
				Failures)
		assert.Equal(t, w.Lockout,
			got.
				Lockout)

	}
}

func TestDefaultAuthThresholds_DescendingOrder(t *testing.T) {
	t.Parallel()

	for i := 1; i < len(DefaultAuthThresholds); i++ {
		assert.False(t, DefaultAuthThresholds[i].
			Failures >=
			DefaultAuthThresholds[i-1].Failures,
		)

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
	require.True(t, blocked)
	assert.Equal(t, 1*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 1*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 5*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 5*
		time.Minute,
		lockout)

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
	require.True(t, blocked)
	assert.Equal(t, 15*
		time.Minute,
		lockout,
	)

}

func TestBlockedError_Format(t *testing.T) {
	t.Parallel()

	msg := BlockedError(5 * time.Minute)
	require.NotEqual(t,
		"", msg)
	assert.True(t, strings.Contains(msg, "5m0s"))

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
	require.True(t, blocked)

	mr.FastForward(authFailWindow() + time.Second)

	blocked, _ = limiter.IsBlocked(ctx, "1.2.3.4")
	assert.False(t, blocked)

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

	ttl := mr.TTL(authFailKey(AuthScopeAPIKey, "9.9.9.9"))
	require.False(t, ttl <=
		0)
	require.LessOrEqual(t, ttl, authFailWindow())

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
	assert.False(t, blocked)

}

func TestAuthLimiter_LockoutExpiresBeforeFailureWindow(t *testing.T) {
	t.Parallel()
	limiter, mr := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for range 10 {
		limiter.RecordFailure(ctx, "1.2.3.4")
	}

	blocked, retryAfter := limiter.IsBlocked(ctx, "1.2.3.4")
	require.True(t, blocked)
	require.Equal(t, time.
		Minute,
		retryAfter)

	mr.FastForward(time.Minute + time.Second)

	blocked, _ = limiter.IsBlocked(ctx, "1.2.3.4")
	require.False(t, blocked)
	require.True(t, mr.
		Exists(authFailKey(AuthScopeAPIKey,

			"1.2.3.4")))

}

func TestAuthLimiter_ScopedResetDoesNotClearOtherAuthSchemes(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ip := "1.2.3.4"

	for range 10 {
		limiter.RecordFailureScoped(ctx, ip, AuthScopeOIDC)
	}
	limiter.ResetScoped(ctx, ip, AuthScopeAPIKey)

	blocked, retryAfter := limiter.IsBlockedScoped(ctx, ip, AuthScopeOIDC)
	require.True(t, blocked)
	require.Equal(t, time.
		Minute,
		retryAfter)

	apiBlocked, _ := limiter.IsBlockedScoped(ctx, ip, AuthScopeAPIKey)
	require.False(t, apiBlocked)

}

func TestAuthLimiter_ProfilingScopeIsIsolatedFromInternalSecret(t *testing.T) {
	t.Parallel()
	limiter, _ := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ip := "1.2.3.4"

	for range 10 {
		limiter.RecordFailureScoped(ctx, ip, AuthScopeProfiling)
	}
	limiter.ResetScoped(ctx, ip, AuthScopeInternalSecret)

	profilingBlocked, retryAfter := limiter.IsBlockedScoped(ctx, ip, AuthScopeProfiling)
	require.True(t, profilingBlocked)
	require.Equal(t, time.
		Minute,
		retryAfter)

	internalBlocked, _ := limiter.IsBlockedScoped(ctx, ip, AuthScopeInternalSecret)
	require.False(t, internalBlocked)

	limiter.ResetScoped(ctx, ip, AuthScopeProfiling)
	for range 10 {
		limiter.RecordFailureScoped(ctx, ip, AuthScopeInternalSecret)
	}

	profilingBlocked, _ = limiter.IsBlockedScoped(ctx, ip, AuthScopeProfiling)
	require.False(t, profilingBlocked)

	internalBlocked, _ = limiter.IsBlockedScoped(ctx, ip, AuthScopeInternalSecret)
	require.True(t, internalBlocked)

}

func TestAuthLimiter_HigherTierReachableAfterShortLockoutExpires(t *testing.T) {
	t.Parallel()
	limiter, mr := newTestAuthLimiter(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ip := "1.2.3.4"

	for range 10 {
		limiter.RecordFailure(ctx, ip)
	}
	mr.FastForward(time.Minute + time.Second)

	for range 15 {
		limiter.RecordFailure(ctx, ip)
	}

	blocked, retryAfter := limiter.IsBlocked(ctx, ip)
	require.True(t, blocked)
	require.Equal(t, 5*
		time.Minute,
		retryAfter,
	)

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
	assert.False(t, blocked)

}

func TestAuthLimiter_Nil_FailsOpen(t *testing.T) {
	t.Parallel()

	var limiter *AuthLimiter
	blocked, _ := limiter.IsBlocked(context.Background(), "1.2.3.4")
	assert.False(t, blocked)

	// Should not panic.
	limiter.RecordFailure(context.Background(), "1.2.3.4")
	limiter.Reset(context.Background(), "1.2.3.4")
}

func TestAuthLimiter_Disabled_FailsOpen(t *testing.T) {
	t.Parallel()

	limiter := NewAuthLimiter(nil, false)
	blocked, _ := limiter.IsBlocked(context.Background(), "1.2.3.4")
	assert.False(t, blocked)

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
