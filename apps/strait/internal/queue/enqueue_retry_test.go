package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"strait/internal/domain"
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
	if err != nil {
		t.Fatalf("EnqueueWithRetry() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	if sleeps != 2 {
		t.Fatalf("sleeps = %d, want 2", sleeps)
	}
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
			t.Fatal("sleep should not be called for non-throttle errors")
			return nil
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnqueueWithRetry() error = %v, want %v", err, wantErr)
	}
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
			t.Fatal("sleep should not run once budget is exceeded")
			return nil
		},
	})
	if !errors.Is(err, ErrEnqueueThrottled) {
		t.Fatalf("EnqueueWithRetry() error = %v, want throttle", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}
