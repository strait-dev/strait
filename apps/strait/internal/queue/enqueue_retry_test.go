package queue

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/stretchr/testify/require"
)

type retryTestQueue struct {
	enqueueFn func(context.Context, *domain.JobRun) error
}

func (q retryTestQueue) Enqueue(ctx context.Context, run *domain.JobRun) error {
	if q.enqueueFn != nil {
		return q.enqueueFn(ctx, run)
	}
	return nil
}

func TestDefaultInternalEnqueueRetryConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultInternalEnqueueRetryConfig()
	require.Equal(t, 1500*time.Millisecond, cfg.MaxElapsed)
	require.Equal(t, 50*time.Millisecond, cfg.BaseDelay)
	require.Equal(t, 250*time.Millisecond, cfg.MaxDelay)
	require.InDelta(t, 0.25, cfg.JitterFrac, 0)
}

func TestEnqueueWithRetry_SucceedsAfterThrottle(t *testing.T) {
	t.Parallel()

	attempts := 0
	sleeps := 0
	err := EnqueueWithRetry(context.Background(), retryTestQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			attempts++
			if attempts < 3 {
				return &ThrottledError{ProjectID: "proj", RetryAfter: 25 * time.Millisecond}
			}
			return nil
		},
	}, &domain.JobRun{}, EnqueueRetryConfig{
		MaxElapsed: time.Second,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   50 * time.Millisecond,
		JitterFrac: 0,
		sleep: func(context.Context, time.Duration) error {
			sleeps++
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, 3, attempts)
	require.Equal(t, 2, sleeps)
}

func TestEnqueueWithRetry_StopsOnNonThrottle(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	err := EnqueueWithRetry(context.Background(), retryTestQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			return wantErr
		},
	}, &domain.JobRun{}, EnqueueRetryConfig{
		MaxElapsed: time.Second,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		JitterFrac: 0,
		sleep: func(context.Context, time.Duration) error {
			require.Fail(t,

				"sleep should not be called for non-throttle errors")
			return nil
		},
	})
	require.ErrorIs(t,
		err, wantErr)
}

func TestEnqueueWithRetry_ReturnsThrottleWhenBudgetExceeded(t *testing.T) {
	t.Parallel()

	attempts := 0
	err := EnqueueWithRetry(context.Background(), retryTestQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			attempts++
			return &ThrottledError{ProjectID: "proj", RetryAfter: 40 * time.Millisecond}
		},
	}, &domain.JobRun{}, EnqueueRetryConfig{
		MaxElapsed: 30 * time.Millisecond,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		JitterFrac: 0,
		sleep: func(context.Context, time.Duration) error {
			require.Fail(t,

				"sleep should not run once budget is exceeded")
			return nil
		},
	})
	require.ErrorIs(t,
		err, ErrEnqueueThrottled)
	require.Equal(t, 1, attempts)
}

func TestEnqueueWithRetry_StopsWhenContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	attempts := 0
	sleeps := 0

	err := EnqueueWithRetry(ctx, retryTestQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			attempts++
			return &ThrottledError{ProjectID: "proj", RetryAfter: 25 * time.Millisecond}
		},
	}, &domain.JobRun{}, EnqueueRetryConfig{
		MaxElapsed: time.Second,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   25 * time.Millisecond,
		JitterFrac: 0,
		sleep: func(ctx context.Context, _ time.Duration) error {
			sleeps++
			cancel()
			<-ctx.Done()
			return ctx.Err()
		},
	})
	require.ErrorIs(t,
		err, context.
			Canceled)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, sleeps)
}

func TestBackpressureRetryDelayJitter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		delay     time.Duration
		jitter    float64
		randFloat float64
		want      time.Duration
	}{
		{
			name:      "minimum_offset",
			delay:     100 * time.Millisecond,
			jitter:    0.25,
			randFloat: 0,
			want:      75 * time.Millisecond,
		},
		{
			name:      "maximum_offset",
			delay:     100 * time.Millisecond,
			jitter:    0.25,
			randFloat: 1,
			want:      125 * time.Millisecond,
		},
		{
			name:      "rounds_fractional_offset",
			delay:     3 * time.Nanosecond,
			jitter:    0.5,
			randFloat: 1,
			want:      5 * time.Nanosecond,
		},
		{
			name:      "negative_clamp",
			delay:     time.Nanosecond,
			jitter:    math.Nextafter(1, 2),
			randFloat: 0,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := backpressureRetryDelay(&ThrottledError{ProjectID: "proj", RetryAfter: tt.delay}, 0, EnqueueRetryConfig{
				BaseDelay:  tt.delay,
				MaxDelay:   tt.delay,
				JitterFrac: tt.jitter,
				randFloat:  func() float64 { return tt.randFloat },
			})
			require.Equal(t, tt.want, got)
		})
	}
}

func TestBackpressureRetryDelayCapsExponentialDelay(t *testing.T) {
	t.Parallel()

	got := backpressureRetryDelay(ErrEnqueueThrottled, 12, EnqueueRetryConfig{
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   250 * time.Millisecond,
		JitterFrac: 0,
	})
	require.Equal(t, 250*time.Millisecond, got)
}

func TestEnqueueWithRetry_StopsWhenContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Millisecond))
	defer cancel()

	attempts := 0
	sleeps := 0
	err := EnqueueWithRetry(ctx, retryTestQueue{
		enqueueFn: func(context.Context, *domain.JobRun) error {
			attempts++
			return &ThrottledError{ProjectID: "proj", RetryAfter: 25 * time.Millisecond}
		},
	}, &domain.JobRun{}, EnqueueRetryConfig{
		MaxElapsed: time.Second,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   25 * time.Millisecond,
		JitterFrac: 0,
		sleep: func(ctx context.Context, _ time.Duration) error {
			sleeps++
			return ctx.Err()
		},
	})
	require.ErrorIs(t,
		err, context.
			DeadlineExceeded)
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, sleeps)
}
