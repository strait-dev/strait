package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
)

// fakeAuditReclaimerStore is a minimal ReaperStore implementation that
// exercises reclaimAuditDeadletter without requiring a real database.
// Only audit-related methods are meaningful; all others return zero values.
type fakeAuditReclaimerStore struct {
	mockReaperStore

	listLimit atomic.Int32
	listFn    func(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error)
	createFn  func(ctx context.Context, ev *domain.AuditEvent) error
	deleteFn  func(ctx context.Context, id string) error

	createCalls atomic.Int32
	deleteCalls atomic.Int32
}

func (f *fakeAuditReclaimerStore) ListAuditEventsDeadletter(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error) {
	f.listLimit.Store(int32(limit))
	if f.listFn != nil {
		return f.listFn(ctx, limit)
	}
	return nil, nil, nil
}

func (f *fakeAuditReclaimerStore) CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error {
	f.createCalls.Add(1)
	if f.createFn != nil {
		return f.createFn(ctx, ev)
	}
	return nil
}

func (f *fakeAuditReclaimerStore) DeleteAuditEventDeadletter(ctx context.Context, id string) error {
	f.deleteCalls.Add(1)
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id)
	}
	return nil
}

func (f *fakeAuditReclaimerStore) DeleteAuditEventsBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

func newFakeDLQEvents(n int) ([]domain.AuditEvent, []string) {
	events := make([]domain.AuditEvent, n)
	ids := make([]string, n)
	for i := 0; i < n; i++ {
		id := "dlq-" + time.Now().Format("20060102150405") + "-" + itoa(i)
		events[i] = domain.AuditEvent{
			ID:        id,
			ProjectID: "proj-fake",
			Action:    domain.AuditActionJobTriggered,
			CreatedAt: time.Now().UTC(),
		}
		ids[i] = id
	}
	return events, ids
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}

func TestReclaimAuditDeadletter_UsesConfiguredBatchSize(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
		return nil, nil, nil
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQReclaimBatch(137)

	r.reclaimAuditDeadletter(ctx)

	if got := fake.listLimit.Load(); got != 137 {
		t.Fatalf("list limit = %d, want 137", got)
	}
}

func TestReclaimAuditDeadletter_DefaultBatchWhenUnset(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)
	if got := fake.listLimit.Load(); got != defaultAuditDLQReclaimBatch {
		t.Fatalf("list limit = %d, want %d", got, defaultAuditDLQReclaimBatch)
	}
}

func TestReclaimAuditDeadletter_DeletesFromDLQAfterChainInsert(t *testing.T) {
	ctx := context.Background()
	events, _ := newFakeDLQEvents(3)
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.ID
		}
		return events, ids, nil
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)

	if got := fake.createCalls.Load(); got != 3 {
		t.Fatalf("CreateAuditEvent calls = %d, want 3", got)
	}
	if got := fake.deleteCalls.Load(); got != 3 {
		t.Fatalf("DeleteAuditEventDeadletter calls = %d, want 3", got)
	}
}

func TestReclaimAuditDeadletter_ChainInsertFailure_SkipsDelete(t *testing.T) {
	ctx := context.Background()
	events, _ := newFakeDLQEvents(2)
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.ID
		}
		return events, ids, nil
	}
	fake.createFn = func(_ context.Context, _ *domain.AuditEvent) error {
		return errors.New("chain down")
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)

	if got := fake.createCalls.Load(); got != 2 {
		t.Fatalf("CreateAuditEvent calls = %d, want 2", got)
	}
	// Row must stay in DLQ when chain insert fails.
	if got := fake.deleteCalls.Load(); got != 0 {
		t.Fatalf("DeleteAuditEventDeadletter calls = %d, want 0 on chain failure", got)
	}
}

func TestReclaimAuditDeadletter_ListError_RecordsOperation(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
		return nil, nil, errors.New("db broken")
	}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	// Must not panic and must return cleanly.
	r.reclaimAuditDeadletter(ctx)
}
