package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"strait/internal/domain"

	"github.com/jackc/pgx/v5"
)

func TestBatchlogConfig_NormalizedDefaults(t *testing.T) {
	cfg := BatchlogConfig{}.normalized()
	if cfg.TickInterval != 100*time.Millisecond {
		t.Fatalf("TickInterval = %s, want 100ms", cfg.TickInterval)
	}
	if cfg.LeaseDuration != 30*time.Second {
		t.Fatalf("LeaseDuration = %s, want 30s", cfg.LeaseDuration)
	}
	if cfg.LeaseOwner == "" {
		t.Fatal("LeaseOwner is empty")
	}
}

func TestQueueLeaseExpired(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Second)
	future := now.Add(time.Second)
	if !leaseExpired(now, &past) {
		t.Fatal("past lease should be expired")
	}
	if leaseExpired(now, &future) {
		t.Fatal("future lease should not be expired")
	}
	if leaseExpired(now, nil) {
		t.Fatal("nil lease should not be expired")
	}
}

func TestBatchlogDequeueUsesStaticParameterizedQuery(t *testing.T) {
	t.Parallel()

	errCaptured := errors.New("captured")
	tests := []struct {
		name        string
		projectID   string
		wantArgs    int
		wantProject bool
	}{
		{name: "global", wantArgs: 4},
		{name: "project", projectID: "project-1", wantArgs: 5, wantProject: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var capturedSQL string
			var capturedArgs []any
			db := &mockDBTX{
				queryFn: func(_ context.Context, sql string, args ...any) (pgx.Rows, error) {
					capturedSQL = sql
					capturedArgs = append([]any(nil), args...)
					return nil, errCaptured
				},
			}
			q := NewBatchlogQueue(db, NewPostgresQueue(db), BatchlogConfig{
				LeaseOwner:    "worker-1",
				LeaseDuration: 5 * time.Second,
			})

			if tt.projectID == "" {
				_, _ = q.DequeueN(context.Background(), 10)
			} else {
				_, _ = q.DequeueNByProject(context.Background(), 10, tt.projectID)
			}

			if len(capturedArgs) != tt.wantArgs {
				t.Fatalf("args len = %d, want %d: %#v", len(capturedArgs), tt.wantArgs, capturedArgs)
			}
			if capturedArgs[0] != 10 || capturedArgs[1] != "worker-1" || capturedArgs[2] != 5*time.Second || capturedArgs[3] != domain.StatusQueued {
				t.Fatalf("unexpected args: %#v", capturedArgs)
			}
			if tt.wantProject {
				if capturedArgs[4] != tt.projectID {
					t.Fatalf("project arg = %#v, want %q", capturedArgs[4], tt.projectID)
				}
				if !strings.Contains(capturedSQL, "qe.project_id = $5") {
					t.Fatalf("project query missing project predicate:\n%s", capturedSQL)
				}
			} else if strings.Contains(capturedSQL, "qe.project_id = $5") {
				t.Fatalf("global query contains project predicate:\n%s", capturedSQL)
			}
			for _, want := range []string{"FOR UPDATE OF qe SKIP LOCKED", "LIMIT $1", "qe.run_status = $4", "qe.execution_mode = 'http'", "job_max_concurrency"} {
				if !strings.Contains(capturedSQL, want) {
					t.Fatalf("query missing %q:\n%s", want, capturedSQL)
				}
			}
			if strings.Contains(capturedSQL, "leased_job_counts") || strings.Contains(capturedSQL, "leased_key_counts") {
				t.Fatalf("unconstrained fast path should not aggregate leases:\n%s", capturedSQL)
			}
			if strings.Contains(capturedSQL, "LEFT JOIN LATERAL") {
				t.Fatalf("query should aggregate leased counts once instead of per-candidate lateral scans:\n%s", capturedSQL)
			}
			if strings.Contains(capturedSQL, "run_status = 'queued'") {
				t.Fatalf("query should parameterize queued status:\n%s", capturedSQL)
			}
		})
	}
}

func TestBatchlogDequeueQuerySelectionAllocFree(t *testing.T) {
	if allocs := testing.AllocsPerRun(1000, func() { _ = batchlogDequeueHTTPSQL }); allocs != 0 {
		t.Fatalf("global query access allocs = %.2f, want 0", allocs)
	}
	if allocs := testing.AllocsPerRun(1000, func() { _ = batchlogDequeueByProjectSQL }); allocs != 0 {
		t.Fatalf("project query access allocs = %.2f, want 0", allocs)
	}
}

var batchlogQuerySink string

func BenchmarkBatchlogDequeueQuerySelection(b *testing.B) {
	for _, projectID := range []string{"", "project-1"} {
		name := "global"
		if projectID != "" {
			name = "project"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if projectID == "" {
					batchlogQuerySink = batchlogDequeueHTTPSQL
				} else {
					batchlogQuerySink = batchlogDequeueByProjectSQL
				}
			}
		})
	}
}

func FuzzQueueLease(f *testing.F) {
	f.Add(int64(0), int64(-1))
	f.Add(int64(0), int64(1))
	f.Fuzz(func(t *testing.T, nowUnix, deltaMillis int64) {
		now := time.Unix(nowUnix%4102444800, 0)
		expires := now.Add(time.Duration(deltaMillis) * time.Millisecond)
		got := leaseExpired(now, &expires)
		want := deltaMillis <= 0
		if got != want {
			t.Fatalf("leaseExpired(%s, %s) = %v, want %v", now, expires, got, want)
		}
	})
}

func FuzzBatchlog(f *testing.F) {
	f.Add(int64(0), int64(100), "worker-a")
	f.Add(int64(-1), int64(-1), "")
	f.Fuzz(func(t *testing.T, tickMillis, leaseMillis int64, owner string) {
		cfg := BatchlogConfig{
			TickInterval:  time.Duration(tickMillis) * time.Millisecond,
			LeaseDuration: time.Duration(leaseMillis) * time.Millisecond,
			LeaseOwner:    owner,
		}.normalized()
		if cfg.TickInterval <= 0 {
			t.Fatalf("TickInterval = %s, want positive", cfg.TickInterval)
		}
		if cfg.LeaseDuration <= 0 {
			t.Fatalf("LeaseDuration = %s, want positive", cfg.LeaseDuration)
		}
		if cfg.LeaseOwner == "" {
			t.Fatal("LeaseOwner is empty")
		}
	})
}
