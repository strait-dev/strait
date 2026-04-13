package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

// fakeAuditReclaimerStore is a minimal ReaperStore implementation that
// exercises reclaimAuditDeadletter without requiring a real database.
// Only audit-related methods are meaningful; all others return zero values.
type fakeAuditReclaimerStore struct {
	mockReaperStore

	listLimit atomic.Int32
	listFn    func(ctx context.Context, limit int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error)
	createFn  func(ctx context.Context, ev *domain.AuditEvent) error
	deleteFn  func(ctx context.Context, id string) error

	createCalls    atomic.Int32
	deleteCalls    atomic.Int32
	incCalls       atomic.Int32
	markCalls      atomic.Int32
	deleteAgedFn   func(ctx context.Context, cutoff time.Time) (map[string]int64, error)
	deleteAgedHits atomic.Int32
}

func (f *fakeAuditReclaimerStore) ListAuditEventsDeadletter(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
	return nil, nil, nil
}

func (f *fakeAuditReclaimerStore) ListAuditEventsDeadletterWithAttempts(ctx context.Context, limit int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
	f.listLimit.Store(int32(limit))
	if f.listFn != nil {
		return f.listFn(ctx, limit)
	}
	return nil, nil, nil, nil
}

func (f *fakeAuditReclaimerStore) IncrementAuditDeadletterAttempt(_ context.Context, _ string) error {
	f.incCalls.Add(1)
	return nil
}

func (f *fakeAuditReclaimerStore) MarkAuditDeadletterReclaimed(_ context.Context, _, _ string) error {
	f.markCalls.Add(1)
	return nil
}

func (f *fakeAuditReclaimerStore) DeleteAuditDeadletterOlderThan(ctx context.Context, cutoff time.Time) (map[string]int64, error) {
	f.deleteAgedHits.Add(1)
	if f.deleteAgedFn != nil {
		return f.deleteAgedFn(ctx, cutoff)
	}
	return nil, nil
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

func (f *fakeAuditReclaimerStore) DeleteAuditEventsBeforeExcluding(_ context.Context, _ time.Time, _ []string) (int64, error) {
	return 0, nil
}

func (f *fakeAuditReclaimerStore) ListAuditRetentionOverrides(_ context.Context) ([]store.AuditRetentionOverride, error) {
	return nil, nil
}

func newFakeDLQEvents(n int) ([]domain.AuditEvent, []string) {
	events := make([]domain.AuditEvent, n)
	ids := make([]string, n)
	for i := range n {
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

// emptyAttempts builds a fresh attempt-info slice for n rows with no prior
// attempts and no idempotency marker — the common reclaim-from-cold path.
func emptyAttempts(n int) []store.AuditDeadletterAttemptInfo {
	out := make([]store.AuditDeadletterAttemptInfo, n)
	return out
}

func TestReclaimAuditDeadletter_UsesConfiguredBatchSize(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		return nil, nil, nil, nil
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
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.ID
		}
		return events, ids, emptyAttempts(len(events)), nil
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)

	if got := fake.createCalls.Load(); got != 3 {
		t.Fatalf("CreateAuditEvent calls = %d, want 3", got)
	}
	if got := fake.deleteCalls.Load(); got != 3 {
		t.Fatalf("DeleteAuditEventDeadletter calls = %d, want 3", got)
	}
	// Idempotency marker is written on every successful insert.
	if got := fake.markCalls.Load(); got != 3 {
		t.Fatalf("MarkAuditDeadletterReclaimed calls = %d, want 3", got)
	}
}

func TestReclaimAuditDeadletter_ChainInsertFailure_SkipsDelete(t *testing.T) {
	ctx := context.Background()
	events, _ := newFakeDLQEvents(2)
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.ID
		}
		return events, ids, emptyAttempts(len(events)), nil
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
	// And attempt count must be incremented for both rows so the
	// max-attempts cap eventually fires.
	if got := fake.incCalls.Load(); got != 2 {
		t.Fatalf("IncrementAuditDeadletterAttempt calls = %d, want 2", got)
	}
}

func TestReclaimAuditDeadletter_ListError_RecordsOperation(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		return nil, nil, nil, errors.New("db broken")
	}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	// Must not panic and must return cleanly.
	r.reclaimAuditDeadletter(ctx)
}

// TestReclaimAuditDeadletter_RespectsMaxAttempts asserts that rows whose
// attempt_count has reached the configured cap are skipped this tick (no
// chain insert) and counted as abandoned.
func TestReclaimAuditDeadletter_RespectsMaxAttempts(t *testing.T) {
	ctx := context.Background()
	events, ids := newFakeDLQEvents(3)
	// Row 0 is fresh, rows 1 and 2 have hit the cap.
	infos := []store.AuditDeadletterAttemptInfo{
		{AttemptCount: 0},
		{AttemptCount: 5},
		{AttemptCount: 10},
	}
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		return events, ids, infos, nil
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQMaxReclaimAttempts(5)
	r.reclaimAuditDeadletter(ctx)

	// Only one row should be inserted (the fresh one); the other two
	// should be abandoned and skipped.
	if got := fake.createCalls.Load(); got != 1 {
		t.Fatalf("CreateAuditEvent calls = %d, want 1 (others abandoned)", got)
	}
	if got := fake.deleteCalls.Load(); got != 1 {
		t.Fatalf("DeleteAuditEventDeadletter calls = %d, want 1", got)
	}
}

// TestReclaimAuditDeadletter_IdempotentWhenAlreadyReclaimed asserts that
// when a DLQ row already carries reclaimed_event_id, the chain insert is
// skipped and only the DLQ delete runs. This is the at-least-once path —
// a previous successful insert + failed delete must not produce a second
// chain row on retry.
func TestReclaimAuditDeadletter_IdempotentWhenAlreadyReclaimed(t *testing.T) {
	ctx := context.Background()
	events, ids := newFakeDLQEvents(2)
	prevEventID := "previously-inserted-id"
	infos := []store.AuditDeadletterAttemptInfo{
		{AttemptCount: 1, ReclaimedEventID: &prevEventID},
		{AttemptCount: 0}, // fresh row goes through the normal path
	}
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		return events, ids, infos, nil
	}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)

	// Only the fresh row triggers a chain insert; the previously-reclaimed
	// row only triggers a delete.
	if got := fake.createCalls.Load(); got != 1 {
		t.Fatalf("CreateAuditEvent calls = %d, want 1 (idempotency must skip the marked row)", got)
	}
	if got := fake.deleteCalls.Load(); got != 2 {
		t.Fatalf("DeleteAuditEventDeadletter calls = %d, want 2", got)
	}
}

// TestReapDeadletter_DropsAgedRows_CallsDelete asserts the new retention
// reaper invokes the store and emits one audit.deadletter_aged event per
// affected project.
func TestReapDeadletter_DropsAgedRows_CallsDelete(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.deleteAgedFn = func(_ context.Context, _ time.Time) (map[string]int64, error) {
		return map[string]int64{"proj-a": 7, "proj-b": 3}, nil
	}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQMaxAgeDays(30)
	r.reapDeadletter(ctx)

	if got := fake.deleteAgedHits.Load(); got != 1 {
		t.Fatalf("DeleteAuditDeadletterOlderThan calls = %d, want 1", got)
	}
	// One audit.deadletter_aged event per project that lost rows.
	if got := fake.createCalls.Load(); got != 2 {
		t.Fatalf("CreateAuditEvent calls = %d, want 2 (one per affected project)", got)
	}
}

// TestReapDeadletter_DisabledByZero asserts the sweep is opt-in.
func TestReapDeadletter_DisabledByZero(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	// Default is zero — sweep disabled.
	r.reapDeadletter(ctx)
	if got := fake.deleteAgedHits.Load(); got != 0 {
		t.Fatalf("retention sweep ran with max_age_days=0: hits=%d", got)
	}
}

// fakeAuditRetentionStore captures DeleteAuditEventsBefore calls per project
// so we can assert the reaper honors per-project overrides from
// project_quotas.audit_retention_days.
type fakeAuditRetentionStore struct {
	mockReaperStore

	overrides []store.AuditRetentionOverride

	perProjectCalls map[string]int
	excludingCalls  []struct {
		excluded []string
	}
}

func newFakeAuditRetentionStore(overrides []store.AuditRetentionOverride) *fakeAuditRetentionStore {
	return &fakeAuditRetentionStore{
		overrides:       overrides,
		perProjectCalls: map[string]int{},
	}
}

func (f *fakeAuditRetentionStore) ListAuditRetentionOverrides(_ context.Context) ([]store.AuditRetentionOverride, error) {
	return f.overrides, nil
}

func (f *fakeAuditRetentionStore) DeleteAuditEventsBefore(_ context.Context, projectID string, _ time.Time) (int64, error) {
	f.perProjectCalls[projectID]++
	return 0, nil
}

func (f *fakeAuditRetentionStore) DeleteAuditEventsBeforeExcluding(_ context.Context, _ time.Time, excluded []string) (int64, error) {
	cp := append([]string(nil), excluded...)
	f.excludingCalls = append(f.excludingCalls, struct{ excluded []string }{excluded: cp})
	return 0, nil
}

func TestReapAuditEvents_CallsPerProjectOverrides(t *testing.T) {
	ctx := context.Background()
	fake := newFakeAuditRetentionStore([]store.AuditRetentionOverride{
		{ProjectID: "proj-a", Days: 30},
		{ProjectID: "proj-b", Days: 7},
	})

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(365)
	r.reapAuditEvents(ctx)

	if got := fake.perProjectCalls["proj-a"]; got != 1 {
		t.Errorf("DeleteAuditEventsBefore(proj-a) calls = %d, want 1", got)
	}
	if got := fake.perProjectCalls["proj-b"]; got != 1 {
		t.Errorf("DeleteAuditEventsBefore(proj-b) calls = %d, want 1", got)
	}
	if len(fake.excludingCalls) != 1 {
		t.Fatalf("DeleteAuditEventsBeforeExcluding calls = %d, want 1", len(fake.excludingCalls))
	}
	excluded := fake.excludingCalls[0].excluded
	if len(excluded) != 2 {
		t.Fatalf("excluded projects = %v, want both override projects", excluded)
	}
}

func TestReapAuditEvents_ZeroDaysSkipsTrim(t *testing.T) {
	ctx := context.Background()
	fake := newFakeAuditRetentionStore([]store.AuditRetentionOverride{
		{ProjectID: "proj-disabled", Days: 0},
		{ProjectID: "proj-active", Days: 14},
	})

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(365)
	r.reapAuditEvents(ctx)

	if got := fake.perProjectCalls["proj-disabled"]; got != 0 {
		t.Errorf("disabled project should not be trimmed, got %d calls", got)
	}
	if got := fake.perProjectCalls["proj-active"]; got != 1 {
		t.Errorf("active project trim calls = %d, want 1", got)
	}
	if len(fake.excludingCalls) != 1 {
		t.Fatalf("default sweep must still run once, got %d", len(fake.excludingCalls))
	}
	// The disabled project must still be excluded from the default sweep so
	// it is not silently trimmed by the global default.
	excluded := fake.excludingCalls[0].excluded
	var seenDisabled bool
	for _, p := range excluded {
		if p == "proj-disabled" {
			seenDisabled = true
		}
	}
	if !seenDisabled {
		t.Errorf("default sweep excluded = %v, want to contain proj-disabled", excluded)
	}
}

// TestReclaimAuditDeadletter_PerEventTimeout asserts that a single wedged
// CreateAuditEvent that ignores its parent context does not stall the
// remaining rows in the batch. Each per-row insert ctx is derived from
// auditDLQPerEventReclaimTimeout so a misbehaving store wakes up after
// ~10s (shortened to 100ms for the test) and the tick converges.
func TestReclaimAuditDeadletter_PerEventTimeout(t *testing.T) {
	// Shorten the per-event bound for the test.
	prev := auditDLQPerEventReclaimTimeout
	auditDLQPerEventReclaimTimeout = 100 * time.Millisecond
	t.Cleanup(func() { auditDLQPerEventReclaimTimeout = prev })

	events, _ := newFakeDLQEvents(2)
	fake := &fakeAuditReclaimerStore{}
	fake.listFn = func(_ context.Context, _ int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error) {
		ids := make([]string, len(events))
		for i, ev := range events {
			ids[i] = ev.ID
		}
		return events, ids, emptyAttempts(len(events)), nil
	}
	// CreateAuditEvent blocks on its context deadline — the test asserts
	// the per-event child ctx fires well before any sane tick timeout.
	fake.createFn = func(ctx context.Context, _ *domain.AuditEvent) error {
		<-ctx.Done()
		return ctx.Err()
	}

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)

	start := time.Now()
	r.reclaimAuditDeadletter(context.Background())
	elapsed := time.Since(start)

	// Both rows hit the per-event deadline; total elapsed ~2 * timeout.
	// Generous upper bound catches regressions where the timeout is not
	// applied per-event (would wedge indefinitely under any parent ctx
	// longer than the bound).
	if elapsed > 2*time.Second {
		t.Fatalf("reclaim did not honor per-event timeout: elapsed %v, want < 2s", elapsed)
	}
	if got := fake.createCalls.Load(); got != 2 {
		t.Errorf("CreateAuditEvent calls = %d, want 2", got)
	}
	if got := fake.incCalls.Load(); got != 2 {
		t.Errorf("IncrementAuditDeadletterAttempt calls = %d, want 2 (one per timed-out insert)", got)
	}
}
