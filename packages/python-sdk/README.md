# @strait/python

Python SDK for the Strait API. Full feature parity with the TypeScript, Go, Ruby, and Rust SDKs.

## Installation

```bash
pip install strait-python
```

Requires Python 3.11+.

## Quick Start

```python
from strait import Client

client = Client(base_url="https://api.strait.dev", api_key="your-key")

# List jobs
jobs = client.jobs.list()

# Create and trigger a job
job = client.jobs.create({"name": "my-job", "project_id": "proj-1", ...})
run = client.jobs.trigger(job["id"], {"payload": {"key": "value"}})

# Get a run
run = client.runs.get(run["id"])
```

## Configuration

### From `strait.json` (recommended)

Create a `strait.json` at your project root:

```json
{
  "$schema": "https://strait.dev/schema.json",
  "project": {
    "id": "proj_abc123",
    "name": "My Project"
  },
  "sdk": {
    "base_url": "https://api.strait.dev",
    "auth_type": "apiKey",
    "timeout_ms": 30000
  }
}
```

Then load the client from it:

```python
# Reads strait.json from working directory + STRAIT_API_KEY from env
client = Client.from_file()

# Or specify a custom directory
client = Client.from_file(search_dir="/path/to/project")

# Or an explicit file path
client = Client.from_file(path="/path/to/custom-config.json")

# Apply overrides on top
client = Client.from_file(timeout_ms=5000)
```

The SDK reads the `sdk` section from the file. Auth tokens are **never** read from the file — they always come from the `STRAIT_API_KEY` environment variable.

You can also read just the config without creating a client:

```python
from strait import config_from_file

cfg = config_from_file()
cfg = config_from_file(search_dir="/path/to/project")
```

### From environment variables

```python
# Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
client = Client.from_env()
```

### Inline

```python
client = Client(
    base_url="https://api.strait.dev",
    api_key="your-key",
    timeout_ms=5000,
)
```

### Async client

All three configuration methods work with `AsyncClient` too:

```python
from strait import AsyncClient

async with AsyncClient.from_file() as client:
    jobs = await client.jobs.list()

# Or from env
async with AsyncClient.from_env() as client:
    jobs = await client.jobs.list()
```

### Environment variable override precedence

Environment variables always take precedence over `strait.json` values:

| `strait.json` field | Env var | Wins |
|---|---|---|
| `sdk.base_url` | `STRAIT_BASE_URL` | env var |
| `sdk.auth_type` | `STRAIT_AUTH_TYPE` | env var |
| `sdk.timeout_ms` | `STRAIT_TIMEOUT_MS` | env var |
| *(not in file)* | `STRAIT_API_KEY` | env var (only source) |

## Domain Operations (19 Services)

All 186 API operations organized into typed service classes:

| Service | Examples |
|---|---|
| `client.jobs` | list, create, get, update, delete, trigger, bulk_trigger |
| `client.runs` | list, get, delete, replay, bulk_cancel, get_dlq |
| `client.workflows` | list, create, trigger, get_diff, get_policy |
| `client.workflow_runs` | list, pause, resume, approve_step, skip_step |
| `client.deployments` | list, create, finalize, promote, rollback |
| `client.sdk_runs` | complete_run, heartbeat_run, checkpoint_run |
| `client.rbac` | list_roles, create_member, seed_roles |
| + 12 more | environments, secrets, api_keys, webhooks, ... |

## Authoring DSL

```python
from strait.authoring import define_job, define_workflow, JobOptions, WorkflowOptions
from strait.authoring import job_step, approval_step, sleep_step

# Define a job
job = define_job(JobOptions(
    name="Process Order",
    slug="process-order",
    endpoint_url="https://worker.example.com/run",
    project_id="proj-1",
    max_concurrency=10,
    timeout_secs=300,
))

# Define a workflow with DAG validation
wf = define_workflow(WorkflowOptions(
    name="Order Pipeline",
    slug="order-pipeline",
    project_id="proj-1",
    steps=[
        job_step("validate", "validate-job"),
        job_step("charge", "charge-job", depends_on=["validate"]),
        approval_step("approve", depends_on=["charge"]),
        job_step("ship", "ship-job", depends_on=["approve"]),
    ],
))
```

### AI step builder

`ai_step()` creates a job step with LLM-tuned defaults: 600s timeout, 5 retries with exponential backoff, and `large` resource class.

```python
from strait.authoring import ai_step

s = ai_step("summarize", "job_summarize", depends_on=["extract"])
```

All standard step options (e.g. `on_failure`, `payload`, `condition`) are still accepted — the AI defaults only apply when you omit them.

### Durable AI agents

`define_agent()` wraps `define_job()` with agent-specific conventions: automatic checkpointing, cost tracking, iteration counting, and the `strait.kind=agent` tag.

```python
from strait.authoring import define_agent, AgentOptions

async def my_handler(payload, ctx):
    while not ctx.is_budget_exceeded():
        result = await do_work(payload)
        await ctx.checkpoint({"last_result": result})
        await ctx.report_usage(
            provider="openai", model="gpt-4o",
            cost_microusd=result["cost"],
        )
    await ctx.complete({"summary": result})

agent = define_agent(AgentOptions(
    name="Research Agent",
    slug="research-agent",
    endpoint_url="https://worker.dev/agents/research",
    project_id="proj-1",
    max_cost_microusd=5_000_000,
    run=my_handler,
))
```

The handler receives an `AgentRunContext` (extends `RunContext`) with these extras:

| Attribute / Method | Description |
|---|---|
| `ctx.iteration` | Auto-incremented on every `checkpoint()` call |
| `ctx.accumulated_cost_microusd()` | Running total from `report_usage()` calls |
| `ctx.is_budget_exceeded()` | `True` when accumulated cost >= `max_cost_microusd` |

Defaults applied by `define_agent()`: `strait.kind=agent` tag, 600s timeout, 5 max attempts, exponential retry strategy.

### Event definitions

`define_event()` creates a typed, optionally validated event descriptor.

```python
from strait.authoring import define_event

def my_validator(raw):
    if "user_id" not in raw:
        raise ValueError("user_id is required")
    return raw

approval = define_event("approval.granted", validate=my_validator)
parsed = approval.parse(raw_data)
```

When no `validate` function is provided, `parse()` returns the input unchanged.

### Extended RunContext

`RunContext` is the execution context passed to every job and agent handler. It exposes 18 async methods for interacting with the Strait runtime during a run.

```python
from strait.authoring import create_run_context

ctx = create_run_context(client.sdk_runs, run_id, attempt=1)
await ctx.state.set("key", value)
```

| Method | Description |
|---|---|
| `await ctx.checkpoint(state)` | Persist intermediate state for resume |
| `await ctx.report_progress(percent, message)` | Report progress (0-100) |
| `await ctx.heartbeat()` | Keep the run alive |
| `await ctx.report_usage(provider, model, ...)` | Report LLM token/cost usage |
| `await ctx.log_tool_call(tool_name, ...)` | Log an external tool invocation |
| `await ctx.save_output(key, value, schema)` | Save a named output artifact |
| `await ctx.stream_chunk(chunk, stream_id, done)` | Stream incremental output |
| `await ctx.wait_for_event(event_key, ...)` | Pause until an external event fires |
| `await ctx.spawn(job_slug, project_id, ...)` | Spawn a child run |
| `await ctx.continue_run(payload)` | Continue the run with new payload |
| `await ctx.annotate(annotations)` | Attach key-value annotations |
| `await ctx.complete(result)` | Mark the run as completed |
| `await ctx.fail(error)` | Mark the run as failed |
| `await ctx.state.get(key)` | Read from the run's KV store |
| `await ctx.state.set(key, value)` | Write to the run's KV store |
| `await ctx.state.delete(key)` | Delete from the run's KV store |
| `await ctx.state.list()` | List all keys in the run's KV store |
| `ctx.logger` | Standard `logging.Logger` (auto-forwarded to Strait) |

### Test harness

`create_test_context()` builds an in-memory `RunContext` and a `TestRunRecord` for unit-testing handlers without any HTTP calls.

```python
from strait.authoring import create_test_context

async def test_my_handler():
    ctx, record = create_test_context("test-run")
    await my_job_handler(payload, ctx)

    assert len(record.checkpoints) == 1
    assert record.completed is True
    assert record.state_store["key"] == "expected"
```

`TestRunRecord` captures everything the handler did:

| Field | Type | Description |
|---|---|---|
| `record.checkpoints` | `list[dict]` | States passed to `checkpoint()` |
| `record.logs` | `list[dict]` | Log entries |
| `record.usage_reports` | `list[dict]` | Usage reports |
| `record.tool_calls` | `list[dict]` | Tool call logs |
| `record.outputs` | `list[dict]` | Saved outputs |
| `record.progress_updates` | `list[dict]` | Progress updates |
| `record.state_store` | `dict` | Final KV state |
| `record.stream_chunks` | `list[dict]` | Streamed chunks |
| `record.heartbeats` | `int` | Heartbeat count |
| `record.spawns` | `list[dict]` | Spawned child runs |
| `record.events` | `list[dict]` | Events waited on |
| `record.annotations` | `list[dict]` | Annotations |
| `record.continuations` | `list[dict]` | Continuations |
| `record.completed` | `bool` | Whether `complete()` was called |
| `record.failed` | `bool` | Whether `fail()` was called |
| `record.fail_error` | `str \| None` | Error message from `fail()` |
| `record.result` | `dict \| None` | Result passed to `complete()` |

## Composition Helpers

```python
from strait.composition import with_retry, wait_for_run, paginate, Result, RetryOptions

# Retry with exponential backoff
result = with_retry(lambda: client.jobs.trigger("j1", payload), RetryOptions(attempts=5))

# Wait for a run to complete
run = wait_for_run(
    get_run=lambda rid: client.runs.get(rid),
    get_status=lambda r: r["status"],
    run_id="run-1",
)

# Paginate through results
from strait.composition import PaginatedResponse
for item in paginate(lambda q: PaginatedResponse(data=client.jobs.list(query={"cursor": q.cursor})["data"])):
    print(item)
```

### Cost budget

`create_cost_tracker()` and `with_cost_budget()` enforce a spend ceiling on LLM-heavy workflows. When the budget is exceeded, a `CostBudgetExceededError` is raised.

```python
from strait.composition import create_cost_tracker, with_cost_budget, CostBudgetOptions
from strait import CostBudgetExceededError

# Manual tracker
tracker = create_cost_tracker(CostBudgetOptions(
    max_cost_microusd=1_000_000,
    warning_threshold=0.8,
    on_warning=lambda current, max_: print(f"Warning: {current}/{max_} microusd"),
))
tracker.add(500_000)
print(tracker.remaining())  # 500_000

# Scoped budget via callback
async def do_work(tracker):
    tracker.add(200_000)
    print(tracker.current())   # 200_000
    print(tracker.is_exceeded())  # False
    return "done"

result = await with_cost_budget(do_work, CostBudgetOptions(max_cost_microusd=1_000_000))
```

### Checkpoint resume

`with_checkpoint_resume()` wraps a long-running function so it can resume from the last checkpoint after a crash or restart.

```python
from strait.composition import with_checkpoint_resume
from strait.authoring import create_run_context

ctx = create_run_context(client.sdk_runs, run_id, attempt=1)

async def process(state, update):
    for i in range(state.get("next_index", 0), 100):
        await do_step(i)
        update({"next_index": i + 1})
    return "all done"

result = await with_checkpoint_resume(
    ctx,
    last_checkpoint=None,          # or previous checkpoint dict
    fn=process,
    initial_state={"next_index": 0},
    checkpoint_interval=5,         # checkpoint every 5 update() calls
)
```

## FSM State Machines

```python
from strait.fsm import transition_run, RunStatus, RunEvent, is_terminal_run_status

next_status = transition_run(RunStatus.EXECUTING, RunEvent.COMPLETE)
assert next_status == RunStatus.COMPLETED
assert is_terminal_run_status(next_status)
```

## Middleware

```python
from strait import Client, Middleware

mw = Middleware(
    on_request=lambda ctx: print(f"-> {ctx.method} {ctx.url}"),
    on_response=lambda ctx: print(f"<- {ctx.status} ({ctx.duration_ms}ms)"),
    on_error=lambda ctx: print(f"!! {ctx.error}"),
)

client = Client(base_url="...", api_key="...", middleware=[mw])
```

## Custom HTTP Client

You can inject your own `httpx.Client` or `httpx.AsyncClient`:

```python
import httpx

custom_http = httpx.Client(timeout=60.0, verify=False)
client = Client(base_url="...", api_key="...", http_client=custom_http)

# Async
custom_async = httpx.AsyncClient(timeout=60.0)
async_client = AsyncClient(base_url="...", api_key="...", http_client=custom_async)
```

## Error Handling

All errors raised by the SDK inherit from `StraitError`:

```python
from strait import Client, NotFoundError, UnauthorizedError, RateLimitedError

try:
    job = client.jobs.get("job_nonexistent")
except NotFoundError as e:
    print(f"Not found: {e} (status={e.status})")
except UnauthorizedError as e:
    print(f"Auth error: {e}")
except RateLimitedError as e:
    print(f"Rate limited: {e}")
```

| Exception | HTTP status | Description |
|---|---|---|
| `TransportError` | — | Network/transport failure |
| `DecodeError` | — | JSON decode failure |
| `ValidationError` | — | Config or input validation |
| `UnauthorizedError` | 401, 403 | Authentication failure |
| `NotFoundError` | 404 | Resource not found |
| `ConflictError` | 409 | Conflict (duplicate, etc.) |
| `RateLimitedError` | 429 | Rate limit exceeded |
| `ApiError` | other | Generic HTTP error |
| `StraitTimeoutError` | — | Polling timeout |
| `DagValidationError` | — | Workflow DAG is invalid |
| `CostBudgetExceededError` | — | Cost budget exceeded |

All exceptions expose `.status` (for HTTP errors) and `.body` (raw response body when available).

## Development

```bash
make bootstrap   # Create venv + install deps
make test        # Run pytest
make lint        # Run ruff
make typecheck   # Run mypy
make run-all     # lint + typecheck + test
```
