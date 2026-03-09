# Event Triggers — Final Improvements Plan

15 items from the final review, organized into 8 commits.
Each commit is self-contained, passes all checks, and builds on the previous.

---

## Commit 1: Rate limiting on event trigger endpoints

**Problem:** `POST /v1/events/{eventKey}/send` and `POST /v1/events/prefix/{prefix}/send`
have no rate limiting, unlike all other write endpoints.

**Files:**
- `internal/api/routes.go` — add `rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)` to
  both send routes, matching the existing trigger rate limit config pattern.

**Changes:**
```go
r.Route("/events", func(r chi.Router) {
    r.Get("/", s.handleListEventTriggers)
    r.Route("/prefix/{prefix}", func(r chi.Router) {
        r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEventByPrefix)
    })
    r.Route("/{eventKey}", func(r chi.Router) {
        r.Get("/", s.handleGetEventTrigger)
        r.With(rateLimit(triggerRateLimitRequests, triggerRateLimitWindow)).Post("/send", s.handleSendEvent)
    })
})
```

**Why trigger rate limit?** Event sends are analogous to job triggers — external
callers sending signals. Same rate limit config makes sense.

---

## Commit 2: Fix step-before-trigger ordering in `startWaitForEventStep`

**Problem:** `startWaitForEventStep` calls `UpdateStepRunStatus(waiting)` before
`CreateEventTrigger`. If trigger creation fails (e.g., key conflict), the step
is stuck in `waiting` with no trigger row. The reconciliation reaper won't
catch this since it looks for triggers with stale steps, not steps without triggers.

**Files:**
- `internal/workflow/engine_steps.go` — reorder: create trigger first, then update step

**New order:**
1. Render event key (already first — fail-fast)
2. `CreateEventTrigger` (may fail with key conflict)
3. `UpdateStepRunStatus(StepWaiting)` (only if trigger creation succeeded)
4. `onTriggerCreate` callback

Same fix for `startSleepStep`: create trigger first, then update step.

**Edge case:** If `UpdateStepRunStatus` fails after trigger creation, we have an
orphan trigger. This is safe — the trigger will time out via the reaper and be
cleaned up by retention. Far better than a stuck step with no trigger.

---

## Commit 3: Add `notify_url` to `SDKWaitForEventRequest`

**Problem:** SDK endpoint doesn't accept `notify_url`, so SDK consumers
can't get webhook notifications when their triggers are created.

**Files:**
- `internal/api/sdk_wait_event.go` — add `NotifyURL string` field to
  `SDKWaitForEventRequest`, set `trigger.NotifyURL = req.NotifyURL`
- `internal/api/openapi.yaml` + `docs/openapi.yaml` — add `notify_url` to
  the SDK wait-for-event request schema

---

## Commit 4: Sleep step output with metadata

**Problem:** Sleep steps complete with no output, providing no context to
downstream steps.

**Files:**
- `internal/scheduler/reaper.go` — in `completeSleepTrigger`, compute
  sleep output JSON and set it on the step:
  ```go
  sleptSecs := now.Sub(trigger.RequestedAt).Seconds()
  output := json.RawMessage(fmt.Sprintf(
      `{"slept_for_secs":%.1f,"completed_at":"%s"}`,
      sleptSecs, now.UTC().Format(time.RFC3339),
  ))
  // Pass output in fields map
  fields["output"] = output
  ```

---

## Commit 5: Event trigger list filtering by `workflow_run_id` and `source_type`

**Problem:** `handleListEventTriggers` only filters by `status`. Debugging
specific workflows requires querying the DB directly.

**Files:**
- `internal/store/event_triggers.go` — extend `ListEventTriggersByProject` to
  accept optional `workflowRunID` and `sourceType` filter params
- `internal/api/event_triggers.go` — read `workflow_run_id` and `source_type`
  query params, pass to store
- `internal/api/server.go` — update `APIStore` interface signature
- `internal/api/mock_test.go` — update mock
- `internal/api/openapi.yaml` + `docs/openapi.yaml` — document new query params

**Store change:**
```go
func (q *Queries) ListEventTriggersByProject(
    ctx context.Context,
    projectID, status, workflowRunID, sourceType string,
    limit int, cursor *time.Time,
) ([]domain.EventTrigger, error) {
```
Add conditional WHERE clauses for the new params, same pattern as the
existing `status` filter.

---

## Commit 6: `DELETE /v1/events/{eventKey}` — cancel a trigger

**Problem:** No way to manually cancel a specific event trigger. Users must
wait for timeout.

**Files:**
- `internal/api/event_triggers.go` — add `handleCancelEventTrigger`:
  - Get trigger by key
  - Project scope check
  - Verify status is `waiting` (else 409)
  - `UpdateEventTriggerStatus(canceled, "canceled by user")`
  - For workflow_step source: call `workflowCallback.OnStepFailed` to
    respect on_failure policy
  - For job_run source: `UpdateRunStatus(waiting → canceled)`
  - Return 200 with updated trigger
- `internal/api/routes.go` — add `r.Delete("/", s.handleCancelEventTrigger)`
  inside the `/{eventKey}` route group
- `internal/api/server.go` — no change (uses existing store methods)
- `internal/api/openapi.yaml` + `docs/openapi.yaml` — document endpoint
- `internal/api/event_triggers_test.go` — add tests

---

## Commit 7: Unit tests for `OnStepCompleted` and `OnStepFailed`

**Problem:** Both methods have 0% direct unit test coverage (only tested
transitively through reaper tests).

**Files:**
- `internal/workflow/callback_test.go` — add:
  - `TestOnStepCompleted_AdvancesWorkflow`: mock step run as completed,
    verify `fanInAndStartReadyChildren` is called (via `IncrementStepDeps`)
  - `TestOnStepCompleted_StepNotFound`: returns cleanly when step ID doesn't
    match any step run
  - `TestOnStepFailed_RespectsOnFailureContinue`: mock step with
    `on_failure: continue`, verify workflow is NOT failed
  - `TestOnStepFailed_StepNotFound`: logs warning, returns cleanly

---

## Commit 8: Documentation update

**Files:**
- `docs/event-triggers-final-improvements.md` — mark all items complete
- `docs/event-triggers-next-steps.md` — add "Post-GA" section with deferred items:
  - Transaction safety (requires store interface refactor)
  - Reconciliation query index optimization
  - Batch resolve atomicity
  - Webhook notification retry
  - SSE streaming for trigger status
  - Event trigger audit log (`sent_by` field)
  - Event trigger stats endpoint

---

## Deferred Items (not in this PR)

These require larger architectural changes or are feature work beyond
the scope of bug fixes and hardening:

| # | Item | Reason for deferral |
|---|------|---------------------|
| 1 | Transaction safety for handleSendEvent | Requires exposing `TxBeginner` through `APIStore` interface or adding a `RunInTx` method — store interface refactor |
| 3 | Reconciliation query index | Needs benchmarking with production-scale data to validate |
| 4 | Batch resolve atomicity | Design decision: document as best-effort or add tx support (same blocker as #1) |
| 7 | Webhook notification retry | Feature work — exponential backoff, retry queue, DLQ |
| 8 | SSE/WebSocket streaming | Feature work — new endpoint, CDC subscription |
| 10 | Event trigger audit log | Schema change, new migration |
| 14 | Event trigger stats endpoint | Feature work — aggregation queries |
| 15 | Migration squash | Only safe before first production deploy |

---

## Execution Order

```
Commit 1 (rate limiting)     → build/vet/lint/test → commit
Commit 2 (step ordering)     → build/vet/lint/test → commit
Commit 3 (SDK notify_url)    → build/vet/lint/test → commit
Commit 4 (sleep output)      → build/vet/lint/test → commit
Commit 5 (list filtering)    → build/vet/lint/test → commit
Commit 6 (cancel endpoint)   → build/vet/lint/test → commit
Commit 7 (unit tests)        → build/vet/lint/test → commit
Commit 8 (docs)              → commit
→ push → wait for CI green
```

## Estimated Scope

- **~12 files modified**, 2 new test functions
- **0 new migrations** (all code-level changes)
- **~350 lines added**, ~30 lines modified
