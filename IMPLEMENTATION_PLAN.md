# Implementation Plan: Complete Sandbox & Compensation Features

## Current State (Baseline)

### Coverage
| Package | Coverage | Notes |
|---------|----------|-------|
| `internal/sandbox` | 64.4% | `Execute` 29%, `ExecuteStream` 29%, `dispatchSandbox` 11% — no real gRPC tests |
| `internal/sandbox/v1` | 34.8% | Generated protobuf — most getters untested (expected) |
| `internal/workflow` | 81.4% | `CompensateFailedWorkflow` 66%, `RetryFailedCompensation` 73%, `cancelActiveSteps` 77% |
| `internal/worker` | 86.6% | `dispatchSandbox` 11%, `SendWebhook` 0% |
| `internal/api` | 72.8% | No tests for sandbox job creation validation, no `handleRetryCompensation` test |
| `internal/store` | 7.7% | Unit tests only — integration tests are separate build tag |
| **Forge (Elixir)** | 66.7% | All tests use `nil` stream, below 90% threshold |

### Test Counts
- **Go**: 1,468 tests across 21 packages (all passing with `-race`)
- **Elixir**: 6 tests (all passing)
- **Integration/E2E**: 0 tests cover sandbox, compensation, or cancel features

### Dead Code / Missing Wiring
1. `CancelEndpointURL` — stored and exposed in API, but never dispatched
2. `StatusCanceling` — in FSM but never set by the worker
3. `job_versions` INSERT — missing 4 sandbox columns (data loss on job update)
4. `workflow_version_steps` — migration didn't add `compensate_step_ref` column
5. Sandbox client has no auto-reconnect
6. No OTel spans in compensation or sandbox dispatch paths
7. OpenAPI spec doesn't include any new fields or endpoints
8. Forge doesn't return `RESOURCE_EXHAUSTED` when at capacity
9. Forge doesn't enforce `memory_bytes` or `network_enabled` limits

---

## Implementation Phases

### Phase 1: Critical Data Fixes
**Goal**: Fix data integrity issues that would cause silent data loss or runtime failures.

#### 1.1 Migration: Add sandbox columns to `job_versions` table
- **File**: `migrations/000052_add_sandbox_fields_to_versions.up.sql`
- Add `execution_mode`, `sandbox_code`, `sandbox_language`, `cancel_endpoint_url` to `job_versions`
- Add `compensate_step_ref` to `workflow_version_steps`
- **Down migration**: Drop the columns

#### 1.2 Update `job_versions` snapshot INSERT in store
- **File**: `internal/store/jobs.go` — the CTE inside `UpdateJob`
- Add the 4 new columns to both the INSERT column list and the SELECT source
- Verify with a unit test that creates a job with sandbox fields, updates it, and reads the version

#### 1.3 Tests
- **Unit test** (`internal/store/`): Mock test verifying snapshot SQL includes new columns
- **Integration test** (`internal/store/store_integration_test.go`):
  - Create a sandbox job → update it → fetch version → assert sandbox fields preserved
  - Create a workflow step with `compensate_step_ref` → snapshot version → fetch version → assert `compensate_step_ref` preserved
  - Create a workflow run → assert `compensation_status` defaults to `none`

**Commit**: `fix(store): add sandbox columns to job_versions snapshot and version tables`

---

### Phase 2: Cancel Endpoint Dispatch
**Goal**: When a run transitions to `canceling`, send a cancel webhook to `job.CancelEndpointURL`.

#### 2.1 Implement `dispatchCancel` in worker
- **File**: `internal/worker/executor_cancel.go` (new file)
- `func (e *Executor) dispatchCancel(ctx context.Context, job *domain.Job, run *domain.JobRun) error`
- POST to `job.CancelEndpointURL` with JSON body: `{"run_id": run.ID, "job_id": job.ID, "job_slug": job.Slug}`
- Timeout: reuse `ExecutorHTTPTimeout` config
- Log success/failure, don't fail the cancel if dispatch fails (best-effort)

#### 2.2 Wire `StatusCanceling` into the executor
- **File**: `internal/worker/executor_dispatch.go`
- When context is canceled during execution AND `job.CancelEndpointURL != ""`:
  1. Transition run to `StatusCanceling`
  2. Call `dispatchCancel(ctx, job, run)` with a fresh context (parent was canceled)
  3. Transition run to `StatusCanceled`
- When context is canceled AND `job.CancelEndpointURL == ""`:
  - Transition directly to `StatusCanceled` (existing behavior)

#### 2.3 Wire cancel dispatch into CompensationEngine
- **File**: `internal/workflow/compensation.go` — `cancelActiveSteps`
- When transitioning a job run to `canceling`, if the job has a `CancelEndpointURL`, dispatch the cancel webhook
- This requires the `CompensationEngine` to have access to the job store (add `GetJob` to `CompensationStore`)

#### 2.4 Tests
- **Unit tests** (`internal/worker/executor_cancel_test.go`):
  - `TestDispatchCancel_Success` — mock HTTP server returns 200
  - `TestDispatchCancel_ServerError` — returns 500, cancel still succeeds (best-effort)
  - `TestDispatchCancel_Timeout` — slow server, times out, cancel still succeeds
  - `TestDispatchCancel_EmptyURL` — no-op
  - `TestDispatchCancel_InvalidURL` — error logged, cancel still succeeds
  - `TestDispatchCancel_RequestBody` — verify JSON body has correct fields

- **Unit tests** (`internal/worker/executor_test.go`):
  - `TestExecutor_CancelWithCancelEndpoint` — run transitions executing → canceling → canceled
  - `TestExecutor_CancelWithoutCancelEndpoint` — run transitions executing → canceled directly

- **Unit tests** (`internal/workflow/compensation_test.go`):
  - `TestCancelActiveSteps_WithCancelEndpoint` — verify cancel webhook dispatched
  - `TestCancelActiveSteps_CancelEndpointFailure` — verify step still canceled despite webhook failure

**Commit**: `feat(worker): implement CancelEndpointURL dispatch on run cancellation`

---

### Phase 3: Sandbox Client Reconnect
**Goal**: Auto-reconnect to Forge when the connection drops.

#### 3.1 Add reconnect loop to sandbox client
- **File**: `internal/sandbox/client.go`
- Add `Reconnect(ctx context.Context) error` method with exponential backoff (1s → 2s → 4s → 8s → 16s cap)
- Add `StartReconnectLoop(ctx context.Context)` goroutine that watches connection state and auto-reconnects
- Cancel the loop via context (shutdown)
- Log each reconnect attempt and outcome

#### 3.2 Wire reconnect into server startup
- **File**: `cmd/strait/services.go`
- After `sandboxClient.Connect()`, start `sandboxClient.StartReconnectLoop(ctx)` in a goroutine
- Loop exits on shutdown context cancellation

#### 3.3 Tests
- **Unit tests** (`internal/sandbox/client_test.go`):
  - `TestClient_Reconnect_Success` — disconnect, reconnect succeeds
  - `TestClient_Reconnect_BackoffIncreases` — verify delay doubles
  - `TestClient_Reconnect_MaxBackoff` — verify cap at 16s
  - `TestClient_Reconnect_ContextCanceled` — verify loop exits
  - `TestClient_ReconnectLoop_DetectsDisconnect` — verify automatic reconnect triggers
  - `TestClient_Execute_AfterReconnect` — verify Execute works after reconnect

**Commit**: `feat(sandbox): add auto-reconnect with exponential backoff`

---

### Phase 4: Forge Improvements
**Goal**: Make Forge production-ready with proper error handling, resource enforcement, and testability.

#### 4.1 Return `RESOURCE_EXHAUSTED` when at capacity
- **File**: `apps/forge/lib/forge/grpc/sandbox_server.ex`
- Catch `{:error, :max_children}` from `SandboxSupervisor.start_execution`
- Return `GRPC.RPCError` with status `:resource_exhausted`

#### 4.2 Add mock stream module for tests
- **File**: `apps/forge/test/support/mock_stream.ex` (new)
- Implements a process that collects `GRPC.Server.send_reply` calls
- API: `MockStream.start_link()` → `{:ok, stream_pid}`
- API: `MockStream.get_events(stream_pid)` → list of events
- Override `GRPC.Server.send_reply/2` in test — or have Runner accept a callback module

#### 4.3 Refactor Runner to use a send callback
- **File**: `apps/forge/lib/forge/sandbox/runner.ex`
- Add `:send_fn` to state — defaults to `&GRPC.Server.send_reply/2`
- Replace all direct `GRPC.Server.send_reply` calls with `state.send_fn.(stream, event)`
- Tests can inject a capture function

#### 4.4 Tests (Elixir)
- **Runner tests** (`test/forge/sandbox/runner_test.exs`):
  - `test "streams log events for stdout lines"` — inject mock send_fn, run Python `print()`, verify log events
  - `test "streams result event with success=true on exit 0"` — verify result event fields
  - `test "streams result event with success=false on non-zero exit"` — run `sys.exit(1)`, verify error
  - `test "tracks duration_ms correctly"` — run code with `time.sleep(0.5)`, verify duration ≥ 500
  - `test "cleans up temp file after success"` — check temp dir before/after
  - `test "cleans up temp file after timeout"` — timeout, verify no orphan file
  - `test "passes environment variables to process"` — `print(os.environ['MY_VAR'])`, verify in log event
  - `test "passes payload via FORGE_PAYLOAD env var"` — verify `FORGE_PAYLOAD` available

- **SandboxServer tests** (`test/forge/grpc/sandbox_server_test.exs` — new):
  - `test "returns RESOURCE_EXHAUSTED when at max capacity"` — set max_children=0, attempt execute

- **Sandbox (public API) tests** (`test/forge/sandbox_test.exs`):
  - Rewrite existing 3 tests to use mock send_fn instead of nil stream
  - Add `test "captures all streamed events in order"` — verify log → log → result sequence
  - Add `test "returns error tuple when runner crashes"` — inject code that crashes

**Target**: Forge coverage ≥ 90%

**Commit**: `feat(forge): RESOURCE_EXHAUSTED status, mock stream, comprehensive tests`

---

### Phase 5: Sandbox Client & Executor Tests
**Goal**: Bring `sandbox.Client` and `executor_sandbox.go` coverage to ≥ 85%.

#### 5.1 Add gRPC test server for client tests
- **File**: `internal/sandbox/testserver_test.go` (new)
- In-process gRPC server implementing `SandboxExecutorServer`
- Configurable behavior: success, error, stream events, delay, cancel

#### 5.2 Client tests with real gRPC
- **File**: `internal/sandbox/client_test.go` — expand:
  - `TestClient_Execute_Success` — server streams log + result, verify `ExecuteResult`
  - `TestClient_Execute_ErrorFromServer` — server returns gRPC error, verify error
  - `TestClient_Execute_NoResultEvent` — server streams log only (no result), verify `ErrNoResult`
  - `TestClient_Execute_ContextCanceled` — cancel ctx mid-stream, verify context error
  - `TestClient_Execute_StreamMultipleEvents` — 5 log events + 1 result, verify all collected
  - `TestClient_ExecuteStream_HandlerError` — handler returns error, verify propagated
  - `TestClient_ExecuteStream_NotConnected` — no Connect(), verify `ErrNotConnected`
  - `TestClient_Execute_WithEnvVars` — verify env vars sent in request
  - `TestClient_Execute_WithTimeout` — verify timeout sent as resource limit
  - `TestClient_WithDialOptions` — verify custom dial options applied

#### 5.3 Executor sandbox dispatch tests
- **File**: `internal/worker/executor_sandbox_test.go` — expand:
  - `TestDispatchSandbox_Success` — mock client returns result, verify JSON output + trace
  - `TestDispatchSandbox_NotConfigured` — nil client, verify `errSandboxNotConfigured`
  - `TestDispatchSandbox_ExecutionFailed` — client returns error, verify error propagated
  - `TestDispatchSandbox_NoResult` — client returns nil result, verify error
  - `TestDispatchSandbox_FailedResult` — result with `success=false`, verify `EndpointError`
  - `TestDispatchSandbox_EmptyResult` — result with empty string, verify nil output
  - `TestDispatchSandbox_WithEnvironment` — job has `EnvironmentID`, verify env vars resolved
  - `TestDispatchSandbox_EnvironmentResolveFails` — env resolution fails, execution continues
  - `TestDispatchSandbox_StreamEvents_Published` — verify `publishEvent` called for log events
  - `TestDispatchSandbox_DurationFromResult` — verify `execTrace.DispatchMs` uses result's `duration_ms`
  - `TestDispatchSandbox_DurationFallback` — result has 0 duration, verify fallback to wall clock

**Target**: `sandbox` package ≥ 85%, `executor_sandbox` ≥ 90%

**Commit**: `test(sandbox): gRPC test server, comprehensive client and executor tests`

---

### Phase 6: Compensation Engine Tests
**Goal**: Full coverage of all compensation paths including edge cases.

#### 6.1 Expand unit tests
- **File**: `internal/workflow/compensation_test.go` — add:
  - `TestCancelWorkflowRun_NilWorkflowRun` — store returns nil, verify error
  - `TestCancelWorkflowRun_TerminalState` — already completed, verify error message
  - `TestCancelWorkflowRun_CancelActiveStepsFails` — verify error propagated
  - `TestCancelWorkflowRun_MarkCanceledFails` — compensation succeeds but status update fails
  - `TestCancelWorkflowRun_NoCompensationSteps` — no steps have `compensate_step_ref`, verify `CompensationNone`
  - `TestCancelWorkflowRun_AllCompensationFails` — all enqueue calls fail, verify `CompensationFailed`
  - `TestCancelWorkflowRun_PartialCompensation` — some succeed some fail, verify `CompensationPartial`
  - `TestCancelWorkflowRun_CompensationStepNotFound` — `compensate_step_ref` points to nonexistent step
  - `TestCompensateFailedWorkflow_Success` — failed workflow, compensation runs
  - `TestCompensateFailedWorkflow_NilRun` — verify error
  - `TestRetryFailedCompensation_WrongStatus` — status is `completed`, verify "not retryable"
  - `TestRetryFailedCompensation_SkipsCompleted` — already-completed comp steps skipped
  - `TestRetryFailedCompensation_RetriesFailed` — failed comp steps re-enqueued
  - `TestRetryFailedCompensation_MixedResults` — some completed, some failed, some retried
  - `TestCancelActiveSteps_MixedStatuses` — running, waiting, completed, failed steps
  - `TestCancelActiveSteps_JobRunCancelGraceful` — executing → canceling transition
  - `TestCancelActiveSteps_JobRunCancelDirect` — queued → canceled transition
  - `TestCancelActiveSteps_JobRunCancelAllFail` — all transitions fail, verify logged but not error
  - `TestRunCompensation_ReverseChronologicalOrder` — verify compensation order matches reverse finish time
  - `TestRunCompensation_CreateStepRunFails` — verify step counted as failed
  - `TestRunCompensation_EnqueueFails` — verify step counted as failed
  - `TestRunCompensation_UpdateStepRunStatusFails` — enqueue succeeds but status update fails, still counted as compensated
  - `TestRunCompensation_PayloadPropagation` — original step output → compensation job payload

**Target**: `workflow` package ≥ 90%

**Commit**: `test(workflow): exhaustive compensation engine coverage`

---

### Phase 7: API Layer Tests
**Goal**: Test all new API fields and endpoints.

#### 7.1 Job creation/update tests with sandbox fields
- **File**: `internal/api/handler_test.go` or `internal/api/jobs_test.go` (new):
  - `TestHandleCreateJob_SandboxMode` — `execution_mode=sandbox`, `sandbox_code`, `sandbox_language` set
  - `TestHandleCreateJob_SandboxMissingCode` — 400 error
  - `TestHandleCreateJob_SandboxMissingLanguage` — 400 error
  - `TestHandleCreateJob_HTTPModeDefault` — no `execution_mode`, defaults to `http`
  - `TestHandleCreateJob_InvalidExecutionMode` — `execution_mode=docker`, 400 error
  - `TestHandleCreateJob_CancelEndpointURL` — valid URL set, verify stored
  - `TestHandleCreateJob_InvalidCancelEndpointURL` — bad URL, 400 error
  - `TestHandleUpdateJob_SandboxFields` — update all 4 fields
  - `TestHandleCloneJob_SandboxFieldsCopied` — clone preserves sandbox config
  - `TestHandleBatchCreateJobs_SandboxFields` — batch create with sandbox jobs

#### 7.2 Workflow step tests with compensation
- **File**: `internal/api/workflow_handler_test.go` — add:
  - `TestHandleCreateWorkflow_WithCompensateStepRef` — step with `compensate_step_ref` set
  - `TestHandleUpdateWorkflowSteps_CompensateStepRef` — update steps preserves `compensate_step_ref`

#### 7.3 Compensation endpoint tests
- **File**: `internal/api/workflow_handler_test.go` — add:
  - `TestHandleRetryCompensation_Success` — mock compensator returns result
  - `TestHandleRetryCompensation_NotFound` — verify 404
  - `TestHandleRetryCompensation_NotRetryable` — verify 400
  - `TestHandleRetryCompensation_InternalError` — verify 500
  - `TestHandleRetryCompensation_NilEngine` — verify 501

**Commit**: `test(api): sandbox job validation, compensation endpoint, workflow step compensation`

---

### Phase 8: Integration Tests (Go — Store + Queue)
**Goal**: Verify new columns work against real Postgres.

#### 8.1 Store integration tests
- **File**: `internal/store/store_integration_test.go` — add:
  - `TestCreateJob_SandboxFields` — create with sandbox fields, get, verify all fields
  - `TestUpdateJob_SandboxFields` — update sandbox fields, verify version snapshot preserves them
  - `TestCreateJob_DefaultExecutionMode` — create without execution_mode, verify default `http`
  - `TestCreateWorkflowStep_CompensateStepRef` — create step with compensation, get, verify
  - `TestSnapshotWorkflowVersion_CompensateStepRef` — snapshot version, list version steps, verify
  - `TestCreateWorkflowRun_CompensationDefaults` — create run, verify `compensation_status=none`, totals=0
  - `TestUpdateWorkflowRunStatus_CompensationFields` — update with compensation fields, verify persisted
  - `TestListJobsByTag_SandboxFields` — list by tag, verify sandbox fields populated
  - `TestGetJobBySlug_SandboxFields` — get by slug, verify sandbox fields

#### 8.2 Queue integration tests
- **File**: `internal/queue/queue_integration_test.go` — add:
  - `TestEnqueueDequeue_SandboxJob` — enqueue a run for a sandbox job, dequeue, verify `ExecutionMode` on the joined job

**Commit**: `test(store): integration tests for sandbox columns, compensation status, version snapshots`

---

### Phase 9: E2E Tests (Full API Stack)
**Goal**: End-to-end tests exercising the full cancel + compensate flow through the HTTP API.

#### 9.1 Sandbox E2E
- **File**: `internal/e2e/sandbox_e2e_test.go` (new):
  - `TestE2E_CreateSandboxJob` — POST job with sandbox mode, GET job, verify fields
  - `TestE2E_UpdateSandboxJob` — PATCH job to change sandbox code, verify version created
  - `TestE2E_CreateSandboxJob_ValidationErrors` — missing code/language, verify 400

#### 9.2 Compensation E2E
- **File**: `internal/e2e/compensation_e2e_test.go` (new):
  - `TestE2E_CancelWorkflowRun_WithCompensation` — full flow:
    1. Create 2 jobs (allocate + release)
    2. Create workflow with step1 (allocate, compensate_step_ref=release) and step2
    3. Trigger workflow
    4. Wait for step1 to complete
    5. Cancel workflow run
    6. Verify compensation step run created for `release`
    7. Verify `compensation_status` = `completed` or `running`
    8. Verify `compensation_steps_total` and `compensation_steps_completed`
  - `TestE2E_CancelWorkflowRun_NoCompensation` — cancel a workflow with no compensation steps
  - `TestE2E_RetryCompensation` — trigger compensation, force-fail it, retry
  - `TestE2E_CancelWorkflowRun_AlreadyTerminal` — verify 400

**Commit**: `test(e2e): sandbox job CRUD and workflow cancel + compensation E2E tests`

---

### Phase 10: Cross-Service Integration Tests (Strait ↔ Forge)
**Goal**: Verify the full gRPC round-trip between Go and Elixir services.

#### 10.1 Docker Compose test infrastructure
- **File**: `tests/integration/docker-compose.test.yml` (new)
- Minimal compose: postgres + forge (no strait — tests drive the API directly)
- Forge built from `apps/forge/Dockerfile`

#### 10.2 Go integration test against real Forge
- **File**: `internal/sandbox/integration_test.go` (new, `//go:build integration`):
  - Uses testcontainers-go to start Forge container
  - `TestIntegration_Execute_PythonSuccess` — run `print("hello")`, verify result
  - `TestIntegration_Execute_PythonError` — run `raise Exception("fail")`, verify error
  - `TestIntegration_Execute_PythonTimeout` — run `time.sleep(60)` with 2s timeout
  - `TestIntegration_Execute_StreamEvents` — run multi-line print, verify log events received
  - `TestIntegration_Execute_PayloadPassthrough` — set payload, verify `FORGE_PAYLOAD` env var
  - `TestIntegration_Execute_EnvVars` — set custom env vars, verify available in Python
  - `TestIntegration_Execute_ContextCancel` — cancel Go context mid-execution, verify stream ends
  - `TestIntegration_Execute_UnsupportedLanguage` — language=`ruby`, verify error
  - `TestIntegration_Execute_ConcurrentExecutions` — 5 parallel executions, all succeed
  - `TestIntegration_Execute_LargeOutput` — 10K lines of output, verify all streamed

#### 10.3 Full round-trip test (API → Executor → Forge → Result)
- **File**: `internal/e2e/sandbox_roundtrip_e2e_test.go` (new, `//go:build integration`):
  - Requires postgres + forge containers
  - `TestE2E_SandboxJobExecution` — full flow:
    1. Create sandbox job via API
    2. Trigger the job
    3. Poll run status until completed
    4. Verify run result contains Python output
    5. Verify run events contain sandbox_log entries

**Commit**: `test(integration): Strait ↔ Forge gRPC round-trip tests with testcontainers`

---

### Phase 11: Observability
**Goal**: Add OTel tracing to sandbox and compensation code paths.

#### 11.1 Add spans to compensation engine
- **File**: `internal/workflow/compensation.go`
- Spans: `compensation.CancelWorkflowRun`, `compensation.RunCompensation`, `compensation.CancelActiveSteps`, `compensation.RetryFailed`
- Attributes: `workflow_run_id`, `steps_total`, `steps_compensated`, `steps_failed`

#### 11.2 Add spans to cancel dispatch
- **File**: `internal/worker/executor_cancel.go`
- Span: `executor.DispatchCancel`
- Attributes: `job_id`, `run_id`, `cancel_endpoint_url`, `response_status`

#### 11.3 Tests
- Verify spans are created (check span names in test exporter)
- **File**: `internal/workflow/compensation_test.go` — add test verifying OTel span created
- **File**: `internal/worker/executor_cancel_test.go` — add test verifying span created

**Commit**: `feat(telemetry): add OTel spans to compensation engine and cancel dispatch`

---

### Phase 12: OpenAPI Spec Update
**Goal**: Document all new fields and endpoints in the OpenAPI spec.

#### 12.1 Update openapi.yaml
- **File**: `internal/api/openapi.yaml`
- Add to Job schema: `execution_mode`, `sandbox_code`, `sandbox_language`, `cancel_endpoint_url`
- Add to WorkflowStep schema: `compensate_step_ref`
- Add to WorkflowRun schema: `compensation_status`, `compensation_steps_total`, `compensation_steps_completed`
- Add endpoint: `POST /v1/workflow-runs/{id}/compensate`
- Update `DELETE /v1/workflow-runs/{id}` response to include `compensation_result`
- Add `CompensationResult` schema

#### 12.2 Tests
- Existing OpenAPI spec test validates the spec can be served — verify it still passes
- Manual: load spec in Scalar and verify rendering

**Commit**: `docs(api): update OpenAPI spec with sandbox, compensation, and cancel fields`

---

### Phase 13: CI Hardening
**Goal**: Ensure CI catches regressions in all new code.

#### 13.1 Add integration test job to CI
- **File**: `.github/workflows/test.yml`
- New job: `integration-sandbox` — runs `go test -tags=integration ./internal/sandbox/...` with Forge testcontainer
- Requires Docker (already available on ubuntu-latest)

#### 13.2 Fix Forge coverage threshold
- **File**: `apps/forge/mix.exs`
- After Phase 4 tests, Forge should be ≥ 90% — threshold stays at 90%
- CI `mix test --cover` will enforce this

#### 13.3 Add coverage gates
- Go packages below 50% should warn (already configured)
- New packages (`sandbox`, `workflow`, `worker`) should maintain ≥ 80%

**Commit**: `ci: add sandbox integration test job, enforce Forge coverage threshold`

---

## Execution Order & Dependencies

```
Phase 1 (data fixes) ──┐
                        ├── Phase 2 (cancel dispatch) ── Phase 11 (observability)
Phase 3 (reconnect) ───┤
                        ├── Phase 5 (sandbox tests) ──── Phase 10 (cross-service)
Phase 4 (forge) ────────┤
                        ├── Phase 6 (compensation tests)
                        ├── Phase 7 (api tests) ──────── Phase 9 (e2e tests)
                        └── Phase 8 (store integration)
                        
Phase 12 (openapi) ── independent
Phase 13 (ci) ──────── depends on all test phases
```

**Critical path**: Phase 1 → Phase 2 → Phase 9 → Phase 10 → Phase 13

Phases 3, 4, 5, 6, 7, 8, 11, 12 can be parallelized within their dependency groups.

---

## Test Count Estimates

| Phase | New Tests (Go) | New Tests (Elixir) | Total |
|-------|---------------:|-------------------:|------:|
| 1 | 3 | 0 | 3 |
| 2 | 14 | 0 | 14 |
| 3 | 6 | 0 | 6 |
| 4 | 0 | 12 | 12 |
| 5 | 21 | 0 | 21 |
| 6 | 22 | 0 | 22 |
| 7 | 15 | 0 | 15 |
| 8 | 10 | 0 | 10 |
| 9 | 6 | 0 | 6 |
| 10 | 11 | 0 | 11 |
| 11 | 2 | 0 | 2 |
| 12 | 0 | 0 | 0 |
| 13 | 0 | 0 | 0 |
| **Total** | **110** | **12** | **122** |

**Post-implementation totals**: ~1,578 Go tests + 18 Elixir tests

---

## Coverage Targets

| Package | Current | Target |
|---------|---------|--------|
| `internal/sandbox` | 64.4% | ≥ 90% |
| `internal/sandbox/v1` | 34.8% | N/A (generated) |
| `internal/workflow` | 81.4% | ≥ 92% |
| `internal/worker` | 86.6% | ≥ 90% |
| `internal/api` | 72.8% | ≥ 80% |
| `internal/store` | 7.7% | ≥ 15% (unit) + integration |
| **Forge (Elixir)** | 66.7% | ≥ 90% |

---

## Files Changed Per Phase

| Phase | New Files | Modified Files |
|-------|-----------|----------------|
| 1 | 2 (migration up/down) | 1 (jobs.go) |
| 2 | 2 (executor_cancel.go, executor_cancel_test.go) | 3 (executor_dispatch.go, compensation.go, compensation_test.go) |
| 3 | 0 | 2 (client.go, client_test.go, services.go) |
| 4 | 1 (mock_stream.ex or test helper) | 3 (runner.ex, sandbox_server.ex, tests) |
| 5 | 1 (testserver_test.go) | 2 (client_test.go, executor_sandbox_test.go) |
| 6 | 0 | 1 (compensation_test.go) |
| 7 | 1 (jobs_sandbox_test.go) | 1 (workflow_handler_test.go) |
| 8 | 0 | 2 (store_integration_test.go, queue_integration_test.go) |
| 9 | 2 (sandbox_e2e_test.go, compensation_e2e_test.go) | 0 |
| 10 | 2 (integration_test.go, sandbox_roundtrip_e2e_test.go) | 0 |
| 11 | 0 | 3 (compensation.go, executor_cancel.go, tests) |
| 12 | 0 | 1 (openapi.yaml) |
| 13 | 0 | 1 (test.yml) |
| **Total** | **11** | **~20** |

---

## Risk Mitigations

1. **Phase 10 requires Docker in CI** — testcontainers-go handles this; ubuntu-latest has Docker
2. **Forge Dockerfile may not build in CI** — needs Elixir + Python; verify `apps/forge/Dockerfile` includes Python runtime
3. **job_versions migration (Phase 1)** — existing rows won't have sandbox columns; use `DEFAULT` values in migration
4. **Compensation engine transaction boundaries** — noted as future work (P3); current implementation is best-effort with status tracking
5. **Proto changes** — any proto changes need `generate.sh` + `check.sh` to pass; CI proto-check job covers this
