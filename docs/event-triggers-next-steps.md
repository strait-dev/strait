# Event Triggers — Next Steps Implementation Plan

All 13 items organized into phases with exact file paths, method signatures, SQL queries, test strategies, and acceptance criteria.

---

## Phase A: Short-Term (Items 1–5) — Pre-Production Hardening

### Item 1: Integration / E2E Tests

**Goal:** Full end-to-end tests against a real Postgres database for the event trigger system.

**File:** `internal/e2e/event_triggers_e2e_test.go` (new)

**Test 1: `TestE2E_WaitForEventStep_CompletesViaAPI`**
- Create a workflow with a `wait_for_event` step (`event_key: "e2e-check:{{app_id}}"`, `event_timeout_secs: 300`)
- Trigger workflow with payload `{"app_id": "app-e2e-123"}`
- Assert step run status = `waiting`, workflow run status = `running`
- Assert event trigger exists via `GET /v1/events/e2e-check:app-e2e-123` with status `waiting`
- Send event via `POST /v1/events/e2e-check:app-e2e-123/send` with payload `{"result": "clear"}`
- Assert step run status = `completed` with output = `{"result": "clear"}`
- Assert workflow run status = `completed`
- Assert event trigger status = `received` with `received_at` set

**Test 2: `TestE2E_WaitForEventStep_TimeoutViaReaper`**
- Create a workflow with a `wait_for_event` step (`event_timeout_secs: 1`)
- Trigger workflow, assert step is `waiting`
- Directly update `expires_at` in DB to `NOW() - 1 minute` (simulate expiry)
- Instantiate a `Reaper` using the real `testStore` and call `reapExpiredEventTriggers` directly (export via a test helper method, or call the full `Run` loop once with a short context)
- Alternative: expose `ReapOnce(ctx)` method on `Reaper` that runs all reap functions once. Add to reaper:
  ```go
  // ReapOnce runs all reaper passes exactly once. Exported for integration tests.
  func (r *Reaper) ReapOnce(ctx context.Context) {
      r.reapStaleDequeued(ctx)
      r.reapStale(ctx)
      r.reapExpired(ctx)
      r.reapTimedOutWorkflows(ctx)
      r.reapExpiredApprovals(ctx)
      r.reapExpiredEventTriggers(ctx)
      r.reapOldWorkflowRuns(ctx)
  }
  ```
  File: `internal/scheduler/reaper.go`
- Assert event trigger status = `timed_out`
- Assert step run status = `failed` with error containing "event trigger timed out"
- Assert workflow run status = `failed`

**Test 3: `TestE2E_SDKWaitForEvent_RoundTrip`**
- Create a job, trigger it, get run token
- Transition run to `executing` (simulate SDK pick-up)
- Call `POST /sdk/v1/runs/{runID}/wait-for-event` with `{"event_key": "sdk-test:123", "timeout_secs": 300}`
- Assert run status = `waiting`
- Assert event trigger exists with `source_type: job_run`
- Send event via `POST /v1/events/sdk-test:123/send` with payload `{"done": true}`
- Assert run status = `queued` with `checkpoint_data` containing `{"done": true}`

**Test 4: `TestE2E_ApprovalStepWithParallelEventTrigger`**
- Create a workflow with an approval step
- Trigger workflow, assert step is `waiting`, approval is `pending`
- Assert parallel event trigger exists for `approval:{wfRunID}:{stepRef}`
- Approve via API, assert step = `completed`
- Assert event trigger status = `received`
- Assert workflow run status = `completed`

**Test 5: `TestE2E_WaitForEventStep_ChainedDependencies`**
- Create a workflow: `wait_step (wait_for_event)` → `job_step (depends_on: wait_step)`
- Trigger workflow, assert `wait_step` = `waiting`, `job_step` = `pending`
- Send event to `wait_step`'s key
- Assert `wait_step` = `completed`, `job_step` = `running` or `pending` (fan-in triggered)

**Test 6: `TestE2E_SendEvent_AlreadyReceived_Returns409`**
- Create a trigger, send event once (200), send again (409)

**Setup changes:**
- In `internal/e2e/e2e_test.go` `TestMain`: add `FFEventTriggers: true` to the config
- In `wfSetup()`: already creates `WorkflowEngine` and `StepCallback` — ensure both are passed to `api.NewServer`
- Add `newReaper()` test helper in the e2e file that creates a `Reaper` with `testStore`

**Migration note:** The e2e tests use `testutil.SetupTestEnv` which runs all migrations, so migration 000049 is already applied.

**Acceptance criteria:**
- All 6 tests pass with `go test -tags integration ./internal/e2e/ -run TestE2E_WaitForEvent -v -count=1`
- Tests are independent (each calls `mustCleanWf(t)`)

---

### Item 2: Transaction Boundaries

**Goal:** Ensure atomicity when receiving an event: the trigger status update, step completion, and fan-in must either all succeed or all fail. Add a reconciliation reaper pass for any inconsistent states.

**Approach A (preferred): Transaction wrapper in `handleSendEvent`**

The `store.WithTx` pattern already exists. The challenge is that `resumeEventSource` calls `OnEventReceived` which uses a `CallbackStore` — we need to pass the transactional `*Queries` through.

**Implementation:**

1. **Add `TxBeginner` to `APIStore` interface** (`internal/api/server.go`):
   ```go
   type APIStore interface {
       // ... existing methods
       BeginTx(ctx context.Context) (pgx.Tx, error)
   }
   ```
   Actually, use the existing pattern — the `*store.Queries` already wraps `pgx.Tx`. The cleaner approach:

2. **Add `WithTx` method to `Server`** (`internal/api/event_triggers.go`):
   ```go
   func (s *Server) handleSendEvent(w http.ResponseWriter, r *http.Request) {
       // ... validate, get trigger ...
       
       err := store.WithTx(r.Context(), s.pool, func(txQ *store.Queries) error {
           // 1. Update trigger status
           if err := txQ.UpdateEventTriggerStatus(ctx, trigger.ID, ...); err != nil {
               return err
           }
           // 2. Resume event source (step completion + fan-in)
           return s.resumeEventSourceTx(ctx, txQ, trigger)
       })
   }
   ```

   **Problem:** `OnEventReceived` uses `s.workflowCallback` which has its own `store` reference. We'd need to thread the tx-scoped store through.

3. **Revised approach — Reconciliation reaper (simpler, more robust):**

   Add a new reaper pass: `reapInconsistentEventTriggers`

   **File:** `internal/scheduler/reaper.go`
   ```go
   func (r *Reaper) reapInconsistentEventTriggers(ctx context.Context) {
       // Find event triggers that are "received" but whose step run is still "waiting"
       // This means the process crashed between trigger update and step completion
       triggers, err := r.store.ListReceivedEventTriggersWithWaitingSteps(ctx)
       // For each, call the appropriate resume logic
   }
   ```

   **File:** `internal/store/event_triggers.go` — new query:
   ```sql
   SELECT et.*
   FROM event_triggers et
   JOIN workflow_step_runs wsr ON wsr.id = et.workflow_step_run_id
   WHERE et.status = 'received'
     AND et.source_type = 'workflow_step'
     AND wsr.status = 'waiting'
     AND et.received_at < NOW() - INTERVAL '30 seconds'
   
   UNION ALL
   
   SELECT et.*
   FROM event_triggers et
   JOIN runs r ON r.id = et.job_run_id
   WHERE et.status = 'received'
     AND et.source_type = 'job_run'
     AND r.status = 'waiting'
     AND et.received_at < NOW() - INTERVAL '30 seconds'
   ```

   The 30-second grace period prevents the reaper from interfering with in-flight operations.

4. **New `ReaperStore` interface method:**
   ```go
   ListReceivedEventTriggersWithStaleSteps(ctx context.Context) ([]domain.EventTrigger, error)
   ```

5. **Reaper reconciliation logic:**
   - For `workflow_step`: call `UpdateStepRunStatus(StepCompleted)` + fan-in (need `WorkflowCallback` interface extension with `OnEventReceived`)
   - For `job_run`: call `UpdateRunStatus(waiting→queued)` with checkpoint data

6. **Extend reaper `WorkflowCallback` interface:**
   ```go
   type WorkflowCallback interface {
       OnJobRunTerminal(ctx context.Context, run *domain.JobRun) error
       OnEventReceived(ctx context.Context, trigger *domain.EventTrigger) error  // NEW
   }
   ```

7. **Register in reaper `Run()` loop** — add `r.reapInconsistentEventTriggers(loopCtx)` after `reapExpiredEventTriggers`.

**Files modified:**
- `internal/scheduler/reaper.go` — new method, interface extension
- `internal/store/event_triggers.go` — new query
- `internal/store/store.go` — interface addition
- `internal/scheduler/mock_test.go` — mock update
- `internal/scheduler/reaper_test.go` — 2 new tests (workflow_step reconciliation, job_run reconciliation)

**Index needed:**
```sql
-- Migration 000050
CREATE INDEX idx_event_triggers_reconcile 
    ON event_triggers(status, source_type, received_at)
    WHERE status = 'received';
```

**Acceptance criteria:**
- Unit test: mock trigger is `received` with step still `waiting` → reaper completes the step
- Unit test: mock trigger is `received` with run still `waiting` → reaper re-queues the run
- `go build`, `go vet`, `golangci-lint`, tests pass

---

### Item 3: Event Key Collision Handling

**Goal:** Better error messages on UNIQUE constraint violation, document namespacing conventions.

**Implementation:**

1. **Detect and wrap Postgres unique violation** (`internal/store/event_triggers.go`):
   ```go
   import "github.com/jackc/pgx/v5/pgconn"
   
   func (q *Queries) CreateEventTrigger(ctx context.Context, trigger *domain.EventTrigger) error {
       // ... existing insert ...
       if err != nil {
           var pgErr *pgconn.PgError
           if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
               return fmt.Errorf("event key %q already exists: %w", trigger.EventKey, ErrEventKeyConflict)
           }
           return fmt.Errorf("create event trigger: %w", err)
       }
   }
   ```

2. **Define sentinel error** (`internal/store/event_triggers.go`):
   ```go
   var ErrEventKeyConflict = errors.New("event key conflict")
   ```

3. **Handle in API layer** (`internal/api/event_triggers.go`):
   In `handleSendEvent` — already handles this correctly (returns 409).

4. **Handle in engine** (`internal/workflow/engine_steps.go`):
   ```go
   if err := e.store.CreateEventTrigger(ctx, trigger); err != nil {
       if errors.Is(err, store.ErrEventKeyConflict) {
           return fmt.Errorf("event key %q is already in use by another trigger — "+
               "use a unique key pattern like {workflow_id}:{run_id}:{step_ref}: %w", 
               renderedKey, err)
       }
       return fmt.Errorf("create event trigger: %w", err)
   }
   ```

5. **Handle in SDK endpoint** (`internal/api/sdk_wait_event.go`):
   ```go
   if errors.Is(err, store.ErrEventKeyConflict) {
       respondError(w, r, http.StatusConflict, "event key already in use")
       return
   }
   ```

6. **Auto-namespace for workflow steps** — Consider prefixing engine-created event keys:
   The engine currently uses the raw rendered key. We could auto-prefix:
   ```go
   renderedKey = fmt.Sprintf("wf:%s:%s:%s", wfRun.ID, step.StepRef, renderedKey)
   ```
   **Decision:** Do NOT auto-prefix — it removes user control. Instead, document the convention in the OpenAPI spec and add a validation hint in the error message.

7. **Add documentation** to the OpenAPI spec (item 5) describing key conventions:
   ```
   Event keys must be globally unique. Recommended patterns:
   - Workflow steps: `{domain}:{entity_id}` e.g., `aml-check:app-456`
   - SDK runs: `{service}:{correlation_id}` e.g., `payment:order-789`
   - Include workflow/run IDs for per-execution uniqueness: `aml:{{workflow_run_id}}:{{step_ref}}`
   ```

**Files modified:**
- `internal/store/event_triggers.go` — sentinel error, wrap unique violation
- `internal/workflow/engine_steps.go` — better error message
- `internal/api/sdk_wait_event.go` — 409 on conflict
- Tests: `internal/store/event_triggers_test.go`, `internal/workflow/engine_test.go`, `internal/api/sdk_wait_event_test.go`

**Acceptance criteria:**
- Store returns `ErrEventKeyConflict` on duplicate key
- Engine error message includes namespacing hint
- SDK endpoint returns 409 with clear message
- 3 new tests (one per layer)

---

### Item 4: Event Trigger Cancellation on Workflow Cancel

**Goal:** When a workflow run is canceled (via API or reaper timeout), also cancel its pending event triggers.

**Implementation:**

1. **New store method** (`internal/store/event_triggers.go`):
   ```go
   func (q *Queries) CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error) {
       query := `
           UPDATE event_triggers
           SET status = 'canceled', error = 'workflow canceled'
           WHERE workflow_run_id = $1
             AND status = 'waiting'
       `
       tag, err := q.db.Exec(ctx, query, workflowRunID)
       if err != nil {
           return 0, fmt.Errorf("cancel event triggers for workflow run: %w", err)
       }
       return tag.RowsAffected(), nil
   }
   ```

2. **New store method for job run cancellation** (`internal/store/event_triggers.go`):
   ```go
   func (q *Queries) CancelEventTriggerByJobRun(ctx context.Context, jobRunID string) error {
       query := `
           UPDATE event_triggers
           SET status = 'canceled', error = 'job run canceled'
           WHERE job_run_id = $1
             AND status = 'waiting'
       `
       _, err := q.db.Exec(ctx, query, jobRunID)
       return err
   }
   ```

3. **Add to `APIStore` interface** (`internal/api/server.go`):
   ```go
   CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
   ```

4. **Integrate into `handleCancelWorkflowRun`** (`internal/api/workflow_runs.go`):
   After canceling step runs and job runs, add:
   ```go
   // Cancel any pending event triggers for this workflow.
   if _, err := s.store.CancelEventTriggersByWorkflowRun(r.Context(), run.ID); err != nil {
       slog.Warn("failed to cancel event triggers for workflow (non-fatal)", "workflow_run_id", run.ID, "error", err)
   }
   ```

5. **Integrate into reaper `reapTimedOutWorkflows`** (`internal/scheduler/reaper.go`):
   After the step run / job run cancel loop, add:
   ```go
   if _, cancelErr := r.store.CancelEventTriggersByWorkflowRun(ctx, wfRun.ID); cancelErr != nil {
       slog.Error("failed to cancel event triggers for timed out workflow", "workflow_run_id", wfRun.ID, "error", cancelErr)
   }
   ```

6. **Add to `ReaperStore` interface**:
   ```go
   CancelEventTriggersByWorkflowRun(ctx context.Context, workflowRunID string) (int64, error)
   ```

7. **Index** (migration 000050, or append to 000049 if not yet deployed):
   ```sql
   CREATE INDEX idx_event_triggers_workflow_run 
       ON event_triggers(workflow_run_id, status)
       WHERE status = 'waiting';
   ```

**Files modified:**
- `internal/store/event_triggers.go` — 2 new methods
- `internal/store/store.go` — interface additions
- `internal/api/workflow_runs.go` — cancel triggers on workflow cancel
- `internal/api/server.go` — APIStore interface update
- `internal/scheduler/reaper.go` — cancel triggers on workflow timeout + ReaperStore update
- `internal/scheduler/mock_test.go` — mock update
- `internal/api/mock_test.go` — mock update
- Tests: 3 new tests (cancel API, reaper timeout, store method)

**Acceptance criteria:**
- Canceling a workflow via API also cancels its `waiting` event triggers
- Reaper timeout also cancels event triggers
- Event triggers in `received`/`timed_out` states are NOT affected
- E2E test (item 1, if done first) validates the full flow

---

### Item 5: OpenAPI Spec Update

**Goal:** Document the 4 new endpoints in both `internal/api/openapi.yaml` and `docs/openapi.yaml`.

**New tag:**
```yaml
- name: EventTriggers
  description: External event trigger management for durable waits
```

**Endpoints to document:**

1. **`POST /v1/events/{eventKey}/send`**
   - Summary: Send an event to a waiting trigger
   - Parameters: `eventKey` (path, required)
   - Request body: `{ "payload": {} }` (optional)
   - Responses: 200 (trigger object), 404 (not found), 409 (already received/timed_out)

2. **`GET /v1/events/{eventKey}`**
   - Summary: Get a single event trigger by key
   - Parameters: `eventKey` (path, required)
   - Response: 200 (trigger object), 404 (not found)

3. **`GET /v1/events`**
   - Summary: List event triggers for a project
   - Parameters: `project_id` (query, required), `status` (query, optional), `limit`, `cursor`
   - Response: 200 (paginated list)

4. **`POST /sdk/v1/runs/{runID}/wait-for-event`** (SDK section)
   - Summary: Pause a running job until an external event arrives
   - Auth: Bearer run token
   - Request body: `{ "event_key": "string", "timeout_secs": 3600 }`
   - Response: 200 (event trigger object), 400 (validation), 409 (key conflict)
   - Note: Requires `FF_EVENT_TRIGGERS` feature flag

**Schema to add:**
```yaml
EventTrigger:
  type: object
  properties:
    id: { type: string }
    event_key: { type: string }
    project_id: { type: string }
    source_type: { type: string, enum: [workflow_step, job_run] }
    workflow_run_id: { type: string, nullable: true }
    workflow_step_run_id: { type: string, nullable: true }
    job_run_id: { type: string, nullable: true }
    status: { type: string, enum: [waiting, received, timed_out, canceled] }
    request_payload: { type: object, nullable: true }
    response_payload: { type: object, nullable: true }
    timeout_secs: { type: integer }
    requested_at: { type: string, format: date-time }
    received_at: { type: string, format: date-time, nullable: true }
    expires_at: { type: string, format: date-time }
    error: { type: string, nullable: true }

SendEventRequest:
  type: object
  properties:
    payload: { type: object, nullable: true }

SDKWaitForEventRequest:
  type: object
  required: [event_key]
  properties:
    event_key: { type: string }
    timeout_secs: { type: integer, default: 3600 }
    payload: { type: object, nullable: true }
```

**Event key conventions section** (add to description):
```
## Event Key Conventions

Event keys are globally unique identifiers for event triggers. Recommended patterns:
- Workflow steps: `{domain}:{entity_id}` — e.g., `aml-check:app-456`
- SDK runs: `{service}:{correlation_id}` — e.g., `payment:order-789`  
- Per-execution uniqueness: use `{{workflow_run_id}}` in the template

Keys support template variables from the workflow payload:
`aml-check:{{application_id}}` with payload `{"application_id": "app-456"}` 
renders to `aml-check:app-456`.
```

**Files modified:**
- `internal/api/openapi.yaml` — endpoints + schemas
- `docs/openapi.yaml` — mirror the same changes

**Acceptance criteria:**
- Both YAML files are valid OpenAPI 3.0.3
- All 4 endpoints documented with request/response schemas
- Event key conventions section added

---

## Phase B: Medium-Term Fast Follows (Items 6–10)

### Item 6: Event Trigger Cleanup/Retention

**Goal:** Delete old terminal event triggers to prevent unbounded table growth.

**Implementation:**

1. **New store method** (`internal/store/event_triggers.go`):
   ```go
   func (q *Queries) DeleteEventTriggersFinishedBefore(ctx context.Context, before time.Time, limit int) (int64, error) {
       query := `
           DELETE FROM event_triggers
           WHERE id IN (
               SELECT id FROM event_triggers
               WHERE status IN ('received', 'timed_out', 'canceled')
                 AND COALESCE(received_at, expires_at) < $1
               LIMIT $2
           )
       `
       tag, err := q.db.Exec(ctx, query, before, limit)
       if err != nil {
           return 0, fmt.Errorf("delete old event triggers: %w", err)
       }
       return tag.RowsAffected(), nil
   }
   ```

2. **Add to `ReaperStore` interface** and `SchedulerStore`.

3. **New reaper method** (`internal/scheduler/reaper.go`):
   ```go
   func (r *Reaper) reapOldEventTriggers(ctx context.Context) {
       ctx, span := otel.Tracer("strait").Start(ctx, "reaper.ReapOldEventTriggers")
       defer span.End()
   
       before := time.Now().Add(-r.eventTriggerRetention)
       count, err := r.store.DeleteEventTriggersFinishedBefore(ctx, before, r.deleteBatchLimit)
       if err != nil {
           slog.Error("failed to delete old event triggers", "error", err)
           return
       }
       if count > 0 {
           slog.Info("deleted old event triggers", "count", count, "before", before.UTC())
       }
   }
   ```

4. **Add `eventTriggerRetention` field** to `Reaper` struct. Default: 30 days. Configurable via `WithEventTriggerRetention(d time.Duration)`.

5. **Register in `Run()` loop** — `r.reapOldEventTriggers(loopCtx)`.

6. **Config** (`internal/config/config.go`):
   ```go
   EventTriggerRetentionDays int `envconfig:"EVENT_TRIGGER_RETENTION_DAYS" default:"30"`
   ```

**Files modified:**
- `internal/store/event_triggers.go` — new method
- `internal/store/store.go` — interface
- `internal/scheduler/reaper.go` — new method, config
- `internal/config/config.go` — new config
- `cmd/strait/services.go` — pass config to reaper
- Tests: 2 (store method, reaper integration)

**Acceptance criteria:**
- Old terminal triggers are deleted in batches
- `waiting` triggers are never deleted
- Configurable retention period

---

### Item 7: Idempotent Event Delivery

**Goal:** Allow re-sending an event with the same payload to return 200 (idempotent) instead of 409.

**Implementation:**

1. **Modify `handleSendEvent`** (`internal/api/event_triggers.go`):
   ```go
   if trigger.Status != domain.EventTriggerStatusWaiting {
       // If already received, check if payload matches for idempotency.
       if trigger.Status == domain.EventTriggerStatusReceived {
           if payloadsMatch(trigger.ResponsePayload, req.Payload) {
               // Idempotent — return the existing trigger as-is.
               respondJSON(w, r, http.StatusOK, trigger)
               return
           }
       }
       respondError(w, r, http.StatusConflict, "event trigger already "+trigger.Status)
       return
   }
   ```

2. **`payloadsMatch` helper:**
   ```go
   func payloadsMatch(existing, incoming json.RawMessage) bool {
       if len(existing) == 0 && len(incoming) == 0 {
           return true
       }
       // Normalize: unmarshal and re-marshal for canonical comparison
       var a, b any
       if err := json.Unmarshal(existing, &a); err != nil {
           return false
       }
       if err := json.Unmarshal(incoming, &b); err != nil {
           return false
       }
       ea, _ := json.Marshal(a)
       eb, _ := json.Marshal(b)
       return string(ea) == string(eb)
   }
   ```

3. **If payload differs, return 409** with message `"event already received with different payload"`.

**Files modified:**
- `internal/api/event_triggers.go` — idempotency check + helper
- Tests: 2 (same payload = 200, different payload = 409)

**Acceptance criteria:**
- Same key + same payload = 200 (no side effects)
- Same key + different payload = 409 with clear error
- Same key + not yet received = normal 200 flow

---

### Item 8: Webhook-Based Event Delivery (Notification Webhook)

**Goal:** When an event trigger is created, optionally POST a notification to a configured webhook URL telling the external system "I'm waiting for event X".

**Implementation:**

1. **New domain fields** (`internal/domain/types.go`):
   ```go
   type EventTrigger struct {
       // ... existing fields ...
       NotifyURL    string `json:"notify_url,omitempty"`     // webhook URL to call on creation
       NotifyStatus string `json:"notify_status,omitempty"`  // pending, sent, failed
   }
   ```

2. **Migration 000050 (or 000051):**
   ```sql
   ALTER TABLE event_triggers ADD COLUMN notify_url TEXT;
   ALTER TABLE event_triggers ADD COLUMN notify_status TEXT NOT NULL DEFAULT '';
   ```

3. **Workflow step config** — add `event_notify_url` to `WorkflowStep`:
   ```go
   EventNotifyURL string `json:"event_notify_url,omitempty"`
   ```
   With template rendering support (e.g., `https://partner.com/callback?ref={{app_id}}`).

4. **SDK request** — add optional `notify_url` field to `SDKWaitForEventRequest`.

5. **Notification sender** — New file `internal/webhook/event_notify.go`:
   ```go
   type EventNotifier struct {
       client  *http.Client
       store   NotifyStore
       logger  *slog.Logger
   }
   
   func (n *EventNotifier) Notify(ctx context.Context, trigger *domain.EventTrigger) error {
       payload := map[string]any{
           "event_key":   trigger.EventKey,
           "trigger_id":  trigger.ID,
           "expires_at":  trigger.ExpiresAt,
           "callback_url": fmt.Sprintf("/v1/events/%s/send", trigger.EventKey),
       }
       // POST to trigger.NotifyURL with JSON payload
       // Update notify_status to "sent" or "failed"
       // Retry with backoff (max 3 attempts)
   }
   ```

6. **Integration points:**
   - `startWaitForEventStep` → after creating trigger, fire-and-forget notify (goroutine with timeout)
   - `handleSDKWaitForEvent` → same pattern
   - Alternatively: use the existing webhook delivery queue pattern (`webhook_deliveries` table) for reliable delivery with retries

7. **CDC/pubsub:** The notification status changes are published via the existing CDC handler.

**Files modified:**
- `internal/domain/types.go` — new fields
- `internal/store/event_triggers.go` — scan new columns, update query
- `internal/workflow/engine_steps.go` — pass notify_url
- `internal/api/sdk_wait_event.go` — accept notify_url
- `internal/webhook/event_notify.go` — new file
- Migration 000050/000051

**Acceptance criteria:**
- Creating a trigger with `notify_url` POSTs to that URL
- Notification includes event key, trigger ID, expiry, and callback URL
- Failed notifications are logged but don't block trigger creation
- `notify_status` tracks delivery state

---

### Item 9: CLI Commands

**Goal:** Add `strait triggers list`, `strait triggers get`, `strait triggers send` commands.

**Implementation:**

1. **New CLI file** (`cmd/strait/triggers.go`):
   ```go
   func newTriggersCommand(state *appState) *cobra.Command {
       cmd := &cobra.Command{
           Use:   "triggers",
           Short: "Manage event triggers",
       }
       cmd.AddCommand(newTriggersListCommand(state))
       cmd.AddCommand(newTriggersGetCommand(state))
       cmd.AddCommand(newTriggersSendCommand(state))
       return cmd
   }
   ```

2. **`strait triggers list`:**
   ```go
   func newTriggersListCommand(state *appState) *cobra.Command {
       var projectID, status string
       cmd := &cobra.Command{
           Use:   "list",
           Short: "List event triggers",
           RunE: func(cmd *cobra.Command, _ []string) error {
               cli, err := newAPIClient(state)
               // GET /v1/events?project_id=X&status=Y
               // Format as table: ID | KEY | STATUS | SOURCE | REQUESTED_AT | EXPIRES_AT
           },
       }
       cmd.Flags().StringVar(&projectID, "project", "", "project ID (required)")
       cmd.Flags().StringVar(&status, "status", "", "filter by status")
       _ = cmd.MarkFlagRequired("project")
       return cmd
   }
   ```

3. **`strait triggers get`:**
   ```go
   func newTriggersGetCommand(state *appState) *cobra.Command {
       cmd := &cobra.Command{
           Use:   "get <event-key>",
           Short: "Get event trigger by key",
           Args:  cobra.ExactArgs(1),
           RunE: func(cmd *cobra.Command, args []string) error {
               cli, err := newAPIClient(state)
               // GET /v1/events/{eventKey}
               // Print full trigger details
           },
       }
       return cmd
   }
   ```

4. **`strait triggers send`:**
   ```go
   func newTriggersSendCommand(state *appState) *cobra.Command {
       var payload string
       cmd := &cobra.Command{
           Use:   "send <event-key>",
           Short: "Send event to a waiting trigger",
           Args:  cobra.ExactArgs(1),
           RunE: func(cmd *cobra.Command, args []string) error {
               cli, err := newAPIClient(state)
               // POST /v1/events/{eventKey}/send with payload
           },
       }
       cmd.Flags().StringVar(&payload, "payload", "", "JSON payload")
       return cmd
   }
   ```

5. **Client methods** (`internal/cli/client/api.go`):
   ```go
   func (c *Client) ListEventTriggers(ctx context.Context, projectID, status string) ([]domain.EventTrigger, error)
   func (c *Client) GetEventTrigger(ctx context.Context, eventKey string) (*domain.EventTrigger, error)
   func (c *Client) SendEvent(ctx context.Context, eventKey string, payload json.RawMessage) (*domain.EventTrigger, error)
   ```

6. **Register in root** (`cmd/strait/root.go`):
   ```go
   cmd.AddCommand(newTriggersCommand(state))
   ```

**Files modified:**
- `cmd/strait/triggers.go` — new file
- `cmd/strait/root.go` — register command
- `internal/cli/client/api.go` — 3 new methods
- `internal/cli/client/types.go` — request types if needed
- Tests: `cmd/strait/triggers_test.go` — cobra command tests

**Acceptance criteria:**
- `strait triggers list --project proj-1` shows table output
- `strait triggers get aml-check:app-456` shows trigger details
- `strait triggers send aml-check:app-456 --payload '{"ok":true}'` sends event
- All commands respect `--format json` flag

---

### Item 10: Metrics and Observability

**Goal:** Add OTel/Prometheus metrics for event trigger operations.

**Implementation:**

1. **New metrics** (`internal/telemetry/metrics.go`):
   ```go
   type Metrics struct {
       // ... existing ...
       
       // Event trigger metrics
       EventTriggersCreated    metric.Int64Counter
       EventTriggersReceived   metric.Int64Counter
       EventTriggersTimedOut   metric.Int64Counter
       EventTriggerWaitDuration metric.Float64Histogram
   }
   ```

2. **Initialize in `InitMetrics`:**
   ```go
   eventTriggersCreated, err := meter.Int64Counter(
       "strait.event_triggers.created",
       metric.WithDescription("Total event triggers created"),
       metric.WithUnit("1"),
   )
   
   eventTriggersReceived, err := meter.Int64Counter(
       "strait.event_triggers.received",
       metric.WithDescription("Total events received (triggers completed)"),
       metric.WithUnit("1"),
   )
   
   eventTriggersTimedOut, err := meter.Int64Counter(
       "strait.event_triggers.timed_out",
       metric.WithDescription("Total event triggers that expired"),
       metric.WithUnit("1"),
   )
   
   eventTriggerWaitDuration, err := meter.Float64Histogram(
       "strait.event_triggers.wait_duration",
       metric.WithDescription("Duration between trigger creation and event receipt"),
       metric.WithUnit("s"),
   )
   ```

3. **Instrument event trigger creation** — engine + SDK handler:
   ```go
   if s.metrics != nil {
       s.metrics.EventTriggersCreated.Add(ctx, 1, 
           metric.WithAttributes(attribute.String("source_type", trigger.SourceType)))
   }
   ```

4. **Instrument event receipt** (`handleSendEvent`):
   ```go
   if s.metrics != nil {
       s.metrics.EventTriggersReceived.Add(ctx, 1,
           metric.WithAttributes(attribute.String("source_type", trigger.SourceType)))
       if trigger.ReceivedAt != nil {
           waitSecs := trigger.ReceivedAt.Sub(trigger.RequestedAt).Seconds()
           s.metrics.EventTriggerWaitDuration.Record(ctx, waitSecs,
               metric.WithAttributes(attribute.String("source_type", trigger.SourceType)))
       }
   }
   ```

5. **Instrument reaper timeouts:**
   ```go
   if r.metrics != nil {
       r.metrics.EventTriggersTimedOut.Add(ctx, 1,
           metric.WithAttributes(attribute.String("source_type", trigger.SourceType)))
   }
   ```

6. **Pass `Metrics` to reaper and API server** — add to `Reaper` struct and `ServerDeps`.

**Files modified:**
- `internal/telemetry/metrics.go` — 4 new metrics
- `internal/api/event_triggers.go` — instrument creation/receipt
- `internal/api/sdk_wait_event.go` — instrument creation
- `internal/scheduler/reaper.go` — instrument timeouts
- `internal/workflow/engine_steps.go` — instrument creation
- `internal/api/server.go` — accept metrics in deps
- `internal/scheduler/reaper.go` — accept metrics
- `cmd/strait/services.go` — pass metrics through

**Acceptance criteria:**
- `/metrics` endpoint exposes `strait_event_triggers_created_total`, `strait_event_triggers_received_total`, `strait_event_triggers_timed_out_total`, `strait_event_triggers_wait_duration_seconds`
- All metrics have `source_type` label
- Wait duration histogram captures the full wait time

---

## Phase C: Longer-Term (Items 11–13)

### Item 11: Event Patterns / Wildcards

**Goal:** Support wildcard event keys so one `send` call can resolve multiple waiting triggers.

**Implementation:**

1. **New concept: event key matching**
   - Exact match (current): `aml-check:app-456` matches exactly
   - Wildcard pattern: `aml-check:*` matches all triggers whose key starts with `aml-check:`
   - Pattern stored on the SENDER side, not the trigger side

2. **New API parameter** on `POST /v1/events/{eventKey}/send`:
   ```
   POST /v1/events/aml-check:*/send?match=prefix
   ```
   Or new endpoint: `POST /v1/events/send-pattern`

3. **Store method:**
   ```go
   func (q *Queries) ListEventTriggersByKeyPrefix(ctx context.Context, prefix string) ([]domain.EventTrigger, error) {
       query := `
           SELECT ... FROM event_triggers
           WHERE event_key LIKE $1 || '%'
             AND status = 'waiting'
           LIMIT 100
       `
   }
   ```

4. **Batch processing:** Iterate through matches, apply `UpdateEventTriggerStatus` + `resumeEventSource` for each.

5. **Index:** `CREATE INDEX idx_event_triggers_key_prefix ON event_triggers(event_key text_pattern_ops) WHERE status = 'waiting';`

6. **Uniqueness consideration:** The UNIQUE constraint on `event_key` means each trigger still has a unique key. The wildcard is on the SEND side only — one send matches many waiting triggers.

**Risk:** Performance concern with large `LIKE` queries. Mitigate with the partial index and LIMIT.

**Files modified:**
- `internal/store/event_triggers.go` — new query
- `internal/api/event_triggers.go` — new handler or modified handler
- `internal/api/routes.go` — new route if separate endpoint
- Migration — new index

**Acceptance criteria:**
- `POST /v1/events/aml-check:*/send?match=prefix` resolves all waiting `aml-check:*` triggers
- Response includes count of resolved triggers
- Partial index ensures query performance

---

### Item 12: Event Trigger Chaining

**Goal:** A step's completion can automatically send an event to another waiting trigger, enabling workflow-to-workflow communication.

**Implementation:**

1. **New workflow step field:** `event_emit_key` — when the step completes, emit its output as an event to this key.
   ```go
   type WorkflowStep struct {
       // ... existing ...
       EventEmitKey string `json:"event_emit_key,omitempty"` // key to emit to on completion
   }
   ```

2. **Migration:**
   ```sql
   ALTER TABLE workflow_steps ADD COLUMN event_emit_key TEXT;
   ALTER TABLE workflow_version_steps ADD COLUMN event_emit_key TEXT;
   ```

3. **Emit on step completion** — In `StepCallback.fanInAndStartReadyChildren` or `checkWorkflowCompletion`, after a step completes:
   ```go
   if step.EventEmitKey != "" {
       renderedKey := renderStringTemplate(step.EventEmitKey, wfRun.Payload)
       trigger, err := s.store.GetEventTriggerByEventKey(ctx, renderedKey)
       if err == nil && trigger != nil && trigger.Status == domain.EventTriggerStatusWaiting {
           // Auto-send event with step output as payload
           now := time.Now()
           s.store.UpdateEventTriggerStatus(ctx, trigger.ID, domain.EventTriggerStatusReceived, stepRun.Output, &now, "")
           s.resumeEventSource(ctx, trigger)
       }
   }
   ```

4. **Use case:** Workflow A has a `wait_for_event` step with key `approval:app-123`. Workflow B has a step with `event_emit_key: "approval:app-123"`. When B's step completes, it automatically unblocks A.

**Files modified:**
- `internal/domain/types.go` — new field
- `internal/store/workflow_steps.go` — scan new column
- `internal/workflow/callback.go` — emit logic in step completion path
- Migration

**Acceptance criteria:**
- Step completion with `event_emit_key` auto-sends event to waiting trigger
- Chaining works across different workflows
- Circular chains are safe (trigger is `received` before processing)

---

### Item 13: Durable Sleep as Event Trigger

**Goal:** Implement `sleep` steps as a special case of event triggers — no external event needed, the reaper "wakes" them when the duration elapses.

**Implementation:**

1. **New step type:** `WorkflowStepTypeSleep = "sleep"`

2. **New workflow step field:** `SleepDurationSecs int`

3. **Migration:**
   ```sql
   ALTER TABLE workflow_steps ADD COLUMN sleep_duration_secs INT;
   ALTER TABLE workflow_version_steps ADD COLUMN sleep_duration_secs INT;
   ```

4. **Engine `startStep` case for `sleep`:**
   ```go
   case domain.WorkflowStepTypeSleep:
       return e.startSleepStep(ctx, stepRun, step, wfRun, now)
   ```

5. **`startSleepStep`:**
   ```go
   func (e *WorkflowEngine) startSleepStep(ctx context.Context, stepRun *domain.WorkflowStepRun, step *domain.WorkflowStep, wfRun *domain.WorkflowRun, now time.Time) error {
       if err := e.store.UpdateStepRunStatus(ctx, stepRun.ID, domain.StepWaiting, map[string]any{"started_at": now}); err != nil {
           return err
       }
       
       trigger := &domain.EventTrigger{
           ID:                newID(),
           EventKey:          fmt.Sprintf("sleep:%s:%s", wfRun.ID, step.StepRef),
           ProjectID:         wfRun.ProjectID,
           SourceType:        domain.EventSourceWorkflowStep,
           WorkflowRunID:     wfRun.ID,
           WorkflowStepRunID: stepRun.ID,
           Status:            domain.EventTriggerStatusWaiting,
           TimeoutSecs:       step.SleepDurationSecs,
           RequestedAt:       now,
           ExpiresAt:         now.Add(time.Duration(step.SleepDurationSecs) * time.Second),
       }
       return e.store.CreateEventTrigger(ctx, trigger)
   }
   ```

6. **Reaper modification** — In `reapExpiredEventTriggers`, treat sleep triggers differently:
   ```go
   // Check if this is a sleep trigger (key starts with "sleep:")
   if strings.HasPrefix(trigger.EventKey, "sleep:") {
       // Complete the step instead of failing it
       e.store.UpdateStepRunStatus(ctx, trigger.WorkflowStepRunID, domain.StepCompleted, ...)
       // Trigger fan-in
   }
   ```

   Or better: add a `trigger_type` field (`event`, `sleep`) and handle them separately.

7. **Alternative approach:** Add a boolean `IsSleep` to `EventTrigger` or a `TriggerType` field:
   ```go
   TriggerType string // "event" or "sleep"
   ```
   Migration: `ALTER TABLE event_triggers ADD COLUMN trigger_type TEXT NOT NULL DEFAULT 'event';`

8. **Reaper distinction:**
   - `trigger_type=event` + expired → **fail** (timed out)
   - `trigger_type=sleep` + expired → **complete** (sleep finished)

**Files modified:**
- `internal/domain/types.go` — new step type, trigger type, sleep fields
- `internal/store/event_triggers.go` — scan new column
- `internal/store/workflow_steps.go` — scan new column
- `internal/workflow/engine_steps.go` — new case
- `internal/scheduler/reaper.go` — handle sleep completion
- Migration

**Acceptance criteria:**
- `sleep` step creates event trigger with `trigger_type=sleep`
- When `expires_at` passes, reaper completes the step (not fails it)
- Fan-in works correctly after sleep completion
- Sleep steps don't require any external event

---

## Implementation Order

```
Phase A (Short-Term — do these first, in order):
  Item 4: Event trigger cancellation     (small, prevents orphans)
  Item 3: Event key collision handling    (small, better errors)
  Item 2: Transaction / reconciliation    (medium, prevents data loss)
  Item 1: Integration / E2E tests         (large, validates all of the above)
  Item 5: OpenAPI spec update             (medium, documentation)

Phase B (Medium-Term — after Phase A ships):
  Item 6: Cleanup/retention               (small, operational)
  Item 10: Metrics                        (medium, observability)
  Item 7: Idempotent delivery             (small, API improvement)
  Item 9: CLI commands                    (medium, developer experience)
  Item 8: Notification webhooks           (large, new subsystem)

Phase C (Longer-Term — after Phase B, as needed):
  Item 13: Durable sleep                  (medium, leverages existing system)
  Item 11: Wildcard patterns              (medium, new query patterns)
  Item 12: Event chaining                 (medium, cross-workflow)
```

## Migration Plan

All new migrations should be in separate files to avoid conflicts:

| Migration | Items | Description |
|-----------|-------|-------------|
| `000050_event_trigger_improvements.up.sql` | 2, 4 | Reconciliation index, workflow_run_id index |
| `000051_event_trigger_notify.up.sql` | 8 | `notify_url`, `notify_status` columns |
| `000052_event_trigger_types.up.sql` | 12, 13 | `event_emit_key`, `sleep_duration_secs`, `trigger_type` |
| `000053_event_trigger_prefix_index.up.sql` | 11 | `text_pattern_ops` index for prefix matching |
