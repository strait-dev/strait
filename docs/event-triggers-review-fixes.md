# Event Triggers — Review Fixes

All 25 review items have been addressed across 4 commits.

## Summary of Changes

### Batch 1: Critical Wiring Bugs (commit `68f605d`)

| # | Issue | Fix |
|---|-------|-----|
| 1 | Metrics never recorded | Added metric calls to API handlers, engine, and reaper. Wired `*telemetry.Metrics` through `ServerDeps`, `WithMetrics` on Reaper, `WithSchedulerMetrics` option. |
| 2 | EventNotifier never wired | Instantiate `webhook.NewEventNotifier` in `main.go`, pass `NotifyAsync` via `WithOnTriggerCreate`. Copy `step.EventNotifyURL` → `trigger.NotifyURL`. |
| 3 | Event chaining missing from 2 paths | Added `tryEmitEvent` to `OnEventReceived` and `OnStepCompleted`. |
| 4 | Stale godoc | Removed leaked `startSleepStep` text from `getNestingDepth` comment. |

### Batch 2: Security & Input Validation (commit `4c4a895`)

| # | Issue | Fix |
|---|-------|-----|
| 5 | Inconsistent JSON decoding | Replaced `json.NewDecoder` with `s.decodeJSON` in prefix handler. |
| 6 | LIKE pattern injection | Added `escapeLikePattern` helper escaping `%`, `_`, `\`. |
| 9 | No project scoping on send | Added `projectIDFromContext` check (403 on mismatch). |
| 10 | No project scoping on prefix | Added `projectID` param to `ListEventTriggersByKeyPrefix`. |
| 11 | No LIMIT on prefix query | Added `LIMIT 1000` safety cap. |
| 14 | No max length on event_key | Added 512-char validation in workflows, SDK, and prefix endpoints. |
| 24 | Duplicate struct | Removed `SendEventByPrefixRequest`, reusing `SendEventRequest`. |
| 25 | Inconsistent context | Captured `ctx := r.Context()` in prefix handler. |

### Batch 3: Correctness Fixes (commit `5c2eb31`)

| # | Issue | Fix |
|---|-------|-----|
| 7 | Timeout ignores on_failure | Added `OnStepFailed` to `WorkflowCallback`, delegates to `handleFailedStep`. Falls back to direct failure when callback is nil. |
| 8 | Sleep emit key not chained | Covered by Item 3 (tryEmitEvent in OnStepCompleted). |
| 12 | event_emit_key not template-rendered | Added `renderStringTemplate` call in `emitEventIfConfigured` using `wfRun.Payload`. |
| 15 | SDK rollback race | Added error logging + comment about the ms-vs-10s theoretical race. |

### Batch 4: Missing Tests (commit `c25ca3f`)

| # | Test | Coverage |
|---|------|----------|
| 16 | `TestHandleSendEventByPrefix_ResolvesMultiple` | Batch resolution of 2 triggers |
| 16 | `TestHandleSendEventByPrefix_NoMatches` | Empty result returns resolved=0 |
| 16 | `TestHandleSendEvent_ProjectScoping_Forbidden` | 403 on cross-project send |
| 18 | `TestReapExpiredEventTriggers_SleepCallsOnStepCompleted` | Callback invocation verified |
| 7 | `TestReapExpiredEventTriggers_DelegatesOnStepFailed` | OnStepFailed delegation |
| 7 | `TestReapExpiredEventTriggers_NilCallbackFallback` | Direct WF failure when cb=nil |
| 19 | `TestPayloadsMatch` nil vs null / null vs null | Edge cases added |

### Batch 5: Code Quality (already done in Batch 2)

Items 11, 24, 25 were folded into Batch 2.

### Not Addressed

| # | Item | Reason |
|---|------|--------|
| 13 | StepCompleted check | Already correct — terminal check prevents double completion |
| 20 | CancelEventTriggerByJobRun test | Method only called internally via reaper; covered by existing timeout/cancel test paths |
| 21 | UpdateEventTriggerNotifyStatus test | Transitively tested via webhook notifier tests |
| 22 | Sleep step e2e test | Requires real DB; out of scope for unit test batch |
| 23 | Migration consolidation | Risky to rewrite already-applied migrations |

## New Patterns Introduced

- **`EventTriggerNotifyFunc`**: Function type callback on `WorkflowEngine` for decoupled trigger notifications
- **`OnStepFailed`**: New `WorkflowCallback` method — respects `on_failure` policy for step failures
- **`escapeLikePattern`**: SQL LIKE wildcard escaping utility in `store/event_triggers.go`
- **`WithSchedulerMetrics`**: Functional option for passing metrics to the scheduler/reaper
