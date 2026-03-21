package health

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type mockPool struct {
	available int
	active    int
}

func (m *mockPool) Available() int   { return m.available }
func (m *mockPool) ActiveCount() int { return m.active }

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
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
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
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
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
		{name: "stale tick", lastTick: now.Add(-2 * time.Second), maxAge: 100 * time.Millisecond, wantErr: true, wantErrPart: "scheduler stale"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			checker := NewSchedulerChecker(func() time.Time { return tt.lastTick }, tt.maxAge)
			err := checker.Check(context.Background())
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
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
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErrPart)
				}
				// Redis checker should be non-critical.
				if IsCritical(checker) {
					t.Fatal("expected redis checker to be non-critical")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
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
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantErrPart != "" && !strings.Contains(err.Error(), tt.wantErrPart) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErrPart)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected nil, got %v", err)
			}
		})
	}
}
