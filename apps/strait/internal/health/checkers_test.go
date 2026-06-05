package health

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPool struct {
	available int
	active    int
}

func (m *mockPool) Available() int   { return m.available }
func (m *mockPool) ActiveCount() int { return m.active }

type mockSequinReadinessClient struct {
	healthErr       error
	sinkConsumerErr error
}

func (m mockSequinReadinessClient) Health(context.Context) error {
	return m.healthErr
}

func (m mockSequinReadinessClient) SinkConsumerHealth(context.Context) error {
	return m.sinkConsumerErr
}

func TestNewPoolChecker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pool        *mockPool
		wantErr     bool
		wantErrPart string
	}{
		{name: "healthy pool", pool: &mockPool{available: 5, active: 3}},
		{name: "exhausted pool", pool: &mockPool{available: 0, active: 2}, wantErr: true, wantErrPart: "worker pool exhausted"},
		{name: "idle pool", pool: &mockPool{available: 0, active: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := NewPoolChecker(tt.pool)
			err := checker.Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrPart != "" {
					assert.Contains(t, err.Error(), tt.wantErrPart)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewMigrationChecker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		current     uint
		dirty       bool
		checkErr    error
		wantErr     bool
		wantErrPart string
	}{
		{name: "up-to-date", current: 42, dirty: false, checkErr: nil},
		{name: "dirty migration", current: 42, dirty: true, checkErr: nil, wantErr: true, wantErrPart: "dirty"},
		{name: "pending migration error", current: 0, dirty: false, checkErr: errors.New("pending"), wantErr: true, wantErrPart: "migration status unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := NewMigrationChecker(tt.current, tt.dirty, tt.checkErr)
			err := checker.Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrPart != "" {
					assert.Contains(t, err.Error(), tt.wantErrPart)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewSchedulerChecker(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name        string
		lastTick    time.Time
		maxAge      time.Duration
		wantErr     bool
		wantErrPart string
	}{
		{name: "fresh tick", lastTick: now.Add(-20 * time.Millisecond), maxAge: 200 * time.Millisecond},
		{name: "zero tick", lastTick: time.Time{}, maxAge: 200 * time.Millisecond, wantErr: true, wantErrPart: "scheduler tick unavailable"},
		{name: "stale tick", lastTick: now.Add(-2 * time.Second), maxAge: 100 * time.Millisecond, wantErr: true, wantErrPart: "scheduler stale"},
		{name: "tick at exact max age boundary is healthy", lastTick: now.Add(-200 * time.Millisecond), maxAge: 200 * time.Millisecond},
		{name: "tick 1ns past max age is stale", lastTick: now.Add(-200*time.Millisecond - time.Nanosecond), maxAge: 200 * time.Millisecond, wantErr: true, wantErrPart: "scheduler stale"},
		{name: "very small max age triggers stale", lastTick: now.Add(-1 * time.Millisecond), maxAge: 1 * time.Nanosecond, wantErr: true, wantErrPart: "scheduler stale"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := newSchedulerChecker(func() time.Time { return tt.lastTick }, tt.maxAge, func() time.Time { return now })
			err := checker.Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrPart != "" {
					assert.Contains(t, err.Error(), tt.wantErrPart)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}

type mockRedisPinger struct {
	err error
}

func (m *mockRedisPinger) Ping(_ context.Context) error { return m.err }

func TestNewRedisChecker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pingErr     error
		wantErr     bool
		wantErrPart string
	}{
		{name: "healthy redis", pingErr: nil},
		{name: "redis down", pingErr: errors.New("connection refused"), wantErr: true, wantErrPart: "redis ping failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := NewRedisChecker(&mockRedisPinger{err: tt.pingErr})
			err := checker.Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrPart != "" {
					assert.Contains(t, err.Error(), tt.wantErrPart)
				}
				assert.True(t, IsCritical(checker))
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestNewSequinChecker(t *testing.T) {
	t.Parallel()

	t.Run("healthy sequin", func(t *testing.T) {
		t.Parallel()
		checker := NewSequinChecker(mockSequinReadinessClient{})
		require.NoError(t, checker.Check(context.Background()))
		assert.True(t, IsCritical(checker))
	})

	t.Run("unhealthy sequin process", func(t *testing.T) {
		t.Parallel()
		checker := NewSequinChecker(mockSequinReadinessClient{healthErr: errors.New("HTTP 503")})
		err := checker.Check(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sequin health failed")
		assert.True(t, IsCritical(checker))
	})

	t.Run("unhealthy sink consumer", func(t *testing.T) {
		t.Parallel()
		checker := NewSequinChecker(mockSequinReadinessClient{sinkConsumerErr: errors.New("consumer paused")})
		err := checker.Check(context.Background())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "sequin sink consumer health failed")
		assert.True(t, IsCritical(checker))
	})
}

func TestNewQueueDepthChecker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		depth       int64
		depthErr    error
		threshold   int64
		wantErr     bool
		wantErrPart string
	}{
		{name: "below threshold", depth: 9, threshold: 10},
		{name: "at threshold", depth: 10, threshold: 10},
		{name: "above threshold", depth: 11, threshold: 10, wantErr: true, wantErrPart: "exceeds threshold"},
		{name: "query error", depthErr: errors.New("query failed"), threshold: 10, wantErr: true, wantErrPart: "queue depth check failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := NewQueueDepthChecker(func(context.Context) (int64, error) {
				if tt.depthErr != nil {
					return 0, tt.depthErr
				}
				return tt.depth, nil
			}, tt.threshold)

			err := checker.Check(context.Background())
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrPart != "" {
					assert.Contains(t, err.Error(), tt.wantErrPart)
				}
				return
			}
			require.NoError(t, err)
		})
	}
}
