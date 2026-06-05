package queue

import (
	"context"
	"errors"
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
	require.EqualValues(t, 3, attempts)
	require.EqualValues(t, 2, sleeps)

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
	require.True(t,
		errors.Is(err, wantErr))

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
	require.True(t,
		errors.Is(err, ErrEnqueueThrottled))
	require.EqualValues(t, 1, attempts)

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
	require.True(t,
		errors.Is(err, context.
			Canceled,
		))
	require.EqualValues(t, 1, attempts)
	require.EqualValues(t, 1, sleeps)

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
	require.True(t,
		errors.Is(err, context.
			DeadlineExceeded,
		))
	require.EqualValues(t, 1, attempts)
	require.EqualValues(t, 1, sleeps)

}
