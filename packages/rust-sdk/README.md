# strait

Rust SDK for the Strait platform API with full feature parity across all five Strait SDKs.

## Install

Add to your `Cargo.toml`:

```toml
[dependencies]
strait = "0.1"
tokio = { version = "1", features = ["full"] }
```

## Quick Start

```rust
use strait::Client;
use strait::operations::Jobs;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let client = Client::builder()
        .base_url("https://api.strait.dev")
        .bearer_token("sk_live_...")
        .build()?;

    let jobs = Jobs::new(&client);

    let run = jobs.trigger("job_abc", serde_json::json!({
        "payload": { "sku": "ABC-123" },
    })).await?;

    println!("Run ID: {}", run["id"]);
    Ok(())
}
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

```rust
// Reads strait.json from working directory + STRAIT_API_KEY from env
let client = Client::from_file(None).await?;

// Or specify a custom directory
let client = Client::from_file(Some(
    strait::ConfigFileOptions::builder()
        .dir("/path/to/project")
        .build()
)).await?;

// Or an explicit file path
let client = Client::from_file(Some(
    strait::ConfigFileOptions::builder()
        .path("/path/to/custom-config.json")
        .build()
)).await?;

// Apply additional client options on top
let client = Client::from_file_with(None, |b| {
    b.middleware(my_middleware)
}).await?;
```

The SDK reads the `sdk` section from the file. Auth tokens are **never** read from the file — they always come from the `STRAIT_API_KEY` environment variable.

You can also read just the config without creating a client:

```rust
let cfg = strait::Config::from_file()?;
```

Or extract the project ID:

```rust
let project_id = strait::Config::project_id_from_file()?;
```

### From environment variables

```rust
let client = Client::from_env()?;
// Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
```

### Inline

```rust
let client = Client::builder()
    .base_url("https://api.strait.dev")
    .api_key("sk_live_...")
    .timeout_ms(5000)
    .build()?;
```

### Environment variable override precedence

Environment variables always take precedence over `strait.json` values:

| `strait.json` field | Env var | Wins |
|---|---|---|
| `sdk.base_url` | `STRAIT_BASE_URL` | env var |
| `sdk.auth_type` | `STRAIT_AUTH_TYPE` | env var |
| `sdk.timeout_ms` | `STRAIT_TIMEOUT_MS` | env var |
| *(not in file)* | `STRAIT_API_KEY` | env var (only source) |

## Client options

| Option | Description |
|---|---|
| `.base_url(url)` | API base URL (trailing slashes stripped) |
| `.bearer_token(token)` | Bearer token auth |
| `.api_key(key)` | API key auth |
| `.run_token(token)` | Run token auth |
| `.auth(auth)` | Set auth mode directly |
| `.default_headers(h)` | Headers sent with every request |
| `.timeout_ms(ms)` | Timeout in milliseconds (default: 30000) |
| `.http_client(client)` | Custom `reqwest::Client` |
| `.middleware(mw)` | Request/response/error hooks |

## Authoring DSL

```rust
use strait::authoring::{self, JobOptions, RunContext};
use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
struct Payload {
    sku: String,
}

let job = authoring::define_job(JobOptions {
    name: "Sync Inventory",
    slug: "sync-inventory",
    endpoint_url: "https://worker.dev/jobs/sync",
    project_id: "proj_1",
    run: |payload: Payload, ctx: RunContext| async move {
        sync_inventory(&payload.sku).await
    },
})?;

// Register and trigger
job.register(&client, None).await?;
job.trigger(&client, Payload { sku: "ABC-123".into() }).await?;
```

### AI step builder

Use `ai_step()` to create steps with AI-optimised defaults (600s timeout, 5 retries, exponential backoff with 2s–120s delays, large resource class):

```rust
use strait::authoring::{ai_step, job_step, BaseStepOptions};

let steps = vec![
    job_step("extract", "job_extract", BaseStepOptions::default()),
    ai_step("summarize", "job_summarize", BaseStepOptions {
        depends_on: Some(vec!["extract".into()]),
        ..Default::default()
    }),
];
```

### Durable AI agents

Define long-running agents with built-in cost tracking and durable execution:

```rust
use strait::authoring::{define_agent, AgentOptions, AgentRunContext};

let agent = define_agent(AgentOptions {
    name: "Research Agent".into(),
    slug: "research-agent".into(),
    endpoint_url: "https://worker.dev/agents/research".into(),
    project_id: Some("proj_1".into()),
    max_cost_microusd: Some(5_000_000),
    ..Default::default()
});
```

`AgentRunContext` exposes `iteration()`, `accumulated_cost_microusd()`, and `is_budget_exceeded()` for fine-grained control inside the agent loop. Agents are tagged `strait.kind:agent` by default and use 600s timeout, 5 attempts, and exponential retry.

### Event definitions

Define typed events and parse incoming payloads:

```rust
use strait::authoring::{define_event, EventDefinition};

let event = define_event("approval.granted", None);
let parsed = event.parse(serde_json::json!({"approved": true})).unwrap();
```

### Extended RunContext

`RunContext` exposes the full set of durable-execution callbacks:

| Field | Description |
|---|---|
| `checkpoint` | Persist intermediate state |
| `report_progress` | Report progress percentage |
| `heartbeat` | Keep the run alive |
| `report_usage` | Report resource usage |
| `log_tool_call` | Log a tool invocation |
| `save_output` | Save step output |
| `state.get` | Read a state key |
| `state.set` | Write a state key |
| `state.delete` | Remove a state key |
| `state.list` | List all state keys |
| `stream_chunk` | Stream a chunk to listeners |
| `wait_for_event` | Pause until an event fires |
| `spawn` | Spawn a child run |
| `continue_run` | Continue a paused run |
| `annotate` | Add metadata annotations |
| `complete` | Mark the run as completed |
| `fail` | Mark the run as failed |

Create a context wired to a live client:

```rust
use strait::authoring::run_context_client::create_run_context;

let ctx = create_run_context(client.clone(), run_id, 1);
```

### Test harness

`create_test_context` returns an in-memory context and a shared recording so you can assert against every callback your handler invokes:

```rust
use strait::authoring::test_helpers::create_test_context;

let (ctx, record) = create_test_context("test-run", 1);
// run your handler with ctx
assert_eq!(record.lock().unwrap().checkpoints.len(), 1);
assert!(record.lock().unwrap().completed);
```

## Workflow DAG

```rust
use strait::authoring::{self, WorkflowOptions, Step};

let wf = authoring::define_workflow(WorkflowOptions {
    name: "Order Pipeline",
    slug: "order-pipeline",
    project_id: "proj_1",
    steps: vec![
        Step::job("validate", "job_validate"),
        Step::job("charge", "job_charge").depends_on(&["validate"]),
        Step::approval("review").depends_on(&["charge"]),
    ],
})?;
```

## Composition Helpers

```rust
use strait::composition;

// Retry with backoff
let result = composition::with_retry(
    || async { call_api().await },
    composition::RetryOptions { attempts: 5, delay_ms: 100 },
).await?;

// Paginate
let mut stream = composition::paginate(|cursor| async move {
    list_fn(cursor).await
});
while let Some(item) = stream.next().await {
    // process item
}

// Wait for run
let run = composition::wait_for_run(
    |id| async move { get_run(id).await },
    |run| get_status(run),
    "run_123",
    None,
).await?;
```

### Cost budget

Track accumulated costs and abort when a micro-USD budget is exceeded:

```rust
use strait::composition::{CostTracker, with_cost_budget};

let tracker = CostTracker::new(5_000_000); // $5.00 budget
tracker.add(120_000);
assert!(!tracker.is_exceeded());

// Wrap an async block so it short-circuits on budget breach
let result = with_cost_budget(tracker.clone(), || async {
    expensive_call().await
}).await?;
```

Exceeding the budget returns `StraitError::CostBudgetExceeded`.

### Checkpoint resume

Resume a composition from its last checkpoint instead of replaying from scratch:

```rust
use strait::composition::with_checkpoint_resume;

let result = with_checkpoint_resume(
    ctx.clone(),
    || async { long_running_pipeline(ctx.clone()).await },
).await?;
```

## FSM State Machines

```rust
use strait::fsm;

fsm::can_transition_run(fsm::RunStatus::Executing, fsm::RunEvent::Complete);  // true
fsm::is_terminal_run_status(fsm::RunStatus::Completed);                        // true
```

## Middleware

```rust
let client = Client::builder()
    .base_url("https://api.strait.dev")
    .bearer_token("sk_live_...")
    .middleware(strait::Middleware {
        on_request: Some(Box::new(|ctx| {
            println!("{} {}", ctx.method, ctx.url);
        })),
        on_response: Some(Box::new(|ctx| {
            println!("{} {}ms", ctx.status, ctx.duration_ms);
        })),
        on_error: Some(Box::new(|ctx| {
            eprintln!("error: {}", ctx.error);
        })),
    })
    .build()?;
```

## Custom HTTP Client

Pass a custom `reqwest::Client` to the builder:

```rust
let http_client = reqwest::Client::builder()
    .pool_max_idle_per_host(10)
    .connect_timeout(std::time::Duration::from_secs(5))
    .build()?;

let client = Client::builder()
    .base_url("https://api.strait.dev")
    .api_key("sk_live_...")
    .http_client(http_client)
    .build()?;
```

## Error Handling

All errors are typed enums. Use `match` to handle specific error kinds:

```rust
use strait::error::StraitError;

match jobs.get("nonexistent").await {
    Ok(result) => println!("Found: {:?}", result),
    Err(StraitError::NotFound { message, .. }) => println!("Not found: {}", message),
    Err(StraitError::Unauthorized { message, .. }) => println!("Auth error: {}", message),
    Err(StraitError::RateLimited { message, .. }) => println!("Rate limited: {}", message),
    Err(e) => println!("Error: {}", e),
}
```

| Error variant | HTTP status | Description |
|---|---|---|
| `StraitError::Transport` | — | Network/transport failure |
| `StraitError::Decode` | — | JSON decode failure |
| `StraitError::Validation` | — | Config or input validation |
| `StraitError::Unauthorized` | 401, 403 | Authentication failure |
| `StraitError::NotFound` | 404 | Resource not found |
| `StraitError::Conflict` | 409 | Conflict (duplicate, etc.) |
| `StraitError::RateLimited` | 429 | Rate limit exceeded |
| `StraitError::Api` | other | Generic HTTP error |
| `StraitError::Timeout` | — | Polling timeout |
| `StraitError::DagValidation` | — | Workflow DAG is invalid |
| `StraitError::CostBudgetExceeded` | — | Cost budget exceeded |

## Development

```bash
cargo test
```
