# @strait/python

Python SDK for the Strait API. Full feature parity with the Go and TypeScript SDKs.

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

### From Environment Variables

```python
# Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
client = Client.from_env()
```

### Async Client

```python
from strait import AsyncClient

async with AsyncClient(base_url="https://api.strait.dev", api_key="your-key") as client:
    jobs = await client.jobs.list()
```

## Features

### Domain Operations (19 Services)

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

### Authoring DSL

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

### Composition Helpers

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

### FSM State Machines

```python
from strait.fsm import transition_run, RunStatus, RunEvent, is_terminal_run_status

next_status = transition_run(RunStatus.EXECUTING, RunEvent.COMPLETE)
assert next_status == RunStatus.COMPLETED
assert is_terminal_run_status(next_status)
```

### Middleware

```python
from strait import Client, Middleware

mw = Middleware(
    on_request=lambda ctx: print(f"-> {ctx.method} {ctx.url}"),
    on_response=lambda ctx: print(f"<- {ctx.status} ({ctx.duration_ms}ms)"),
    on_error=lambda ctx: print(f"!! {ctx.error}"),
)

client = Client(base_url="...", api_key="...", middleware=[mw])
```

## Development

```bash
make bootstrap   # Create venv + install deps
make test        # Run pytest
make lint        # Run ruff
make typecheck   # Run mypy
make run-all     # lint + typecheck + test
```
