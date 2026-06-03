package scheduler

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/sourcegraph/conc"
)

// Unit tests for the priority promoter. Integration tests live in
// priority_promoter_integration_test.go.

type fakeDB struct {
	calls    int
	lastSQL  string
	lastArgs []any
	rows     int64
	err      error
}

func (f *fakeDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	f.calls++
	f.lastSQL = sql
	f.lastArgs = args
	if f.err != nil {
		return pgconn.CommandTag{}, f.err
	}
	// pgconn.NewCommandTag requires a byte buffer like "UPDATE 3"; easier to
	// fabricate a tag that reports the right RowsAffected via fake string.
	return pgconn.NewCommandTag("UPDATE " + toBase10(f.rows)), nil
}
func (f *fakeDB) Query(_ context.Context, _ string, _ ...any) (pgx.Rows, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeDB) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	return nil
}

func toBase10(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type fakeLocker struct {
	acquired  bool
	released  bool
	acquireOK bool
	err       error
}

func (f *fakeLocker) TryAdvisoryLock(_ context.Context, _ int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	f.acquired = true
	return f.acquireOK, nil
}
func (f *fakeLocker) ReleaseAdvisoryLock(_ context.Context, _ int64) error {
	f.released = true
	return nil
}

func TestPriorityPromoter_Defaults(t *testing.T) {
	p := NewPriorityPromoter(&fakeDB{}, PriorityPromoterConfig{})
	if p.interval != 60*time.Second {
		t.Errorf("interval = %v", p.interval)
	}
	if p.ageThreshold != 5*time.Minute {
		t.Errorf("ageThreshold = %v", p.ageThreshold)
	}
	if p.maxPriority != 1000 {
		t.Errorf("maxPriority = %d", p.maxPriority)
	}
	if p.batchLimit != 500 {
		t.Errorf("batchLimit = %d", p.batchLimit)
	}
}

func TestPriorityPromoter_RunOnce_IssuesUpdate(t *testing.T) {
	db := &fakeDB{rows: 3}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		Interval:     time.Second,
		AgeThreshold: 10 * time.Second,
		MaxPriority:  100,
		BatchLimit:   50,
		Logger:       slog.Default(),
	})
	if err := p.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if db.calls != 1 {
		t.Errorf("calls = %d, want 1", db.calls)
	}
	if p.RowsPromoted() != 3 {
		t.Errorf("RowsPromoted = %d, want 3", p.RowsPromoted())
	}
	if p.Iterations() != 1 {
		t.Errorf("Iterations = %d, want 1", p.Iterations())
	}
}

func TestPriorityPromoter_RunOnce_WithLock_Acquired(t *testing.T) {
	db := &fakeDB{rows: 1}
	locker := &fakeLocker{acquireOK: true}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	if err := p.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if !locker.acquired {
		t.Error("locker should have been called")
	}
	if !locker.released {
		t.Error("locker should have released")
	}
	if db.calls != 1 {
		t.Errorf("db calls = %d, want 1", db.calls)
	}
}

func TestPriorityPromoter_RunOnce_WithLock_NotAcquired(t *testing.T) {
	db := &fakeDB{rows: 1}
	locker := &fakeLocker{acquireOK: false}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	if err := p.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce: %v", err)
	}
	if db.calls != 0 {
		t.Errorf("db calls = %d, want 0 (lock not acquired)", db.calls)
	}
	if locker.released {
		t.Error("locker should not release when not acquired")
	}
}

func TestPriorityPromoter_RunOnce_LockError(t *testing.T) {
	db := &fakeDB{}
	locker := &fakeLocker{err: errors.New("pg down")}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{}).WithAdvisoryLocker(locker)
	if err := p.runOnce(context.Background()); err == nil {
		t.Error("expected error")
	}
	if db.calls != 0 {
		t.Errorf("db calls = %d, want 0", db.calls)
	}
}

func TestPriorityPromoter_RunOnce_UpdateError(t *testing.T) {
	db := &fakeDB{err: errors.New("deadlock")}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{})
	if err := p.runOnce(context.Background()); err == nil {
		t.Error("expected update error")
	}
	if p.Iterations() != 1 {
		t.Errorf("Iterations = %d, want 1", p.Iterations())
	}
}

func TestPriorityPromoter_QuerySanity(t *testing.T) {
	db := &fakeDB{rows: 0}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		MaxPriority:  42,
		AgeThreshold: 30 * time.Second,
		BatchLimit:   100,
	})
	_ = p.runOnce(context.Background())
	if db.lastSQL == "" {
		t.Fatal("no SQL captured")
	}
	// The exec must be parameterized (no inline values) so pg can cache the
	// plan. $1 $2 $3 should appear.
	for _, p := range []string{"$1", "$2", "$3", "job_runs", "status = 'queued'", "WHERE id IN (SELECT id FROM candidates)", "LEAST(priority + 1", "UPDATE job_run_queue q", "q.run_id = promoted.id"} {
		if !contains(db.lastSQL, p) {
			t.Errorf("SQL missing %q: %s", p, db.lastSQL)
		}
	}
	if len(db.lastArgs) != 3 {
		t.Errorf("args = %v, want 3 args", db.lastArgs)
	}
	if db.lastArgs[0] != 42 {
		t.Errorf("max priority arg = %v", db.lastArgs[0])
	}
	if db.lastArgs[2] != 100 {
		t.Errorf("batch limit arg = %v", db.lastArgs[2])
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestPriorityPromoter_RunExitsOnContextCancel(t *testing.T) {
	var concWG conc.WaitGroup
	defer concWG.Wait()
	db := &fakeDB{rows: 0}
	p := NewPriorityPromoter(db, PriorityPromoterConfig{
		Interval: 5 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	concWG.Go(func() {
		p.Run(ctx)
		close(done)
	})
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not exit on cancel")
	}
	if p.Iterations() < 2 {
		t.Errorf("Iterations = %d, want >= 2", p.Iterations())
	}
}
