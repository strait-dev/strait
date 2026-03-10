# Event Triggers — Production Readiness Plan

13 items across 4 priority tiers. **All 13 items are now implemented.**

Each item specifies exact files, interfaces, SQL, tests, and acceptance criteria.

**Constraints**: Same as the original PR — `go build ./...`, `go vet ./...`, `golangci-lint run --timeout=5m ./...`, `go test -race ./...` must pass after each item. Commit after each item.

**Status**: ✅ All items complete. CI green.

---

## P0 — Before First Deploy (Items 1–3)

These items are **one-time opportunities** that cannot be done after the first production migration runs, or address input validation gaps that could cause data corruption.

---

### Item 1: Migration Squash (000049–000055 → single 000049)

**Why**: Seven sequential ALTERs for one feature adds startup time, rollback complexity, and cognitive overhead. Must be done before any production database runs these migrations.

**Files**:
- `migrations/000049_event_trigger_support.up.sql` — rewrite with final schema
- `migrations/000049_event_trigger_support.down.sql` — rewrite with full teardown
- Delete: `migrations/00005{0,1,2,3,4,5}_*` (12 files)

**New `000049_event_trigger_support.up.sql`**:
```sql
-- Event trigger fields on workflow_steps
ALTER TABLE workflow_steps
  ADD COLUMN event_key TEXT,
  ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600,
  ADD COLUMN event_notify_url TEXT,
  ADD COLUMN event_emit_key TEXT,
  ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;

-- Mirror on versioned steps
ALTER TABLE workflow_version_steps
  ADD COLUMN event_key TEXT,
  ADD COLUMN event_timeout_secs INT NOT NULL DEFAULT 3600,
  ADD COLUMN event_notify_url TEXT,
  ADD COLUMN event_emit_key TEXT,
  ADD COLUMN sleep_duration_secs INT NOT NULL DEFAULT 0;

-- Event triggers table
CREATE TABLE event_triggers (
    id                    TEXT        PRIMARY KEY,
    event_key             TEXT        NOT NULL UNIQUE,
    project_id            TEXT        NOT NULL,
    source_type           TEXT        NOT NULL,
    trigger_type          TEXT        NOT NULL DEFAULT 'event',
    workflow_run_id       TEXT,
    workflow_step_run_id  TEXT,
    job_run_id            TEXT,
    status                TEXT        NOT NULL DEFAULT 'waiting',
    request_payload       JSONB,
    response_payload      JSONB,
    timeout_secs          INT         NOT NULL DEFAULT 3600,
    requested_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at           TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ NOT NULL,
    error                 TEXT,
    notify_url            TEXT,
    notify_status         TEXT        NOT NULL DEFAULT '',
    event_emit_key        TEXT,
    sent_by               TEXT        NOT NULL DEFAULT '',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Reaper: find expired waiting triggers
CREATE INDEX idx_event_triggers_status_expires
    ON event_triggers(status, expires_at) WHERE status = 'waiting' AND expires_at IS NOT NULL;

-- Lookup by step/job/workflow run
CREATE INDEX idx_event_triggers_step_run
    ON event_triggers(workflow_step_run_id) WHERE workflow_step_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_job_run
    ON event_triggers(job_run_id) WHERE job_run_id IS NOT NULL;
CREATE INDEX idx_event_triggers_project
    ON event_triggers(project_id, status);

-- Cancel triggers when workflow is canceled/timed out
CREATE INDEX idx_event_triggers_workflow_run
    ON event_triggers(workflow_run_id, status) WHERE status = 'waiting';

-- Reconciliation: find received triggers with stale steps
CREATE INDEX idx_event_triggers_reconcile
    ON event_triggers(status, source_type, received_at) WHERE status = 'received';

-- Prefix matching for batch send
CREATE INDEX idx_event_triggers_event_key_prefix
    ON event_triggers(event_key text_pattern_ops);

-- Extend webhook_deliveries for event trigger notifications
ALTER TABLE webhook_deliveries
  ADD COLUMN IF NOT EXISTS event_trigger_id TEXT REFERENCES event_triggers(id) ON DELETE CASCADE;
ALTER TABLE webhook_deliveries ALTER COLUMN run_id DROP NOT NULL;
ALTER TABLE webhook_deliveries ALTER COLUMN job_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_webhook_deliveries_pending_retry
    ON webhook_deliveries(next_retry_at) WHERE status = 'pending' AND next_retry_at IS NOT NULL;
```

**New `000049_event_trigger_support.down.sql`**:
```sql
DROP INDEX IF EXISTS idx_webhook_deliveries_pending_retry;
ALTER TABLE webhook_deliveries DROP COLUMN IF EXISTS event_trigger_id;
-- Note: NOT restoring NOT NULL on run_id/job_id — down migration
-- for a feature removal, not a rollback to mid-feature state.

DROP TABLE IF EXISTS event_triggers;

ALTER TABLE workflow_steps
  DROP COLUMN IF EXISTS event_key,
  DROP COLUMN IF EXISTS event_timeout_secs,
  DROP COLUMN IF EXISTS event_notify_url,
  DROP COLUMN IF EXISTS event_emit_key,
  DROP COLUMN IF EXISTS sleep_duration_secs;

ALTER TABLE workflow_version_steps
  DROP COLUMN IF EXISTS event_key,
  DROP COLUMN IF EXISTS event_timeout_secs,
  DROP COLUMN IF EXISTS event_notify_url,
  DROP COLUMN IF EXISTS event_emit_key,
  DROP COLUMN IF EXISTS sleep_duration_secs;
```

**Tests**: Run full E2E suite to verify migrations apply cleanly on fresh database.

**Acceptance**: `migrate up` from 000048 applies in a single migration. `migrate down` removes all event trigger objects. All existing tests pass.

---

### Item 2: Event Key Input Validation

**Why**: Event keys are UNIQUE indexed strings that flow into SQL LIKE queries and Redis channel names. Malformed keys could cause query issues or injection.

**Files**:
- `internal/api/event_triggers.go` — add `validateEventKey()` helper
- `internal/api/sdk_wait_event.go` — call `validateEventKey()`
- `internal/workflow/engine_steps.go` — call `validateEventKey()` on rendered key
- `internal/api/event_triggers_test.go` — table-driven tests
- `internal/workflow/engine_test.go` — rejection test

**New helper** (`internal/api/event_triggers.go`):
```go
// validateEventKey returns an error string if the key is invalid, empty string if OK.
func validateEventKey(key string) string {
    if len(key) > 512 {
        return "event key must be at most 512 characters"
    }
    if len(key) == 0 {
        return "event key is required"
    }
    for i := 0; i < len(key); i++ {
        if key[i] < 0x20 { // control characters including \x00
            return "event key contains invalid characters (control characters not allowed)"
        }
    }
    return ""
}
```

**Call sites**:
1. `handleSendEvent` — validate `eventKey` URL param
2. `handleSendEventByPrefix` — validate `prefix` field
3. `handleSDKWaitForEvent` — validate `req.EventKey`
4. `startWaitForEventStep` — validate rendered `eventKey` before CreateEventTrigger

**Tests** (table-driven in `event_triggers_test.go`):
```
- empty string → error
- 513 chars → error  
- contains \x00 → error
- contains \n → error
- "valid:key-123" → ok
- "aml:{{resolved}}" → ok (after template rendering)
- 512 chars exactly → ok
```

**Acceptance**: Keys with control characters rejected with 400. Keys over 512 chars rejected. All existing tests pass.

---

### Item 3: Index Benchmarking with EXPLAIN ANALYZE

**Why**: The reconciliation query joins `event_triggers` with `workflow_step_runs`/`job_runs`. Without benchmarking, we don't know if the indexes are effective at scale.

**Files**:
- `internal/e2e/event_triggers_bench_test.go` — new benchmark file

**Benchmark test** (runs against real Postgres via testcontainers):
```go
func BenchmarkListExpiredEventTriggers(b *testing.B) {
    // Seed 10,000 waiting triggers with varying expiry times
    // Run ListExpiredEventTriggers in a loop
    // Report ns/op and allocs/op
}

func BenchmarkListReceivedWithStaleSteps(b *testing.B) {
    // Seed 10,000 received triggers with matching step_runs
    // Run ListReceivedEventTriggersWithStaleSteps in a loop
}

func BenchmarkListByKeyPrefix(b *testing.B) {
    // Seed 10,000 triggers with common prefix
    // Run ListEventTriggersByKeyPrefix with LIKE 'prefix%'
}
```

**Manual step** (not automated, run once on staging):
```sql
-- Run these with \timing on and EXPLAIN (ANALYZE, BUFFERS):
EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT * FROM event_triggers
WHERE status = 'waiting' AND expires_at <= NOW()
ORDER BY expires_at ASC LIMIT 1000;

EXPLAIN (ANALYZE, BUFFERS, FORMAT TEXT)
SELECT et.* FROM event_triggers et
LEFT JOIN workflow_step_runs wsr ON et.workflow_step_run_id = wsr.id
LEFT JOIN job_runs jr ON et.job_run_id = jr.id
WHERE et.status = 'received'
  AND et.received_at < NOW() - INTERVAL '30 seconds'
  AND (
    (et.source_type = 'workflow_step' AND wsr.status = 'waiting')
    OR (et.source_type = 'job_run' AND jr.status = 'waiting')
  );
```

**Acceptance**: Benchmark tests run without error. EXPLAIN ANALYZE shows index scans (not seq scans) on tables with 10K+ rows. Document results in commit message.

---

## P1 — First Week in Production (Items 4–5)

These items improve reliability for the first production deployment.

---

### Item 4: Transaction Safety for handleSendEvent (Workflow Step Path)

**Why**: The workflow step path updates trigger status, then calls `resumeEventSource` → `OnEventReceived` outside a transaction. If the process crashes between these operations, the reconciliation reaper catches it (30s delay). For sub-second recovery, wrap both in a single transaction.

**Files**:
- `internal/api/server.go` — add `TxBeginner` to `ServerDeps`
- `internal/api/event_triggers.go` — use `WithTx` in `handleSendEvent`
- `internal/store/store.go` — no changes (already exports `WithTx`, `TxBeginner`)
- `internal/api/mock_test.go` — add `txBeginner` mock field
- `internal/api/event_triggers_test.go` — test transactional path
- `cmd/strait/services.go` — pass `dbPool` (which is a `TxBeginner`) to server

**Approach**:

Add `TxPool` to `ServerDeps` and `Server`:
```go
type ServerDeps struct {
    // ... existing fields
    TxPool store.TxBeginner // pgxpool.Pool — for transactional API operations
}

type Server struct {
    // ... existing fields
    txPool store.TxBeginner
}
```

Rewrite the workflow step path in `handleSendEvent`:
```go
if trigger.SourceType == domain.EventSourceWorkflowStep {
    if s.txPool != nil {
        err = store.WithTx(ctx, s.txPool, func(txQ *store.Queries) error {
            if err := txQ.UpdateEventTriggerStatus(ctx, trigger.ID,
                domain.EventTriggerStatusReceived, req.Payload, &now, ""); err != nil {
                return err
            }
            // Update step status within same tx
            if trigger.WorkflowStepRunID != "" {
                return txQ.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID,
                    domain.StepCompleted, map[string]any{
                        "output":      req.Payload,
                        "finished_at": now,
                    })
            }
            return nil
        })
    } else {
        // Fallback: sequential (existing behavior)
        err = s.store.UpdateEventTriggerStatus(...)
    }
    // Workflow fan-in/progression still happens outside tx
    // (involves multiple tables, queues, and callbacks)
    if err == nil && s.workflowCallback != nil {
        trigger.Status = domain.EventTriggerStatusReceived
        trigger.ResponsePayload = req.Payload
        trigger.ReceivedAt = &now
        s.workflowCallback.OnEventReceived(ctx, trigger)
    }
}
```

**Key insight**: Only the trigger update + step completion go in the transaction. The fan-in/progression callback stays outside because it touches many tables and may enqueue jobs.

**Wire-up** in `cmd/strait/services.go`:
```go
srv := api.NewServer(api.ServerDeps{
    // ... existing
    TxPool: dbPool, // pgxpool.Pool implements store.TxBeginner
})
```

**Tests**:
1. `TestHandleSendEvent_TransactionalWorkflowStep` — verify both trigger + step are updated
2. `TestHandleSendEvent_TxPoolNil_FallsBack` — verify existing behavior when TxPool is nil
3. `TestHandleSendEvent_TxRollbackOnStepError` — verify trigger NOT updated when step update fails

**Acceptance**: Trigger update and step completion are atomic. If step update fails, trigger stays in `waiting`. Fan-in still works. All existing tests pass.

---

### Item 5: SSE Auth for Browser Clients

**Why**: Browser `EventSource` API doesn't support custom headers. The SSE stream endpoint currently requires `Authorization` header, making it unusable from browser JavaScript.

**Files**:
- `internal/api/event_trigger_stream.go` — accept query param token
- `internal/api/middleware.go` — add `sseTokenAuth` middleware variant
- `internal/api/routes.go` — apply to SSE route
- `internal/api/event_triggers_test.go` — test query param auth

**Approach**:

Add a middleware that extracts auth from query param for SSE routes only:
```go
// sseTokenAuth extracts auth token from ?token= query param for SSE endpoints
// where browsers cannot set custom headers (EventSource API limitation).
func (s *Server) sseTokenAuth(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // If Authorization header is already set, use standard auth
        if r.Header.Get("Authorization") != "" || r.Header.Get("X-Internal-Secret") != "" {
            next.ServeHTTP(w, r)
            return
        }
        // Fall back to query param
        token := r.URL.Query().Get("token")
        if token != "" {
            r.Header.Set("Authorization", "Bearer "+token)
        }
        next.ServeHTTP(w, r)
    })
}
```

Apply to SSE route in `routes.go`:
```go
r.With(s.sseTokenAuth).Get("/stream", s.handleEventTriggerStream)
```

**Tests**:
1. `TestEventTriggerStream_QueryParamAuth` — `?token=strait_key` works
2. `TestEventTriggerStream_HeaderAuthStillWorks` — backward compatible
3. `TestEventTriggerStream_NoAuthReturns401` — no header, no param → 401

**Acceptance**: Browser `EventSource` can connect with `?token=strait_...`. Header auth still works. No auth = 401.

---

## P2 — First Month (Items 6–9)

These items address operational visibility, SDK ergonomics, and production tuning.

---

### Item 6: SDK Language Bindings

**Why**: Without typed SDK methods, users must make raw HTTP calls to `wait-for-event`. TypeScript and Python are the primary SDK languages.

**Files**:
- `sdk/typescript/src/strait.ts` — add `waitForEvent()` method
- `sdk/python/strait/client.py` — add `wait_for_event()` method
- `sdk/typescript/src/types.ts` — add `WaitForEventOptions` type
- `sdk/python/strait/types.py` — add dataclass

**TypeScript**:
```typescript
interface WaitForEventOptions {
  eventKey: string;
  timeoutSecs?: number;
  notifyUrl?: string;
}

interface EventTrigger {
  id: string;
  event_key: string;
  status: string;
  // ... all fields from domain.EventTrigger
}

class StraitSDK {
  async waitForEvent(runId: string, options: WaitForEventOptions): Promise<EventTrigger> {
    const resp = await this.post(`/sdk/v1/runs/${runId}/wait-for-event`, {
      event_key: options.eventKey,
      timeout_secs: options.timeoutSecs ?? 3600,
      notify_url: options.notifyUrl,
    });
    return resp.data;
  }
}
```

**Python**:
```python
@dataclass
class WaitForEventOptions:
    event_key: str
    timeout_secs: int = 3600
    notify_url: str | None = None

class StraitSDK:
    def wait_for_event(self, run_id: str, options: WaitForEventOptions) -> EventTrigger:
        resp = self._post(f"/sdk/v1/runs/{run_id}/wait-for-event", {
            "event_key": options.event_key,
            "timeout_secs": options.timeout_secs,
            "notify_url": options.notify_url,
        })
        return EventTrigger(**resp)
```

**Tests**: Unit tests in each SDK's test suite mocking HTTP responses.

**Acceptance**: Both SDKs export typed `waitForEvent` method. Tests pass. README updated with usage example.

---

### Item 7: Event Trigger Retention Tuning

**Why**: The current retention default is 30 days for all terminal triggers. In production, `received` triggers (happy path) can be cleaned up faster than `timed_out`/`canceled` (useful for debugging).

**Files**:
- `internal/config/config.go` — add `EventTriggerRetentionReceivedDays`, `EventTriggerRetentionFailedDays`
- `internal/scheduler/reaper.go` — split `reapOldEventTriggers` into two passes
- `internal/store/event_triggers.go` — add status filter to `DeleteEventTriggersFinishedBefore`
- `internal/scheduler/reaper_test.go` — test split retention
- `docs/configuration/environment-variables.mdx` — document new vars

**New config vars**:
```go
EventTriggerRetentionReceivedDays int `mapstructure:"EVENT_TRIGGER_RETENTION_RECEIVED_DAYS"` // default 7
EventTriggerRetentionFailedDays   int `mapstructure:"EVENT_TRIGGER_RETENTION_FAILED_DAYS"`   // default 30
```

**Updated store method**:
```go
func (q *Queries) DeleteEventTriggersFinishedBefore(
    ctx context.Context, before time.Time, statuses []string, limit int,
) (int64, error)
```

**Reaper split**:
```go
func (r *Reaper) reapOldEventTriggers(ctx context.Context) {
    // Clean received triggers (short retention)
    r.deleteOldTriggers(ctx, r.receivedRetention, []string{"received"})
    // Clean failed triggers (long retention)
    r.deleteOldTriggers(ctx, r.failedRetention, []string{"timed_out", "canceled"})
}
```

**Tests**:
1. Received triggers deleted after short retention, failed triggers kept
2. Failed triggers deleted after long retention
3. Zero retention disables cleanup (existing behavior preserved)

**Acceptance**: Separate retention periods for received vs failed triggers. Existing `EVENT_TRIGGER_RETENTION_DAYS` still works as fallback.

---

### Item 8: Webhook DLQ Visibility

**Why**: Dead-lettered webhook deliveries sit in the database with no way to inspect or retry them via API.

**Files**:
- `internal/api/webhook_dlq.go` — new handler file
- `internal/api/routes.go` — register routes
- `internal/store/webhook_deliveries.go` — add `RetryWebhookDelivery` method
- `internal/api/server.go` — add to `APIStore` interface
- `internal/api/mock_test.go` — add mock methods
- `internal/api/webhook_dlq_test.go` — test file
- `cmd/strait/webhooks.go` — new CLI command file
- `internal/cli/client/api.go` — add client methods
- `docs/openapi.yaml`, `internal/api/openapi.yaml` — add endpoints

**API endpoints**:

```
GET  /v1/webhooks/deliveries?status=dead&limit=50
     → List webhook deliveries, filterable by status
     → Requires API key auth (project scoped)

POST /v1/webhooks/deliveries/{id}/retry
     → Reset a dead delivery to pending with attempt=0
     → The delivery worker picks it up on next poll
     → Returns 409 if delivery is not in 'dead' status
```

**Store method**:
```go
func (q *Queries) RetryWebhookDelivery(ctx context.Context, id string) error {
    query := `
        UPDATE webhook_deliveries
        SET status = 'pending', attempts = 0, next_retry_at = NOW(), updated_at = NOW()
        WHERE id = $1 AND status = 'dead'`
    tag, err := q.db.Exec(ctx, query, id)
    if tag.RowsAffected() == 0 {
        return fmt.Errorf("delivery %s is not in dead status", id)
    }
    return err
}
```

**CLI commands**:
```bash
strait webhooks list-dead --project proj_1
strait webhooks retry <delivery-id>
```

**Tests**:
1. List dead deliveries returns only dead status
2. Retry resets to pending
3. Retry on non-dead returns 409
4. Project scoping enforced

**Acceptance**: Dead deliveries visible and retryable. CLI commands work. Delivery worker picks up retried deliveries.

---

### Item 9: Reaper Load Testing

**Why**: The reaper runs every 30 seconds with `LIMIT 1000`. Under heavy load after an outage, thousands of triggers may expire simultaneously, creating a thundering herd of workflow progressions.

**Files**:
- `internal/e2e/reaper_load_test.go` — new load test file
- `internal/scheduler/reaper.go` — add backpressure if needed

**Load test** (tagged `// +build load`, not run in CI):
```go
func TestReaperLoad_10KExpiredTriggers(t *testing.T) {
    // 1. Seed 10,000 waiting triggers with expires_at in the past
    // 2. Seed matching workflow_step_runs in 'waiting' status
    // 3. Run ReapOnce() and time it
    // 4. Verify all 10K triggers transitioned to timed_out
    // 5. Report: total time, triggers/sec, DB connections used
}

func TestReaperLoad_ConcurrentReapers(t *testing.T) {
    // 1. Seed 5,000 expired triggers
    // 2. Run 3 reapers concurrently
    // 3. Verify no double-processing (each trigger timed_out exactly once)
    // 4. Report: total time, conflicts detected
}
```

**Potential fix if load test reveals issues** — add staggering:
```go
func (r *Reaper) reapExpiredEventTriggers(ctx context.Context) {
    for {
        triggers, err := r.store.ListExpiredEventTriggers(ctx)
        if err != nil || len(triggers) == 0 {
            break
        }
        for _, t := range triggers {
            r.handleExpiredTrigger(ctx, t)
        }
        // If we got a full batch, there might be more — loop
        if len(triggers) < 1000 {
            break
        }
    }
}
```

**Acceptance**: 10K expired triggers processed in <30s (one reaper interval). No duplicate processing with concurrent reapers. If issues found, fix committed with benchmark results.

---

## P3 — Post-GA Improvements (Items 10–13)

These items are enhancements for scale, compliance, and advanced use cases.

---

### Item 10: Event Trigger Archival

**Why**: For compliance use cases (SOC2, HIPAA), event trigger records with `sent_by` audit data must be preserved beyond the retention period.

**Files**:
- `migrations/000050_event_trigger_archive.up.sql` — archive table
- `internal/store/event_trigger_archive.go` — archive store methods
- `internal/scheduler/reaper.go` — archive before delete
- `internal/api/event_trigger_archive.go` — read-only API
- `internal/config/config.go` — `FF_EVENT_TRIGGER_ARCHIVE`

**Migration**:
```sql
CREATE TABLE event_trigger_archive (
    id                    TEXT        PRIMARY KEY,
    event_key             TEXT        NOT NULL,
    project_id            TEXT        NOT NULL,
    source_type           TEXT        NOT NULL,
    trigger_type          TEXT        NOT NULL,
    status                TEXT        NOT NULL,
    sent_by               TEXT,
    requested_at          TIMESTAMPTZ NOT NULL,
    received_at           TIMESTAMPTZ,
    expires_at            TIMESTAMPTZ,
    error                 TEXT,
    archived_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_event_trigger_archive_project ON event_trigger_archive(project_id, archived_at);
```

**Reaper change**:
```go
func (r *Reaper) reapOldEventTriggers(ctx context.Context) {
    triggers := r.store.ListEventTriggersFinishedBefore(ctx, cutoff, limit)
    if r.archiveEnabled {
        r.store.ArchiveEventTriggers(ctx, triggers)
    }
    r.store.DeleteEventTriggersByIDs(ctx, triggerIDs)
}
```

**API endpoint**:
```
GET /v1/events/archive?project_id=...&from=...&to=...
```

**Tests**:
1. Triggers archived before deletion
2. Archive queryable by project and date range
3. Archive disabled when FF off — triggers just deleted

**Acceptance**: Terminal triggers archived before deletion. Archive queryable. Feature-flagged.

---

### Item 11: Wildcard Event Subscriptions

**Why**: Currently prefix matching only works on the send side. A trigger that can wait for any event matching a pattern enables fan-in from multiple sources.

**Files**:
- `internal/domain/types.go` — add `EventKeyPattern` field to `EventTrigger`
- `migrations/000050_event_key_pattern.up.sql` — add column
- `internal/store/event_triggers.go` — pattern matching query
- `internal/api/event_triggers.go` — match patterns on send
- `internal/workflow/engine_steps.go` — support `event_key_pattern` step field

**Design**:

Event triggers can have either `event_key` (exact) or `event_key_pattern` (glob):
```json
{
  "step_ref": "wait-any-approval",
  "type": "wait_for_event",
  "event_key_pattern": "approval:order-123:*"
}
```

When `handleSendEvent` receives an event for key `approval:order-123:manager`, it checks:
1. Exact match on `event_key` (existing)
2. Pattern match on `event_key_pattern` using SQL `LIKE` with glob-to-LIKE conversion

**Store query**:
```sql
SELECT * FROM event_triggers
WHERE status = 'waiting'
  AND (event_key = $1 OR $1 LIKE replace(replace(event_key_pattern, '*', '%'), '?', '_'))
LIMIT 100
```

**Constraints**:
- Pattern triggers don't have UNIQUE constraint on pattern (multiple can wait for same pattern)
- Only `*` (any) and `?` (single char) wildcards supported
- Event key pattern max length: 512 chars

**Tests**:
1. `*` matches any suffix
2. `?` matches single character
3. Exact key still takes priority
4. Multiple pattern triggers resolved on single event

**Acceptance**: Pattern-based triggers work. Exact keys take priority. Performance acceptable with index.

---

### Item 12: Event Trigger Quotas

**Why**: A project creating millions of waiting triggers could degrade database performance for all projects.

**Files**:
- `internal/store/event_triggers.go` — add `CountWaitingByProject`
- `internal/api/event_triggers.go` — check quota before creation
- `internal/api/sdk_wait_event.go` — check quota
- `internal/workflow/engine_steps.go` — check quota
- `internal/config/config.go` — `MaxWaitingTriggersPerProject`
- `internal/api/event_triggers_test.go` — quota exceeded test

**Config**:
```go
MaxWaitingTriggersPerProject int `mapstructure:"MAX_WAITING_TRIGGERS_PER_PROJECT"` // default 10000
```

**Store method**:
```go
func (q *Queries) CountWaitingEventTriggersByProject(ctx context.Context, projectID string) (int64, error) {
    var count int64
    err := q.db.QueryRow(ctx,
        `SELECT COUNT(*) FROM event_triggers WHERE project_id = $1 AND status = 'waiting'`,
        projectID).Scan(&count)
    return count, err
}
```

**Check in API** (before `CreateEventTrigger`):
```go
if s.config.MaxWaitingTriggersPerProject > 0 {
    count, err := s.store.CountWaitingEventTriggersByProject(ctx, projectID)
    if err == nil && count >= int64(s.config.MaxWaitingTriggersPerProject) {
        respondError(w, r, http.StatusTooManyRequests, "project has reached maximum waiting triggers limit")
        return
    }
}
```

**Tests**:
1. Under quota — trigger created
2. At quota — returns 429
3. Quota disabled (0) — no check
4. Terminal triggers don't count toward quota

**Acceptance**: Projects cannot exceed the configured limit. 429 returned with descriptive message. Quota of 0 disables check.

---

### Item 13: Distributed Reaper Coordination

**Why**: Multiple worker instances each run their own reaper, doing redundant work. `LIMIT 1000` and optimistic locking prevent double-processing but waste CPU.

**Files**:
- `internal/scheduler/reaper.go` — add advisory lock
- `internal/store/store.go` — add `TryAdvisoryLock` / `ReleaseAdvisoryLock`
- `internal/scheduler/reaper_test.go` — test lock acquisition

**Approach** — use PostgreSQL `pg_try_advisory_lock`:
```go
const reaperAdvisoryLockID = 8675309 // arbitrary unique ID

func (r *Reaper) ReapOnce(ctx context.Context) {
    // Try to acquire advisory lock — if another reaper holds it, skip this pass.
    acquired, err := r.store.TryAdvisoryLock(ctx, reaperAdvisoryLockID)
    if err != nil {
        r.logger.Warn("failed to acquire reaper lock", "error", err)
        return
    }
    if !acquired {
        r.logger.Debug("reaper lock held by another instance, skipping")
        return
    }
    defer r.store.ReleaseAdvisoryLock(ctx, reaperAdvisoryLockID)

    // ... existing reap passes
}
```

**Store methods**:
```go
func (q *Queries) TryAdvisoryLock(ctx context.Context, lockID int64) (bool, error) {
    var acquired bool
    err := q.db.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, lockID).Scan(&acquired)
    return acquired, err
}

func (q *Queries) ReleaseAdvisoryLock(ctx context.Context, lockID int64) error {
    _, err := q.db.Exec(ctx, `SELECT pg_advisory_unlock($1)`, lockID)
    return err
}
```

**Key considerations**:
- `pg_try_advisory_lock` is session-level — released when connection returns to pool
- Non-blocking — if lock is held, the reaper skips this pass (next pass in 30s)
- Graceful degradation — if lock query fails, reaper runs anyway (log warning)

**Tests**:
1. Single reaper acquires lock and runs
2. Second reaper skips when lock is held
3. Lock released after reap completes
4. Lock query failure → reaper still runs (fallback)

**Acceptance**: Only one reaper instance runs per 30-second interval across all workers. No double-processing. Graceful fallback on lock failure.

---

## Implementation Order

```
Item 1  (migration squash)     → must be first, before any deploy
Item 2  (key validation)       → quick, high safety value
Item 3  (index benchmark)      → informs Items 4 and 9
Item 4  (tx safety)            → reliability improvement
Item 5  (SSE browser auth)     → enables frontend integration
Item 6  (SDK bindings)         → developer experience
Item 7  (retention tuning)     → operational control
Item 8  (webhook DLQ)          → operational visibility
Item 9  (load testing)         → validates production readiness
Item 10 (archival)             → compliance
Item 11 (wildcards)            → advanced feature
Item 12 (quotas)               → abuse prevention
Item 13 (reaper coordination)  → efficiency at scale
```

## Validation After Each Item

```bash
go build ./...
go vet ./...
golangci-lint run --timeout=5m ./...
go test -race -count=1 -timeout=120s ./...
```

Commit with descriptive message. Push. Wait for CI green.
