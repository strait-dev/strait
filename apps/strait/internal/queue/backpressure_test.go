package queue

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestThrottledError_Unwrap(t *testing.T) {
	e := &ThrottledError{ProjectID: "p", RetryAfter: 0}
	assert.ErrorIs(t, e, ErrEnqueueThrottled)
}

func TestAsThrottled_Positive(t *testing.T) {
	err := &ThrottledError{ProjectID: "p"}
	throttled, ok := AsThrottled(err)
	require.True(t, ok)
	require.Equal(t, "p", throttled.ProjectID)
}

func TestAsThrottled_Negative(t *testing.T) {
	if _, ok := AsThrottled(errors.New("other")); ok {
		assert.Fail(t, "non-throttled should not match")
	}
}

func TestBackpressure_NilSafeAndDisabled(t *testing.T) {
	var b *Backpressure
	require.NoError(t, b.TryConsume(context.Background(), "p"))
	require.NoError(t, b.TryConsumeN(context.Background(), "p", 3))

	b2 := NewBackpressure(nil, BackpressureConfig{}, false)
	require.NoError(t, b2.TryConsume(context.Background(), "p"))
}

func TestBackpressure_EmptyProjectID(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	require.NoError(t, b.TryConsume(context.Background(), ""))
}

func TestBackpressure_SampleAvailableTokensNoOpGuards(t *testing.T) {
	t.Parallel()

	var b *Backpressure
	samples, err := b.SampleAvailableTokens(context.Background(), 10)
	require.NoError(t, err)
	require.Nil(t, samples)

	disabled := NewBackpressure(nil, BackpressureConfig{}, false)
	samples, err = disabled.SampleAvailableTokens(context.Background(), 10)
	require.NoError(t, err)
	require.Nil(t, samples)

	enabled := NewBackpressure(nil, BackpressureConfig{}, true)
	samples, err = enabled.SampleAvailableTokens(context.Background(), 0)
	require.NoError(t, err)
	require.Nil(t, samples)
}

func TestBackpressure_DefaultConfig(t *testing.T) {
	b := NewBackpressure(nil, BackpressureConfig{}, true)
	assert.Equal(t, 1000, b.cfg.DefaultMaxTokens)
	assert.Equal(t, 100, b.cfg.DefaultRefillPerSec)
	assert.Equal(t, 32, b.cfg.LocalLeaseSize)
}

func TestBackpressure_LocalLeaseReducesDBConsumes(t *testing.T) {
	t.Parallel()

	var queryRowCalls int
	db := &mockDBTX{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			queryRowCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*int)) = 0
				*(dest[1].(*int)) = 10
				*(dest[2].(*int)) = 1
				return nil
			}}
		},
	}
	b := NewBackpressure(db, BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 1,
		LocalLeaseSize:      3,
	}, true)

	for range 3 {
		require.NoError(t, b.TryConsume(context.Background(), "project-lease"))
	}
	require.Equal(t, 1, queryRowCalls)

	require.NoError(t, b.TryConsume(context.Background(), "project-lease"))
	require.Equal(t, 2, queryRowCalls)
}

func TestBackpressure_LocalLeaseSerializesConcurrentRefill(t *testing.T) {
	t.Parallel()

	var queryRowCalls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	db := &mockDBTX{
		queryRowFn: func(ctx context.Context, _ string, _ ...any) pgx.Row {
			if queryRowCalls.Add(1) == 1 {
				close(started)
			}
			select {
			case <-ctx.Done():
				return &mockRow{scanFn: func(...any) error { return ctx.Err() }}
			case <-release:
				return &mockRow{scanFn: func(dest ...any) error {
					*(dest[0].(*int)) = 0
					*(dest[1].(*int)) = 100
					*(dest[2].(*int)) = 100
					return nil
				}}
			}
		},
	}
	b := NewBackpressure(db, BackpressureConfig{
		DefaultMaxTokens:    100,
		DefaultRefillPerSec: 100,
		LocalLeaseSize:      20,
	}, true)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const consumers = 20
	start := make(chan struct{})
	errs := make(chan error, consumers)
	var wg sync.WaitGroup
	for range consumers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- b.TryConsume(ctx, "project-refill")
		}()
	}

	close(start)
	<-started
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, queryRowCalls.Load())
}

func TestBackpressure_LocalLeaseWaiterUsesCompletedRefill(t *testing.T) {
	t.Parallel()

	const projectID = "project-waiter"
	b := NewBackpressure(nil, BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 1,
		LocalLeaseSize:      3,
	}, true)
	refill := &leaseRefill{done: make(chan struct{})}

	b.mu.Lock()
	b.leaseRefills[projectID] = refill
	b.mu.Unlock()

	errs := make(chan error, 1)
	go func() {
		errs <- b.tryConsumeWithLocalLease(context.Background(), &mockDBTX{}, projectID)
	}()

	b.mu.Lock()
	delete(b.leaseRefills, projectID)
	b.localLeases[projectID] = 1
	close(refill.done)
	b.mu.Unlock()

	require.NoError(t, <-errs)
	require.Equal(t, 0, b.localLeases[projectID])
}

func TestBackpressure_LocalLeaseWaiterReturnsRefillError(t *testing.T) {
	t.Parallel()

	const projectID = "project-waiter-error"
	wantErr := errors.New("refill failed")
	refill := &leaseRefill{
		done: make(chan struct{}),
		err:  wantErr,
	}
	close(refill.done)

	b := NewBackpressure(nil, BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 1,
		LocalLeaseSize:      3,
	}, true)
	b.leaseRefills[projectID] = refill

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := b.tryConsumeWithLocalLease(ctx, &mockDBTX{}, projectID)
	require.ErrorIs(t, err, wantErr)
}

func TestBackpressure_StrictLeaseConsumesDBEveryCall(t *testing.T) {
	t.Parallel()

	var queryRowCalls int
	db := &mockDBTX{
		queryRowFn: func(context.Context, string, ...any) pgx.Row {
			queryRowCalls++
			return &mockRow{scanFn: func(dest ...any) error {
				*(dest[0].(*int)) = 0
				*(dest[1].(*int)) = 10
				*(dest[2].(*int)) = 1
				return nil
			}}
		},
	}
	b := NewBackpressure(db, BackpressureConfig{
		DefaultMaxTokens:    10,
		DefaultRefillPerSec: 1,
		LocalLeaseSize:      1,
	}, true)

	for range 3 {
		require.NoError(t, b.TryConsume(context.Background(), "project-strict"))
	}
	require.Equal(t, 3, queryRowCalls)
}

func TestBackpressure_ThrottleRetryAfterUsesRefillRate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		refill int
		n      int
		want   time.Duration
	}{
		{name: "refill_rate_estimate", refill: 4, n: 8, want: 2 * time.Second},
		{name: "no_refill_fallback", refill: 0, n: 8, want: time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := &mockDBTX{
				queryRowFn: func(context.Context, string, ...any) pgx.Row {
					return &mockRow{scanFn: func(...any) error {
						return pgx.ErrNoRows
					}}
				},
			}
			b := NewBackpressure(db, BackpressureConfig{
				DefaultMaxTokens:    10,
				DefaultRefillPerSec: tt.refill,
				LocalLeaseSize:      1,
			}, true)

			err := b.TryConsumeN(context.Background(), "project-throttled", tt.n)
			throttled, ok := AsThrottled(err)
			require.True(t, ok)
			assert.Equal(t, "project-throttled", throttled.ProjectID)
			assert.Equal(t, tt.want, throttled.RetryAfter)
		})
	}
}

func TestBackpressure_SampleAvailableTokensErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		db      *mockDBTX
		wantErr string
	}{
		{
			name: "query error",
			db: &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return nil, errors.New("db unavailable")
				},
			},
			wantErr: "sample backpressure tokens",
		},
		{
			name: "scan error",
			db: &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &tokenSampleRows{samples: []TokenSample{{ProjectID: "project-a", Tokens: 12}}, scanErr: errors.New("bad row")}, nil
				},
			},
			wantErr: "scan sample",
		},
		{
			name: "rows error",
			db: &mockDBTX{
				queryFn: func(context.Context, string, ...any) (pgx.Rows, error) {
					return &tokenSampleRows{err: errors.New("rows closed early")}, nil
				},
			},
			wantErr: "rows closed early",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := NewBackpressure(tt.db, BackpressureConfig{}, true)
			samples, err := b.SampleAvailableTokens(context.Background(), 3)
			require.ErrorContains(t, err, tt.wantErr)
			assert.Empty(t, samples)
		})
	}
}

type tokenSampleRows struct {
	samples []TokenSample
	idx     int
	scanErr error
	err     error
	closed  bool
}

func (r *tokenSampleRows) Close() { r.closed = true }

func (r *tokenSampleRows) Err() error { return r.err }

func (r *tokenSampleRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

func (r *tokenSampleRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

func (r *tokenSampleRows) Next() bool {
	if r.idx >= len(r.samples) {
		return false
	}
	r.idx++
	return true
}

func (r *tokenSampleRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if len(dest) != 2 {
		return errors.New("tokenSampleRows: expected two destinations")
	}
	projectID, ok := dest[0].(*string)
	if !ok {
		return errors.New("tokenSampleRows: project destination is not *string")
	}
	tokens, ok := dest[1].(*int64)
	if !ok {
		return errors.New("tokenSampleRows: tokens destination is not *int64")
	}
	sample := r.samples[r.idx-1]
	*projectID = sample.ProjectID
	*tokens = sample.Tokens
	return nil
}

func (r *tokenSampleRows) Values() ([]any, error) {
	sample := r.samples[r.idx-1]
	return []any{sample.ProjectID, sample.Tokens}, nil
}

func (r *tokenSampleRows) RawValues() [][]byte { return nil }

func (r *tokenSampleRows) Conn() *pgx.Conn { return nil }
