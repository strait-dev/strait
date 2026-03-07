# Workflows

Workflows define a DAG (directed acyclic graph) of steps. Each step executes a job, awaits human approval, or triggers a sub-workflow. See [API Reference](api-reference.md) for workflow endpoints.

### Features

- Directed acyclic graph workflows with Kahn's algorithm validation
- Fan-in with atomic dependency counter (Postgres row-level lock serializes concurrent updates)
- Fan-out: multiple steps can depend on the same parent step
- Template variable substitution: `{{variable_name}}` placeholders in step payloads, resolved from trigger payload with dot-notation support
- Step output transformation via JSONPath (`output_transform` field) to extract specific values from step results
- Workflow run labels: key-value labels on workflow runs for filtering and organization
- Step overrides at trigger time: disable specific steps and override timeout/retry configuration per step when triggering
- Sub-workflow support: steps can reference another workflow (step type `sub_workflow`) with nesting depth limits
- Three step types: `job` (execute a job), `approval` (human gate with approver list), `sub_workflow` (nested workflow DAG)
- Workflow versioning with automatic step snapshot on update
- Workflow cloning
- Workflow pause/resume
- Step-level retry configuration (max attempts, backoff policy, initial/max delay)
- Workflow run retry from first failed step
- Per-step approval gates with configurable approvers and timeout
- Skip and force-complete operations for pending/waiting steps
- Workflow-level concurrency control (MaxConcurrentRuns) and per-run parallel step limits (MaxParallelSteps)
- Cron-triggered workflows with SkipIfRunning support
- Workflow run retention cleanup via reaper
- Three failure policies per step: `fail_workflow`, `skip_dependents`, `continue`
- Step conditions: `step_status`, `all_of`, `any_of` (supports nesting)
- Payload merging: trigger payload + step-level payload + parent step outputs (keyed by `step_ref`)
- 7-state step FSM: `pending`, `waiting`, `running`, `completed`, `failed`, `skipped`, `canceled`
- Event-driven step progression — hooks into all 7 terminal code paths (executor, SDK, cancel, reaper)

### Concepts

- **Workflow**: A named DAG definition with one or more steps.
- **Step**: A node in the DAG. References a job by `job_id` and is identified by a unique `step_ref`.
- **Dependencies**: A step lists its parent steps in `depends_on`. Root steps (no dependencies) start immediately.
- **Workflow Run**: An execution instance of a workflow. Contains step runs for each step.
- **Step Run**: Tracks the execution of a single step within a workflow run.

### Step Dependencies and Fan-In

When a step has multiple dependencies, it uses an atomic counter to track completions. Each parent step that completes atomically increments the counter via a single `UPDATE ... RETURNING` query. The step only starts when all dependencies are met. Postgres row-level locks serialize concurrent updates, preventing race conditions.

### Failure Policies

Each step can specify an `on_failure` policy:

| Policy | Behavior |
|--------|----------|
| `fail_workflow` | Fail the entire workflow and cancel remaining steps (default) |
| `skip_dependents` | Skip all downstream steps, but let other branches continue |
| `continue` | Treat the failure as a success for dependency purposes |

### Step Conditions

Steps can have conditions that control whether they run when dependencies are met:

```json
{"type": "step_status", "step_ref": "extract", "status": "completed"}
```

```json
{"type": "all_of", "conditions": [
  {"type": "step_status", "step_ref": "a", "status": "completed"},
  {"type": "step_status", "step_ref": "b", "status": "completed"}
]}
```

```json
{"type": "any_of", "conditions": [
  {"type": "step_status", "step_ref": "primary", "status": "completed"},
  {"type": "step_status", "step_ref": "fallback", "status": "completed"}
]}
```

If a condition evaluates to false, the step is skipped.

### Payload Flow

When a step starts, its payload is constructed by merging three sources (later sources override earlier):

1. **Trigger payload**: The payload provided when triggering the workflow
2. **Step payload**: Static payload defined on the step
3. **Parent outputs**: A `parent_outputs` key containing the result of each parent step, keyed by `step_ref`

### Template Variables
Steps can use `{{variable_name}}` placeholders in their payload that are resolved from the trigger payload. Dot-notation is supported (e.g., `{{user.email}}`). Type preservation: if the entire value is a single `{{var}}`, the native type is preserved; if embedded in text like `"Hello {{name}}"`, it becomes a string.

### Output Transformation
Steps can define an `output_transform` field with a JSONPath expression (using gjson syntax) to extract specific values from step results. For example, `"result.data.items"` extracts a nested array from the step's raw output. The transformation is applied before the output is persisted and before it's passed to dependent steps via `parent_outputs`.

### Step Overrides
When triggering a workflow, you can pass `step_overrides` to customize individual steps:
```json
{
  "payload": {...},
  "step_overrides": {
    "expensive-step": {"disabled": true},
    "fast-step": {"timeout_secs": 10, "max_attempts": 5}
  }
}
```
Disabled steps are removed from the DAG and their dependencies are pruned from downstream steps.

### Step Types
Three step types are supported:
- `job`: Executes a job (default). References a job via `job_id`.
- `approval`: Human gate. Pauses execution until approved via API. Configurable `approval_timeout_secs` and list of `approval_approvers`.
- `sub_workflow`: Nested DAG. References another workflow via `sub_workflow_id`. Nesting depth is validated (default max 10). Child workflow outputs are aggregated as the parent step's output.

### Workflow Versioning
Workflow version is auto-incremented on update. Step definitions are snapshotted per version, so running workflow runs reference the step configuration at the time they were triggered.

### Run Labels
Workflow runs support key-value labels for filtering and organization. Labels can be set at trigger time.

### Retry
Steps support independent retry configuration:
- `retry_max_attempts`: Maximum retry attempts per step
- `retry_backoff`: Policy (`exponential` or `fixed`)
- `retry_initial_delay_secs`: Base delay between retries
- `retry_max_delay_secs`: Maximum delay cap

Workflow runs can also be retried from the first failed step via `POST /v1/workflow-runs/{id}/retry`, which creates a new run with completed steps pre-populated.
