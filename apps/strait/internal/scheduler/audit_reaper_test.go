package scheduler

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAuditReclaimerStore is a minimal ReaperStore implementation that
// exercises reclaimAuditDeadletter without requiring a real database.
// Only audit-related methods are meaningful; all others return zero values.
type fakeAuditReclaimerStore struct {
	mockReaperStore

	listLimit atomic.Int32
	listFn    func(ctx context.Context, limit int) ([]domain.AuditEvent, []string, []store.AuditDeadletterAttemptInfo, error)
	createFn  func(ctx context.Context, ev *domain.AuditEvent) error
	deleteFn  func(ctx context.Context, id, projectID string) error

	createCalls           atomic.Int32
	deleteCalls           atomic.Int32
	incCalls              atomic.Int32
	markCalls             atomic.Int32
	deleteAgedFn          func(ctx context.Context, cutoff time.Time) (map[string]int64, error)
	deleteAgedWithAuditFn func(ctx context.Context, cutoff time.Time, maxAgeDays int) (map[string]int64, error)
	deleteAgedHits        atomic.Int32
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

func (f *fakeAuditReclaimerStore) ReplayAuditEventDeadletter(ctx context.Context, id, projectID, newEventID string) (*domain.AuditEvent, bool, error) {
	ev := &domain.AuditEvent{ID: newEventID, ProjectID: projectID}
	f.createCalls.Add(1)
	if f.createFn != nil {
		if err := f.createFn(ctx, ev); err != nil {
			return nil, false, err
		}
	}
	f.markCalls.Add(1)
	if f.deleteFn != nil {
		if err := f.deleteFn(ctx, id, projectID); err != nil {
			return nil, false, err
		}
	}
	f.deleteCalls.Add(1)
	return ev, true, nil
}

func (f *fakeAuditReclaimerStore) DeleteAuditDeadletterOlderThan(ctx context.Context, cutoff time.Time) (map[string]int64, error) {
	f.deleteAgedHits.Add(1)
	if f.deleteAgedFn != nil {
		return f.deleteAgedFn(ctx, cutoff)
	}
	return nil, nil
}

func (f *fakeAuditReclaimerStore) DeleteAuditDeadletterOlderThanWithAudit(ctx context.Context, cutoff time.Time, maxAgeDays int) (map[string]int64, error) {
	f.deleteAgedHits.Add(1)
	if f.deleteAgedWithAuditFn != nil {
		return f.deleteAgedWithAuditFn(ctx, cutoff, maxAgeDays)
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

func (f *fakeAuditReclaimerStore) DeleteAuditEventDeadletter(ctx context.Context, id, projectID string) error {
	f.deleteCalls.Add(1)
	if f.deleteFn != nil {
		return f.deleteFn(ctx, id, projectID)
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
	require.EqualValues(t, 137,
		fake.listLimit.
			Load())
}

func TestReclaimAuditDeadletter_DefaultBatchWhenUnset(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	r.reclaimAuditDeadletter(ctx)
	require.EqualValues(t, defaultAuditDLQReclaimBatch,

		fake.listLimit.
			Load())
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
	require.EqualValues(t, 3,
		fake.createCalls.
			Load())
	require.EqualValues(t, 3,
		fake.deleteCalls.
			Load())
	require.EqualValues(t, 3,
		fake.markCalls.
			Load())

	// Idempotency marker is written on every successful insert.
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
	require.EqualValues(t, 2,
		fake.createCalls.
			Load())
	require.EqualValues(t, 0,
		fake.deleteCalls.
			Load())
	require.EqualValues(t, 2,
		fake.incCalls.
			Load())

	// Row must stay in DLQ when chain insert fails.

	// And attempt count must be incremented for both rows so the
	// max-attempts cap eventually fires.
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
	require.EqualValues(t, 1,
		fake.createCalls.
			Load())
	require.EqualValues(t, 1,
		fake.deleteCalls.
			Load())

	// Only one row should be inserted (the fresh one); the other two
	// should be abandoned and skipped.
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
	require.EqualValues(t, 1,
		fake.createCalls.
			Load())
	require.EqualValues(t, 2,
		fake.deleteCalls.
			Load())

	// Only the fresh row triggers a chain insert; the previously-reclaimed
	// row only triggers a delete.
}

// TestReapDeadletter_DropsAgedRows_CallsDelete asserts the retention reaper
// delegates to the atomic store path that writes audit.deadletter_aged markers
// in the same transaction as the delete.
func TestReapDeadletter_DropsAgedRows_CallsDelete(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	fake.deleteAgedWithAuditFn = func(_ context.Context, _ time.Time, maxAgeDays int) (map[string]int64, error) {
		require.Equal(t, 30,
			maxAgeDays,
		)

		return map[string]int64{"proj-a": 7, "proj-b": 3}, nil
	}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditDLQMaxAgeDays(30)
	r.reapDeadletter(ctx)
	require.EqualValues(t, 1,
		fake.deleteAgedHits.
			Load())
	require.EqualValues(t, 0,
		fake.createCalls.
			Load())
}

// TestReapDeadletter_DisabledByZero asserts the sweep is opt-in.
func TestReapDeadletter_DisabledByZero(t *testing.T) {
	ctx := context.Background()
	fake := &fakeAuditReclaimerStore{}
	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil)
	// Default is zero — sweep disabled.
	r.reapDeadletter(ctx)
	require.EqualValues(t, 0,
		fake.deleteAgedHits.
			Load())
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
	assert.Equal(t, 1,
		fake.perProjectCalls["proj-a"])
	assert.Equal(t, 1,
		fake.perProjectCalls["proj-b"])
	require.Len(t, fake.
		excludingCalls,
		1)

	excluded := fake.excludingCalls[0].excluded
	require.Len(t, excluded,
		2)
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
	assert.Equal(t, 0,
		fake.perProjectCalls["proj-disabled"])
	assert.Equal(t, 1,
		fake.perProjectCalls["proj-active"])
	require.Len(t, fake.
		excludingCalls,
		1)

	// The disabled project must still be excluded from the default sweep so
	// it is not silently trimmed by the global default.
	excluded := fake.excludingCalls[0].excluded
	var seenDisabled bool
	for _, p := range excluded {
		if p == "proj-disabled" {
			seenDisabled = true
		}
	}
	assert.True(t, seenDisabled)
}

func TestReapAuditEvents_RejectsOverflowRetentionDays(t *testing.T) {
	ctx := context.Background()
	fake := newFakeAuditRetentionStore([]store.AuditRetentionOverride{
		{ProjectID: "proj-overflow", Days: domain.MaxAuditRetentionDays + 1},
	})

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(365)
	r.reapAuditEvents(ctx)
	assert.Equal(t, 0,
		fake.perProjectCalls["proj-overflow"])
	require.Len(t, fake.
		excludingCalls,
		1)
}

func TestReapAuditEvents_RejectsOverflowDefaultRetentionDays(t *testing.T) {
	ctx := context.Background()
	fake := newFakeAuditRetentionStore(nil)

	r := NewReaper(fake, time.Second, time.Minute, 0, 0, false, nil).
		WithAuditRetention(domain.MaxAuditRetentionDays + 1)
	r.reapAuditEvents(ctx)
	require.Empty(t, fake.
		excludingCalls)
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
	require.LessOrEqual(t, elapsed,
		2*time.Second,
	)
	assert.EqualValues(t, 2,
		fake.createCalls.
			Load())
	assert.EqualValues(t, 2,
		fake.incCalls.
			Load())

	// Both rows hit the per-event deadline; total elapsed ~2 * timeout.
	// Generous upper bound catches regressions where the timeout is not
	// applied per-event (would wedge indefinitely under any parent ctx
	// longer than the bound).
}
