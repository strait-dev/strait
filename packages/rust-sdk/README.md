# strait

Rust SDK for the Strait platform API with full feature parity to `@strait/ts`.

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

## FSM State Machines

```rust
use strait::fsm;

fsm::can_transition_run(fsm::RunState::Executing, fsm::RunEvent::Complete);  // true
fsm::is_terminal_run_status(fsm::RunState::Completed);                        // true
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
    Err(StraitError::NotFound(e)) => println!("Not found: {}", e.message),
    Err(StraitError::Unauthorized(e)) => println!("Auth error: {}", e.message),
    Err(StraitError::RateLimited(e)) => println!("Rate limited: {}", e.message),
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

## Development

```bash
cargo test
```
