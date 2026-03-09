# Event Triggers — Review Fixes Plan

25 items from PR review, organized into 6 batches for sequential implementation.
Each batch groups related fixes to minimize file churn. Commit after each batch.

---

## Batch 1: Critical Wiring Bugs (Items 1–4)

Three features are built but disconnected — metrics, webhook notifier, and event
chaining from `OnEventReceived`/`OnStepCompleted`. Plus a stale comment.

### Item 1: Wire up metrics recording

**Problem:** 4 OTel metrics defined in `telemetry/metrics.go` but never recorded.

**Files:**
- `internal/api/event_triggers.go` — record `EventTriggersReceived` in `handleSendEvent` and `handleSendEventByPrefix`
- `internal/api/sdk_wait_event.go` — record `EventTriggersCreated` after `CreateEventTrigger`
- `internal/workflow/engine_steps.go` — record `EventTriggersCreated` in `startWaitForEventStep` and `startSleepStep`
- `internal/scheduler/reaper.go` — record `EventTriggersTimedOut` in `reapExpiredEventTriggers`, record `EventTriggerWaitDuration` in both timeout and completion paths
- `internal/api/server.go` — add `Metrics *telemetry.Metrics` to `ServerDeps`, store on `Server`
- `internal/workflow/engine.go` — add `metrics *telemetry.Metrics` field, accept in constructor
- `internal/scheduler/reaper.go` — add `metrics *telemetry.Metrics` field, accept in constructor
- `cmd/strait/services.go` — pass `metrics` through to Server, Engine, and Reaper

**Approach:**
- Server: add `metrics *telemetry.Metrics` field; in `handleSendEvent` after successful update call `s.metrics.EventTriggersReceived.Add(ctx, 1)`; compute wait duration from `trigger.RequestedAt` to `now` and record `s.metrics.EventTriggerWaitDuration.Record(ctx, duration.Seconds())`
- Engine: add `metrics` param to `NewWorkflowEngine`; in `startWaitForEventStep`/`startSleepStep` after `CreateEventTrigger` call `e.metrics.EventTriggersCreated.Add(ctx, 1)` (nil-safe)
- Reaper: add `metrics` param to `NewReaper`; in expired trigger loop call `r.metrics.EventTriggersTimedOut.Add(ctx, 1)` (nil-safe); in `completeSleepTrigger` compute duration and record
- SDK handler: `s.metrics.EventTriggersCreated.Add(ctx, 1)` after `CreateEventTrigger`
- All metric calls must be nil-safe (check `metrics != nil`)

### Item 2: Wire up EventNotifier

**Problem:** `webhook.EventNotifier` is built and tested but never instantiated or called. `startWaitForEventStep` doesn't copy `step.EventNotifyURL` to `trigger.NotifyURL`.

**Files:**
- `internal/workflow/engine_steps.go` — in `startWaitForEventStep`, set `trigger.NotifyURL = step.EventNotifyURL`
- `internal/workflow/engine.go` — add `notifier` field (interface `EventNotifyFunc func(*domain.EventTrigger)`)
- `cmd/strait/services.go` — create `webhook.NewEventNotifier(queries, slog.Default())`, pass `notifier.NotifyAsync` to engine
- `internal/workflow/engine_steps.go` — after `CreateEventTrigger` in `startWaitForEventStep`, call `e.notifier(trigger)` if non-nil

**Design choice:** Use a function type `func(*domain.EventTrigger)` rather than the full `EventNotifier` interface to keep the engine decoupled from the webhook package.

### Item 3: Add `tryEmitEvent` to `OnEventReceived` and `OnStepCompleted`

**Problem:** Event chaining only works for `OnJobRunTerminal`. Steps completed via event receipt or sleep completion won't auto-emit.

**Files:**
- `internal/workflow/callback.go` — add `s.tryEmitEvent(ctx, targetStepRun)` in `OnEventReceived` before `fanInAndStartReadyChildren`; add `s.tryEmitEvent(ctx, target)` in `OnStepCompleted` before `fanInAndStartReadyChildren`

### Item 4: Fix stale godoc comment

**Problem:** `getNestingDepth` comment has leftover `startSleepStep` text.

**File:** `internal/workflow/engine_steps.go` — remove the two spurious comment lines.

---

## Batch 2: Security & Input Validation (Items 5–6, 9–10, 14)

### Item 5: Use `s.decodeJSON` in `handleSendEventByPrefix`

**Problem:** Uses raw `json.NewDecoder` instead of `s.decodeJSON`, bypassing `DisallowUnknownFields`.

**File:** `internal/api/event_triggers.go` — replace `json.NewDecoder(r.Body).Decode(&req)` with `s.decodeJSON(r, &req)`.

### Item 6: Escape LIKE wildcards in prefix query

**Problem:** User-supplied `%` and `_` in prefix match unintended keys.

**File:** `internal/store/event_triggers.go` — add `escapeLikePattern` helper that replaces `%` → `\%`, `_` → `\_`, `\` → `\\`; update query to use `WHERE event_key LIKE $1 AND status = 'waiting'` and pass `escapeLikePattern(prefix) + "%"` as the argument.

### Item 9: Add project scoping to `handleSendEvent`

**Problem:** Any authenticated user can send events to any trigger if they know the key.

**File:** `internal/api/event_triggers.go` — after retrieving the trigger, check `projectID := projectIDFromContext(r.Context()); if projectID != "" && trigger.ProjectID != projectID { respondError 403 }`. The `projectID != ""` guard allows internal-secret auth (no project) to still work.

### Item 10: Add project scoping to `handleSendEventByPrefix` and `ListEventTriggersByKeyPrefix`

**Files:**
- `internal/store/event_triggers.go` — add `projectID string` parameter to `ListEventTriggersByKeyPrefix`; add `AND project_id = $2` to query when non-empty
- `internal/api/server.go` — update `APIStore` interface
- `internal/api/event_triggers.go` — pass `projectIDFromContext(r.Context())` to store call
- `internal/api/mock_test.go` — update mock signature

### Item 14: Validate `event_key` max length

**Files:**
- `internal/api/workflows.go` — in `validateWorkflowSteps`, add `if len(step.EventKey) > 512 { return error }`
- `internal/api/sdk_wait_event.go` — add `if len(req.EventKey) > 512 { respondError 400 }`
- `internal/api/event_triggers.go` — in `handleSendEvent`, validate `len(eventKey) > 512`

---

## Batch 3: Correctness Fixes (Items 7–8, 12, 15)

### Item 7: Respect `on_failure` policy for event trigger timeouts

**Problem:** Reaper unconditionally fails the workflow when a `wait_for_event` step times out, ignoring `on_failure: "continue"`.

**Current flow in `reapExpiredEventTriggers`:**
1. Mark trigger `timed_out`
2. Mark step `failed` with error
3. Directly `UpdateWorkflowRunStatus` → failed

**Fix:** Instead of directly failing the workflow, delegate to the `WorkflowCallback`:
- Add `OnStepFailed(ctx, workflowRunID, stepRunID string)` to reaper's `WorkflowCallback` interface
- Implement on `StepCallback`: looks up step run, calls `handleFailedStep` (which respects `on_failure`)
- In `reapExpiredEventTriggers`, after marking the step failed, call `r.workflowCallback.OnStepFailed(ctx, trigger.WorkflowRunID, trigger.WorkflowStepRunID)` instead of directly failing the workflow run
- Keep the direct-fail as fallback when `workflowCallback == nil`
- **Same pattern for approval timeout** in `reapExpiredApprovals` (future improvement, not this PR)

**Files:**
- `internal/scheduler/reaper.go` — add `OnStepFailed` to interface, use in `reapExpiredEventTriggers` for workflow_step source
- `internal/workflow/callback.go` — implement `OnStepFailed`
- `internal/scheduler/mock_test.go` — update mock (nil-safe, existing tests pass `nil` callback)

### Item 8: Add `tryEmitEvent` to `completeSleepTrigger` path

**Problem:** `OnStepCompleted` doesn't call `tryEmitEvent`, so sleep steps with `event_emit_key` won't chain.

**File:** `internal/workflow/callback.go` — in `OnStepCompleted`, add `s.tryEmitEvent(ctx, target)` before `fanInAndStartReadyChildren`.

**Note:** This is the same file as Item 3. Item 3 adds it to `OnEventReceived` and `OnStepCompleted`. If doing both in Batch 1, Item 8 is already covered. But double-check that both paths are handled.

### Item 12: Template-render `event_emit_key`

**Problem:** `emitEventIfConfigured` uses raw `step.EventEmitKey` without template expansion.

**File:** `internal/workflow/callback.go` — in `emitEventIfConfigured`, render the key: `renderedKey := renderStringTemplate(step.EventEmitKey, stepRun.Output)` (use step output as context, since we want the completed step's data to drive the key). If empty after render, skip.

**Note:** Need to also consider using the workflow payload as template context. Since `tryEmitEvent` already has the `wfRun`, pass it to `emitEventIfConfigured` and use `wfRun.Payload` as the template context (consistent with `startWaitForEventStep`).

**Updated signature:** `emitEventIfConfigured(ctx, stepRun, step, wfRun)` → render with `wfRun.Payload`.

### Item 15: SDK wait-for-event rollback race condition

**Problem:** Between `UpdateRunStatus(executing→waiting)` and the `CreateEventTrigger` failure rollback `UpdateRunStatus(waiting→executing)`, the reaper could time out the run.

**Fix:** Use optimistic approach — don't rollback. Instead, if `CreateEventTrigger` fails, leave the run in `waiting` and return 500. The reconciliation reaper (`reapInconsistentEventTriggers`) will find it eventually since there's no matching trigger. Add a comment explaining this.

**Alternative (simpler):** Keep the rollback but log a warning if it fails (already the case since `_ =` ignores the error). Add a comment about the race. The window is tiny (milliseconds) and the reaper runs on 10s+ intervals. Accept the theoretical race.

**Decision:** Keep current code, add a comment explaining the race window, and add error logging on the rollback failure.

**File:** `internal/api/sdk_wait_event.go` — replace `_ = s.store.UpdateRunStatus(...)` with logged rollback + comment about the race.

---

## Batch 4: Missing Tests (Items 16–22)

### Item 16: Test `handleSendEventByPrefix`

**File:** `internal/api/event_triggers_test.go`

Tests:
- `TestHandleSendEventByPrefix_ResolvesMultiple` — 2 waiting triggers with same prefix, both resolved
- `TestHandleSendEventByPrefix_NoMatches` — returns `{"resolved": 0}`
- `TestHandleSendEventByPrefix_EmptyPrefix` — returns 400

### Item 17: Test event chaining (`emitEventIfConfigured`)

**File:** `internal/workflow/engine_test.go` (alongside other callback tests)

Test: `TestOnJobRunTerminal_EmitsEvent` — step with `event_emit_key`, mock `GetEventTriggerByEventKey` returns a waiting trigger, verify `UpdateEventTriggerStatus` called with `received` status and step output as payload.

### Item 18: Test `completeSleepTrigger` with callback

**File:** `internal/scheduler/reaper_test.go`

Test: `TestReapExpiredEventTriggers_SleepCallsOnStepCompleted` — provide a mock `WorkflowCallback` (not nil), verify `OnStepCompleted` is called with correct args.

### Item 19: Test `payloadsMatch` nil vs `null` edge case

**File:** `internal/api/event_triggers_test.go`

Add cases to `TestPayloadsMatch`:
- `nil` vs `json.RawMessage("null")` → should return `false` (different semantics: no payload vs explicit null)
- `json.RawMessage("null")` vs `json.RawMessage("null")` → `true`

### Item 20: Test `CancelEventTriggerByJobRun`

**File:** `internal/store/event_triggers_test.go` (new file) or add to existing store tests.

Since store tests use a real DB (integration tag), add a unit test with mock DB in the appropriate test file. Or add to `internal/scheduler/reaper_test.go` since that's where it's consumed.

Test: `TestCancelEventTriggerByJobRun_CallsStore` — verify the mock is called correctly.

### Item 21: Test `UpdateEventTriggerNotifyStatus`

**File:** Already tested transitively through `internal/webhook/event_notify_test.go`. Add explicit assertion that the mock's stored status matches expected values (already done). Mark as covered.

### Item 22: E2E test for sleep step lifecycle

**File:** `internal/e2e/event_triggers_e2e_test.go`

Test: `TestE2E_SleepStep_CompletesViaReaper` — create workflow with sleep step (short duration like 1s), trigger it, verify step is waiting, call `ReapOnce`, verify step completed and workflow advances.

---

## Batch 5: Code Quality / Cosmetic (Items 11, 23–25)

### Item 11: Add LIMIT to `ListEventTriggersByKeyPrefix`

**File:** `internal/store/event_triggers.go` — add `LIMIT 1000` to the query (or accept a `limit int` parameter). Use hardcoded 1000 for safety.

### Item 23: Migration consolidation — SKIP

Consolidating migrations 000049–000053 would require rewriting migration history that may already be applied in staging/dev databases. **Skip this** — it's cosmetic and risky. Document that they should be squashed before the first production deploy if needed.

### Item 24: Remove duplicate `SendEventByPrefixRequest`

**File:** `internal/api/event_triggers.go` — delete `SendEventByPrefixRequest`, reuse `SendEventRequest` in `handleSendEventByPrefix`.

### Item 25: Consistent `context` capture in prefix handler

**File:** `internal/api/event_triggers.go` — capture `ctx := r.Context()` at the top of `handleSendEventByPrefix` and use it throughout, matching other handlers.

---

## Batch 6: Documentation Update

Update `docs/event-triggers-next-steps.md` to mark all items as completed and add notes about the new patterns introduced (metrics wiring, notifier integration, `on_failure` delegation).

---

## Execution Order

```
Batch 1 → build/vet/lint/test → commit
Batch 2 → build/vet/lint/test → commit
Batch 3 → build/vet/lint/test → commit
Batch 4 → build/vet/lint/test → commit
Batch 5 → build/vet/lint/test → commit
Batch 6 → build/vet/lint/test → commit → push → monitor CI
```

## Estimated Scope

- **~15 files modified**, ~3 new test functions
- **0 new migrations** (all fixes are code-level)
- **~400 lines added**, ~50 lines modified
- All existing tests must continue to pass
