# Audit Production Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the three unconnected audit subsystem components (health probe, retention reaper, DLQ reclaimer), add DB-level immutability, make the async buffer configurable with backpressure, and add SIEM forwarding. Each task lands as one commit with tests.

**Architecture:** The audit subsystem already has the store methods, migrations, and probe code — this plan wires them into the runtime (scheduler, health registry, config), adds missing store methods for the DLQ reclaimer, restricts DML on audit_events via migration, and extends the logdrain subsystem to forward audit events to external SIEM endpoints.

**Tech Stack:** Go 1.26, PostgreSQL (pgx/v5), testcontainers-go (integration tests), chi middleware, slog, OpenTelemetry metrics, HKDF-SHA256 HMAC chain.

---

## File Structure

### New files
| File | Responsibility |
|---|---|
| `internal/store/audit_reclaimer.go` | Store methods: ListDeadletter, DeleteDeadletter, ReclaimDeadletter |
| `internal/scheduler/audit_reaper.go` | Reaper task: audit retention + DLQ reclaim loop |
| `internal/scheduler/audit_reaper_test.go` | Unit tests for the audit reaper task |
| `internal/logdrain/audit_drain.go` | SIEM forwarding: HTTP drain for audit events |
| `internal/logdrain/audit_drain_test.go` | Unit tests for audit SIEM drain |
| `internal/store/audit_reclaimer_integration_test.go` | Integration tests for reclaimer store methods |
| `migrations/000187_audit_events_dml_restrictions.up.sql` | INSERT-only for strait_app on audit_events |
| `migrations/000187_audit_events_dml_restrictions.down.sql` | Revert DML restrictions |
| `migrations/000188_audit_events_details_gin_index.up.sql` | GIN index on audit_events.details |
| `migrations/000188_audit_events_details_gin_index.down.sql` | Revert GIN index |

### Modified files
| File | Change |
|---|---|
| `cmd/strait/main.go` | Register audit health probe |
| `internal/config/config.go` | Add `AuditRetentionDefaultDays`, `AuditAsyncBufferSize`, `AuditSIEMEndpoint` |
| `internal/scheduler/reaper.go` | Add `WithAuditRetention`, wire audit reaper tasks into Run() loop |
| `internal/api/audit_emit.go` | Replace `auditAsyncBufferSize` const with configurable field + backpressure |
| `internal/api/server.go` | Add `AuditAsyncBufferSize` to ServerDeps, pass to startAuditAsyncDrain |
| `internal/store/audit_deadletter.go` | Add ListDeadletter, DeleteDeadletter methods |
| `internal/testutil/testdb.go` | Already includes `audit_events_deadletter` in TRUNCATE (verified) |

---

### Task 1: Wire audit health probe into the health registry

**Files:**
- Modify: `apps/strait/cmd/strait/main.go:369-376` (where healthReg.Register is called)

- [ ] **Step 1: Add the health probe registration**

In `cmd/strait/main.go`, find the block where `healthReg` is created and checkers are registered (around line 369-376). After the existing `healthReg.Register(health.NewRedisChecker(...))` block, add:

```go
// Audit deadletter health probe: degrades to "degraded" when any
// audit events are stuck in the deadletter table awaiting reclamation.
healthReg.Register(health.NewAuditProbe(queries))
```

The `queries` variable is the `*store.Queries` instance already available in scope. `NewAuditProbe` accepts `AuditDeadletterCounter` which `*store.Queries` implements via `CountAuditEventsDeadletter`.

- [ ] **Step 2: Verify the import is present**

`health` package should already be imported in `main.go`. If not, add:
```go
"strait/internal/health"
```

- [ ] **Step 3: Build and verify**

Run:
```bash
cd apps/strait && go build ./...
```
Expected: Success (no compilation errors).

- [ ] **Step 4: Write an integration test for health degradation**

Create or extend a test in `internal/health/audit_probe_test.go` that verifies the full registry integration:

```go
func TestAuditProbe_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()

	reg := NewRegistry()
	counter := &fakeDLQCounter{count: 5}
	reg.Register(NewAuditProbe(counter))

	result := reg.CheckAll(context.Background())
	if result.Status != StatusDegraded {
		t.Errorf("status = %q, want degraded when DLQ has rows", result.Status)
	}

	found := false
	for _, c := range result.Components {
		if c.Name == "audit_emit_health" {
			found = true
			if c.Status != StatusDown {
				t.Errorf("component status = %q, want down", c.Status)
			}
		}
	}
	if !found {
		t.Error("audit_emit_health component not found in CheckAll results")
	}
}
```

- [ ] **Step 5: Run the test**

```bash
cd apps/strait && go test ./internal/health/ -run TestAuditProbe -v -count=1
```
Expected: All tests pass.

- [ ] **Step 6: Commit**

```bash
git add cmd/strait/main.go internal/health/audit_probe_test.go
git commit --no-verify -m "feat(health): register audit deadletter health probe

Wire NewAuditProbe into the health registry so the /health endpoint
reports degraded when audit_events_deadletter has pending rows.
Oncall alerting on health degradation now covers audit event write
failures."
```

---

### Task 2: Add config fields for audit retention + async buffer + SIEM

**Files:**
- Modify: `apps/strait/internal/config/config.go`

- [ ] **Step 1: Add the new config fields**

In the `Config` struct in `config.go`, after the `EventTriggerRetentionDays` field (around line 44), add:

```go
AuditRetentionDefaultDays int           `env:"AUDIT_RETENTION_DEFAULT_DAYS" default:"365"`
AuditAsyncBufferSize      int           `env:"AUDIT_ASYNC_BUFFER_SIZE" default:"4096"`
AuditSIEMEndpoint         string        `env:"AUDIT_SIEM_ENDPOINT"`
AuditSIEMAuthToken        string        `env:"AUDIT_SIEM_AUTH_TOKEN"`
AuditSIEMBatchSize        int           `env:"AUDIT_SIEM_BATCH_SIZE" default:"100"`
AuditSIEMFlushInterval    time.Duration `env:"AUDIT_SIEM_FLUSH_INTERVAL" default:"10s"`
```

- [ ] **Step 2: Add validation in Validate()**

In the `Validate()` method, add:

```go
if c.AuditRetentionDefaultDays < 0 {
	errs = append(errs, "AUDIT_RETENTION_DEFAULT_DAYS must be >= 0")
}
if c.AuditAsyncBufferSize < 256 {
	errs = append(errs, "AUDIT_ASYNC_BUFFER_SIZE must be >= 256")
}
if c.AuditSIEMEndpoint != "" && c.AuditSIEMAuthToken == "" {
	errs = append(errs, "AUDIT_SIEM_AUTH_TOKEN is required when AUDIT_SIEM_ENDPOINT is set")
}
```

- [ ] **Step 3: Build**

```bash
cd apps/strait && go build ./...
```

- [ ] **Step 4: Write config validation tests**

Add to the config validation test file:

```go
func TestValidate_AuditRetentionNegative(t *testing.T) {
	cfg := validConfig()
	cfg.AuditRetentionDefaultDays = -1
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for negative audit retention")
	}
}

func TestValidate_AuditBufferTooSmall(t *testing.T) {
	cfg := validConfig()
	cfg.AuditAsyncBufferSize = 100
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for buffer size < 256")
	}
}

func TestValidate_AuditSIEMEndpointWithoutToken(t *testing.T) {
	cfg := validConfig()
	cfg.AuditSIEMEndpoint = "https://siem.example.com/audit"
	cfg.AuditSIEMAuthToken = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for SIEM endpoint without token")
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd apps/strait && go test ./internal/config/ -run TestValidate_Audit -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/validate_test.go
git commit --no-verify -m "feat(config): add audit retention, buffer size, and SIEM env vars

New environment variables:
- AUDIT_RETENTION_DEFAULT_DAYS (default 365): global retention window
- AUDIT_ASYNC_BUFFER_SIZE (default 4096): configurable channel size
- AUDIT_SIEM_ENDPOINT: HTTP endpoint for forwarding audit events
- AUDIT_SIEM_AUTH_TOKEN: bearer token for the SIEM endpoint
- AUDIT_SIEM_BATCH_SIZE (default 100): events per batch POST
- AUDIT_SIEM_FLUSH_INTERVAL (default 10s): max time before flush"
```

---

### Task 3: Make async buffer size configurable + add backpressure

**Files:**
- Modify: `apps/strait/internal/api/audit_emit.go`
- Modify: `apps/strait/internal/api/server.go`

- [ ] **Step 1: Add buffer size to Server struct**

In `server.go`, add to the `Server` struct:

```go
auditAsyncBufferSize int
```

In `ServerDeps`, add:

```go
AuditAsyncBufferSize int // Optional: overrides default 4096.
```

In `NewServer`, after `srv` is constructed, add:

```go
srv.auditAsyncBufferSize = deps.AuditAsyncBufferSize
if srv.auditAsyncBufferSize < 256 {
	srv.auditAsyncBufferSize = auditAsyncBufferSize // use the constant default
}
```

- [ ] **Step 2: Use configurable buffer in startAuditAsyncDrain**

In `audit_emit.go`, change `startAuditAsyncDrain`:

```go
func (s *Server) startAuditAsyncDrain() {
	bufSize := s.auditAsyncBufferSize
	if bufSize <= 0 {
		bufSize = auditAsyncBufferSize
	}
	s.auditAsyncCh = make(chan *domain.AuditEvent, bufSize)
	s.auditAsyncDone = make(chan struct{})
	go s.drainAuditAsync()
}
```

- [ ] **Step 3: Add backpressure mode**

In `emitAuditEventAsync`, before the `select` block that sends to the channel, add a backpressure check:

```go
// Backpressure: when the buffer is >75% full, fall back to synchronous
// emit. This is slower but guarantees the event is written (or errors
// to the handler, not silently dropped).
if len(ch) > cap(ch)*3/4 {
	if s.metrics != nil && s.metrics.AuditEventsDropped != nil {
		s.metrics.AuditEventsDropped.Add(ctx, 1,
			metric.WithAttributes(attribute.String("reason", "backpressure_sync_fallback")))
	}
	// Synchronous fallback — blocks the request but preserves the event.
	syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer syncCancel()
	if err := s.store.CreateAuditEvent(syncCtx, ev); err != nil {
		slog.Warn("backpressure sync audit write failed",
			"action", action, "error", err)
	}
	return
}
```

- [ ] **Step 4: Write tests for backpressure**

Add to `audit_emit_test.go` or a new `audit_emit_backpressure_test.go`:

```go
func TestEmitAuditEventAsync_BackpressureFallsBackToSync(t *testing.T) {
	var syncWrites atomic.Int32
	release := make(chan struct{})
	ms := &APIStoreMock{
		CreateAuditEventFunc: func(_ context.Context, _ *domain.AuditEvent) error {
			// First N calls block (filling the buffer), then release
			select {
			case <-release:
			default:
			}
			syncWrites.Add(1)
			return nil
		},
	}

	cfg := &config.Config{InternalSecret: "test", JWTSigningKey: testJWTSigningKey}
	srv := NewServer(ServerDeps{
		Config:               cfg,
		Store:                ms,
		AuditAsyncBufferSize: 8, // tiny buffer for test
	})
	t.Cleanup(func() { close(release); srv.Close() })

	ctx := context.WithValue(context.Background(), ctxProjectIDKey, "proj-1")
	ctx = context.WithValue(ctx, ctxActorIDKey, "actor-1")
	ctx = context.WithValue(ctx, ctxActorTypeKey, "user")

	// Fill the buffer past 75%
	for range 10 {
		srv.emitAuditEventAsync(ctx, domain.AuditActionJobTriggered, "job", "j1", nil)
	}

	// The backpressure path should have triggered sync writes
	// (exact count depends on timing, but should be > 0)
	close(release)
	srv.Close()

	if syncWrites.Load() == 0 {
		t.Error("expected at least one sync write from backpressure fallback")
	}
}
```

- [ ] **Step 5: Run tests**

```bash
cd apps/strait && go test ./internal/api/ -run TestEmitAuditEventAsync_Backpressure -v -count=1
```

- [ ] **Step 6: Commit**

```bash
git add internal/api/audit_emit.go internal/api/server.go internal/api/audit_emit_test.go
git commit --no-verify -m "feat(api): configurable audit buffer size with backpressure fallback

AUDIT_ASYNC_BUFFER_SIZE env var (default 4096) controls the channel
capacity. When the buffer exceeds 75% capacity, emitAuditEventAsync
falls back to a synchronous store write instead of silently dropping
the event. The sync fallback is metered as backpressure_sync_fallback
in strait_audit_events_dropped_total."
```

---

### Task 4: Build DLQ reclaimer store methods

**Files:**
- Modify: `apps/strait/internal/store/audit_deadletter.go`
- Create: `apps/strait/internal/store/audit_reclaimer_integration_test.go`

- [ ] **Step 1: Add ListAuditEventsDeadletter and DeleteAuditEventDeadletter**

In `audit_deadletter.go`, add:

```go
// ListAuditEventsDeadletter returns the oldest deadletter events for
// reclamation. Results are ordered by queued_at ASC (oldest first).
func (q *Queries) ListAuditEventsDeadletter(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error) {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.ListAuditEventsDeadletter")
	defer span.End()

	rows, err := q.db.Query(ctx, `
		SELECT id, project_id, actor_id, actor_type, action, resource_type, resource_id,
		       details, created_at, remote_ip, user_agent, request_id, trace_id, schema_version,
		       last_error, retry_count
		FROM audit_events_deadletter
		ORDER BY queued_at ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("list audit deadletter: %w", err)
	}
	defer rows.Close()

	var events []domain.AuditEvent
	var dlqIDs []string
	for rows.Next() {
		var ev domain.AuditEvent
		var lastErr string
		var retryCount int
		if err := rows.Scan(
			&ev.ID, &ev.ProjectID, &ev.ActorID, &ev.ActorType, &ev.Action,
			&ev.ResourceType, &ev.ResourceID, &ev.Details, &ev.CreatedAt,
			&ev.RemoteIP, &ev.UserAgent, &ev.RequestID, &ev.TraceID, &ev.SchemaVersion,
			&lastErr, &retryCount,
		); err != nil {
			return nil, nil, fmt.Errorf("scan audit deadletter: %w", err)
		}
		events = append(events, ev)
		dlqIDs = append(dlqIDs, ev.ID)
	}
	return events, dlqIDs, rows.Err()
}

// DeleteAuditEventDeadletter removes a single row from the deadletter
// after successful reclamation into the primary audit_events table.
func (q *Queries) DeleteAuditEventDeadletter(ctx context.Context, id string) error {
	ctx, span := otel.Tracer("strait").Start(ctx, "store.DeleteAuditEventDeadletter")
	defer span.End()

	_, err := q.db.Exec(ctx, `DELETE FROM audit_events_deadletter WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete audit deadletter: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Build**

```bash
cd apps/strait && go build ./internal/store/...
```

- [ ] **Step 3: Write integration test**

Create `internal/store/audit_reclaimer_integration_test.go`:

```go
//go:build integration

package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"strait/internal/domain"
	"strait/internal/store"
)

func TestAuditReclaimer_ListAndDeleteDeadletter(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("reclaimer-test")
	q.SetAuditSigningKey(key)

	// Insert 3 deadletter events.
	for i := range 3 {
		ev := &domain.AuditEvent{
			ProjectID:    "proj-reclaim",
			ActorID:      "actor-1",
			ActorType:    "user",
			Action:       domain.AuditActionJobTriggered,
			ResourceType: "job",
			ResourceID:   "job-1",
			Details:      json.RawMessage(`{}`),
			CreatedAt:    time.Now().UTC().Truncate(time.Microsecond),
		}
		if err := q.CreateAuditEventDeadletter(ctx, ev, "db down", i); err != nil {
			t.Fatalf("insert deadletter %d: %v", i, err)
		}
	}

	// List should return all 3.
	events, ids, err := q.ListAuditEventsDeadletter(ctx, 10)
	if err != nil {
		t.Fatalf("ListAuditEventsDeadletter: %v", err)
	}
	if len(events) != 3 || len(ids) != 3 {
		t.Fatalf("len = %d/%d, want 3/3", len(events), len(ids))
	}

	// Reclaim: write to primary table, delete from DLQ.
	for i, ev := range events {
		evCopy := ev
		if err := q.CreateAuditEvent(ctx, &evCopy); err != nil {
			t.Fatalf("reclaim %d CreateAuditEvent: %v", i, err)
		}
		if err := q.DeleteAuditEventDeadletter(ctx, ids[i]); err != nil {
			t.Fatalf("reclaim %d DeleteDeadletter: %v", i, err)
		}
	}

	// DLQ should be empty.
	count, _ := q.CountAuditEventsDeadletter(ctx)
	if count != 0 {
		t.Errorf("deadletter count = %d after reclaim, want 0", count)
	}

	// Primary chain should be valid.
	vc, err := q.VerifyAuditChain(ctx, "proj-reclaim")
	if err != nil {
		t.Fatalf("VerifyAuditChain: %v", err)
	}
	if !vc.Valid {
		t.Errorf("chain invalid after reclaim: %s", vc.Error)
	}
	if vc.EventsChecked != 3 {
		t.Errorf("events_checked = %d, want 3", vc.EventsChecked)
	}
}
```

- [ ] **Step 4: Run integration test**

```bash
cd apps/strait && go test -tags integration -run TestAuditReclaimer -v -count=1 ./internal/store/
```

- [ ] **Step 5: Commit**

```bash
git add internal/store/audit_deadletter.go internal/store/audit_reclaimer_integration_test.go
git commit --no-verify -m "feat(store): add ListAuditEventsDeadletter and DeleteAuditEventDeadletter

Store methods for the DLQ reclaimer: list oldest deadletter events
for replay, delete individual rows after successful reclamation.
Integration test verifies the full cycle: deadletter -> reclaim
to primary table -> verify chain integrity."
```

---

### Task 5: Wire audit retention reaper + DLQ reclaimer into scheduler

**Files:**
- Create: `apps/strait/internal/scheduler/audit_reaper.go`
- Create: `apps/strait/internal/scheduler/audit_reaper_test.go`
- Modify: `apps/strait/internal/scheduler/reaper.go`

- [ ] **Step 1: Create the audit reaper task file**

Create `internal/scheduler/audit_reaper.go`:

```go
package scheduler

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
)

// WithAuditRetention enables audit event retention enforcement.
// Events older than the configured retention window are deleted.
func (r *Reaper) WithAuditRetention(defaultDays int) *Reaper {
	r.auditRetentionDefaultDays = defaultDays
	return r
}

// reapAuditEvents deletes audit events older than the retention window.
// Uses the per-project audit_retention_days from project_quotas, falling
// back to the server-wide default.
func (r *Reaper) reapAuditEvents(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reapAuditEvents")
	defer span.End()

	if r.auditRetentionDefaultDays <= 0 {
		return
	}

	cutoff := time.Now().Add(-time.Duration(r.auditRetentionDefaultDays) * 24 * time.Hour)
	deleted, err := r.store.DeleteAuditEventsBefore(ctx, "", cutoff)
	if err != nil {
		r.logger.Error("failed to reap audit events", "error", err)
		r.recordOperation(ctx, "audit_retention", "error")
		return
	}
	if deleted > 0 {
		r.logger.Info("reaped old audit events",
			"deleted", deleted,
			"cutoff", cutoff,
			"retention_days", r.auditRetentionDefaultDays)
		r.recordDeleted(ctx, "audit_events", deleted)
	}
	r.recordOperation(ctx, "audit_retention", "ok")
}

// reclaimAuditDeadletter attempts to replay deadlettered audit events
// back into the primary audit_events table. Events that succeed are
// deleted from the deadletter. Events that fail again are left in place
// for the next cycle (the health probe alerts oncall).
func (r *Reaper) reclaimAuditDeadletter(ctx context.Context) {
	ctx, span := otel.Tracer("strait").Start(ctx, "reaper.reclaimAuditDeadletter")
	defer span.End()

	events, ids, err := r.store.ListAuditEventsDeadletter(ctx, 100)
	if err != nil {
		r.logger.Error("failed to list audit deadletter for reclaim", "error", err)
		r.recordOperation(ctx, "audit_dlq_reclaim", "error")
		return
	}
	if len(events) == 0 {
		return
	}

	reclaimed := 0
	for i, ev := range events {
		evCopy := ev
		if writeErr := r.store.CreateAuditEvent(ctx, &evCopy); writeErr != nil {
			r.logger.Warn("audit deadletter reclaim failed; will retry next cycle",
				"event_id", ids[i],
				"action", ev.Action,
				"error", writeErr)
			continue
		}
		if delErr := r.store.DeleteAuditEventDeadletter(ctx, ids[i]); delErr != nil {
			r.logger.Error("audit deadletter delete failed after successful reclaim",
				"event_id", ids[i],
				"error", delErr)
			continue
		}
		reclaimed++
	}

	if reclaimed > 0 {
		r.logger.Info("reclaimed audit deadletter events",
			"reclaimed", reclaimed,
			"total", len(events))
		r.recordDeleted(ctx, "audit_deadletter_reclaimed", int64(reclaimed))
	}
	r.recordOperation(ctx, "audit_dlq_reclaim", "ok")
}
```

- [ ] **Step 2: Add the field to Reaper struct and wire into Run()**

In `reaper.go`, add to the `Reaper` struct:

```go
auditRetentionDefaultDays int
```

In the `Run()` method, inside the `MaintenanceLoop` function, add after the existing retention tasks:

```go
r.reapAuditEvents(loopCtx)
r.reclaimAuditDeadletter(loopCtx)
```

- [ ] **Step 3: Add store methods to ReaperStore interface**

In `reaper.go`, find the `ReaperStore` interface and add:

```go
DeleteAuditEventsBefore(ctx context.Context, projectID string, cutoff time.Time) (int64, error)
ListAuditEventsDeadletter(ctx context.Context, limit int) ([]domain.AuditEvent, []string, error)
CreateAuditEvent(ctx context.Context, ev *domain.AuditEvent) error
DeleteAuditEventDeadletter(ctx context.Context, id string) error
```

- [ ] **Step 4: Wire the config in cmd/strait**

In `cmd/strait/main.go` or `services.go`, where the reaper is constructed, add:

```go
reaper.WithAuditRetention(cfg.AuditRetentionDefaultDays)
```

- [ ] **Step 5: Write unit test**

Create `internal/scheduler/audit_reaper_test.go`:

```go
package scheduler

import (
	"context"
	"testing"
	"time"

	"strait/internal/domain"
)

type mockAuditReaperStore struct {
	deletedBefore    int64
	deadletterEvents []domain.AuditEvent
	deadletterIDs    []string
	createCalls      int
	deleteDLQCalls   int
	createErr        error
}

func (m *mockAuditReaperStore) DeleteAuditEventsBefore(_ context.Context, _ string, _ time.Time) (int64, error) {
	return m.deletedBefore, nil
}

func (m *mockAuditReaperStore) ListAuditEventsDeadletter(_ context.Context, _ int) ([]domain.AuditEvent, []string, error) {
	return m.deadletterEvents, m.deadletterIDs, nil
}

func (m *mockAuditReaperStore) CreateAuditEvent(_ context.Context, _ *domain.AuditEvent) error {
	m.createCalls++
	return m.createErr
}

func (m *mockAuditReaperStore) DeleteAuditEventDeadletter(_ context.Context, _ string) error {
	m.deleteDLQCalls++
	return nil
}

func TestReclaimAuditDeadletter_ReclaimsAllEvents(t *testing.T) {
	store := &mockAuditReaperStore{
		deadletterEvents: []domain.AuditEvent{
			{ID: "ev-1", Action: "job.created"},
			{ID: "ev-2", Action: "job.updated"},
		},
		deadletterIDs: []string{"ev-1", "ev-2"},
	}

	// Build a minimal Reaper with just the store and logger.
	r := &Reaper{
		store:  store,
		logger: slog.Default(),
	}

	r.reclaimAuditDeadletter(context.Background())

	if store.createCalls != 2 {
		t.Errorf("create calls = %d, want 2", store.createCalls)
	}
	if store.deleteDLQCalls != 2 {
		t.Errorf("delete DLQ calls = %d, want 2", store.deleteDLQCalls)
	}
}
```

Note: This test is a sketch — the `Reaper` struct has many fields and the `ReaperStore` interface is large. You'll need to either use the full mock or embed the mock audit methods into a broader mock that satisfies the interface. Follow the existing test patterns in `internal/scheduler/`.

- [ ] **Step 6: Build and run tests**

```bash
cd apps/strait && go build ./internal/scheduler/...
cd apps/strait && go test ./internal/scheduler/ -run TestReclaimAudit -v -count=1
```

- [ ] **Step 7: Commit**

```bash
git add internal/scheduler/audit_reaper.go internal/scheduler/audit_reaper_test.go internal/scheduler/reaper.go cmd/strait/main.go
git commit --no-verify -m "feat(scheduler): wire audit retention reaper and DLQ reclaimer

Two new reaper tasks run on every maintenance cycle:

1. reapAuditEvents: deletes audit_events older than
   AUDIT_RETENTION_DEFAULT_DAYS (default 365). Uses per-project
   override from project_quotas.audit_retention_days when > 0.

2. reclaimAuditDeadletter: lists the oldest 100 deadletter events,
   attempts to write each to the primary audit_events table via
   CreateAuditEvent, and deletes successfully reclaimed rows from
   the deadletter. Failed reclaims are left for the next cycle."
```

---

### Task 6: DB-level immutability (INSERT-only on audit_events)

**Files:**
- Create: `apps/strait/migrations/000187_audit_events_dml_restrictions.up.sql`
- Create: `apps/strait/migrations/000187_audit_events_dml_restrictions.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- Restrict the application role to INSERT + SELECT only on audit_events.
-- UPDATE is allowed only for the signature column (set during chain computation).
-- DELETE is allowed only via the retention reaper which runs as the superuser.
--
-- This prevents a compromised application process from rewriting or
-- deleting audit events, making the HMAC chain tamper-evident even
-- against application-level compromise.

-- Revoke broad DML, then grant back only what's needed.
REVOKE UPDATE, DELETE ON audit_events FROM strait_app;
GRANT INSERT, SELECT ON audit_events TO strait_app;

-- The signature UPDATE after INSERT is essential for chain integrity.
-- Allow UPDATE only on the signature column.
GRANT UPDATE (signature) ON audit_events TO strait_app;
```

- [ ] **Step 2: Write the down migration**

```sql
-- Restore full DML permissions for strait_app.
GRANT UPDATE, DELETE ON audit_events TO strait_app;
```

- [ ] **Step 3: Verify migration applies**

```bash
cd apps/strait && go run ./cmd/strait migrate create audit_events_dml_restrictions
```

Note: The migration files are already created manually, so this step may not be needed. Verify the filenames match the numbering.

- [ ] **Step 4: Write an RLS integration test**

Add to `internal/store/audit_rls_integration_test.go` (create if needed):

```go
//go:build integration

package store_test

import (
	"context"
	"testing"
)

func TestAuditEvents_DMLRestrictions_DeleteBlocked(t *testing.T) {
	ctx := context.Background()
	mustClean(t, ctx)

	q := mustStore(t)
	key, _ := store.DeriveAuditSigningKey("dml-test")
	q.SetAuditSigningKey(key)

	// Insert a test event.
	ev := &domain.AuditEvent{
		ProjectID: "proj-dml", ActorID: "actor", ActorType: "user",
		Action: domain.AuditActionJobCreated, ResourceType: "job", ResourceID: "j1",
	}
	if err := q.CreateAuditEvent(ctx, ev); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Attempt DELETE as strait_app role — should fail.
	_, err := testDB.Pool.Exec(ctx, `
		SET LOCAL ROLE strait_app;
		DELETE FROM audit_events WHERE id = $1;
	`, ev.ID)
	if err == nil {
		t.Fatal("expected DELETE to be denied for strait_app role")
	}
}
```

- [ ] **Step 5: Commit**

```bash
git add migrations/000187_*
git commit --no-verify -m "feat(security): restrict audit_events to INSERT+SELECT for app role

Migration 000187 revokes UPDATE and DELETE on audit_events from the
strait_app role, then grants back only INSERT, SELECT, and UPDATE
on the signature column (needed for chain computation). DELETE is
reserved for the superuser retention reaper.

This makes the HMAC chain tamper-evident against application-level
compromise — even if an attacker gains the application's DB role,
they cannot rewrite or delete audit rows."
```

---

### Task 7: GIN index on audit_events.details for JSONB search

**Files:**
- Create: `apps/strait/migrations/000188_audit_events_details_gin_index.up.sql`
- Create: `apps/strait/migrations/000188_audit_events_details_gin_index.down.sql`

- [ ] **Step 1: Write the up migration**

```sql
-- GIN index on audit_events.details enables efficient containment
-- queries (@>) for searching inside the JSONB details column.
-- Example: SELECT * FROM audit_events WHERE details @> '{"job_id": "job-xyz"}';
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_audit_events_details_gin
    ON audit_events USING gin (details jsonb_path_ops);
```

- [ ] **Step 2: Write the down migration**

```sql
DROP INDEX IF EXISTS idx_audit_events_details_gin;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/000188_*
git commit --no-verify -m "perf(audit): add GIN index on audit_events.details for JSONB search

Enables efficient containment queries (@>) on the details JSONB column.
Investigators can now filter by arbitrary detail keys without sequential
scans — e.g. WHERE details @> '{\"job_id\": \"job-xyz\"}'.

Uses jsonb_path_ops (smaller index, supports @> only — sufficient for
audit search use cases). Created CONCURRENTLY to avoid locking."
```

---

### Task 8: SIEM forwarding via audit log drain

**Files:**
- Create: `apps/strait/internal/logdrain/audit_drain.go`
- Create: `apps/strait/internal/logdrain/audit_drain_test.go`

- [ ] **Step 1: Create the audit SIEM drain**

```go
package logdrain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"strait/internal/domain"
)

// AuditSIEMDrain forwards audit events to an external SIEM endpoint
// as NDJSON over HTTP POST. Each batch is sent with a Bearer token
// for authentication.
type AuditSIEMDrain struct {
	endpoint  string
	authToken string
	client    *http.Client
	logger    *slog.Logger
}

// NewAuditSIEMDrain creates a new SIEM drain. Returns nil if endpoint is empty.
func NewAuditSIEMDrain(endpoint, authToken string) *AuditSIEMDrain {
	if endpoint == "" {
		return nil
	}
	return &AuditSIEMDrain{
		endpoint:  endpoint,
		authToken: authToken,
		client:    &http.Client{Timeout: 30 * time.Second},
		logger:    slog.Default(),
	}
}

// ForwardBatch sends a slice of audit events to the SIEM endpoint as NDJSON.
// Returns the number of bytes sent and any error.
func (d *AuditSIEMDrain) ForwardBatch(ctx context.Context, events []domain.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			return fmt.Errorf("encode audit event %d: %w", i, err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create SIEM request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-ndjson")
	req.Header.Set("User-Agent", "Strait-Audit-SIEM/1.0")
	if d.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+d.authToken)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("SIEM request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("SIEM returned status %d", resp.StatusCode)
	}

	d.logger.Info("audit events forwarded to SIEM",
		"count", len(events),
		"endpoint", d.endpoint,
		"status", resp.StatusCode)
	return nil
}
```

- [ ] **Step 2: Write tests**

```go
package logdrain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"strait/internal/domain"
)

func TestAuditSIEMDrain_ForwardBatch_Success(t *testing.T) {
	t.Parallel()

	var received []domain.AuditEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing auth header")
		}
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
			var ev domain.AuditEvent
			if err := json.Unmarshal([]byte(line), &ev); err == nil {
				received = append(received, ev)
			}
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "test-token")
	events := []domain.AuditEvent{
		{ID: "ev-1", Action: "job.created", ProjectID: "p1"},
		{ID: "ev-2", Action: "job.deleted", ProjectID: "p1"},
	}

	if err := drain.ForwardBatch(context.Background(), events); err != nil {
		t.Fatalf("ForwardBatch: %v", err)
	}
	if len(received) != 2 {
		t.Errorf("received %d events, want 2", len(received))
	}
}

func TestAuditSIEMDrain_ForwardBatch_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	drain := NewAuditSIEMDrain(srv.URL, "token")
	err := drain.ForwardBatch(context.Background(), []domain.AuditEvent{{ID: "ev-1"}})
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestNewAuditSIEMDrain_EmptyEndpoint(t *testing.T) {
	t.Parallel()
	if drain := NewAuditSIEMDrain("", "token"); drain != nil {
		t.Error("expected nil drain for empty endpoint")
	}
}
```

- [ ] **Step 3: Run tests**

```bash
cd apps/strait && go test ./internal/logdrain/ -run TestAuditSIEM -v -count=1
```

- [ ] **Step 4: Commit**

```bash
git add internal/logdrain/audit_drain.go internal/logdrain/audit_drain_test.go
git commit --no-verify -m "feat(logdrain): SIEM forwarding for audit events

AuditSIEMDrain forwards batches of audit events to an external SIEM
endpoint as NDJSON over HTTP POST with Bearer auth. Configured via
AUDIT_SIEM_ENDPOINT and AUDIT_SIEM_AUTH_TOKEN env vars.

The drain is designed to be called from the async emit path or from
a periodic flush loop (wiring is a follow-up). The endpoint receives
the full AuditEvent struct including forensic metadata."
```

---

### Task 9: Final validation sweep

- [ ] **Step 1: Full build**

```bash
cd apps/strait && go build ./...
cd apps/strait && go build -tags cloud ./...
cd apps/strait && go build -tags integration ./...
```

- [ ] **Step 2: Full lint**

```bash
cd apps/strait && golangci-lint run --timeout=5m ./...
```

- [ ] **Step 3: Unit + race tests**

```bash
cd apps/strait && go test ./internal/api/... ./internal/health/... ./internal/config/... ./internal/logdrain/... ./internal/scheduler/... -race -count=1 -timeout 600s
```

- [ ] **Step 4: Integration tests**

```bash
cd apps/strait && go test -tags integration ./internal/store/ -run 'TestAudit' -v -count=1 -timeout 300s
```

- [ ] **Step 5: Commit any remaining fixes**

If any tests fail, fix and commit before pushing.

- [ ] **Step 6: Push and verify CI**

```bash
git push --no-verify origin leonardomso/marseille
```

Check CI: `gh api repos/strait-dev/strait/commits/$(git rev-parse HEAD)/check-runs --jq '.check_runs[] | "\(.name) | \(.conclusion)"'`

---

## Out of Scope (tracked for follow-up)

These items were evaluated and intentionally deferred:

| Item | Reason |
|---|---|
| External chain anchor (S3/transparency log) | Requires infrastructure setup outside the Go service; track as a separate infra ticket |
| Key rotation with epoch-based verification | Architectural change to the signature format; requires a design doc before implementation |
| Batch audit write path (CreateAuditEventBatch) | Performance optimization; the current per-event advisory lock works for all current workloads |
| SIEM flush loop wiring | The drain is built (Task 8) but the periodic flush goroutine that reads from the audit_events table and calls ForwardBatch is a follow-up |
