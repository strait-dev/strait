package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
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
