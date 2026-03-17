# @strait/go

Go SDK for the Strait platform API with full feature parity across all five Strait SDKs.

## Install

```bash
go get github.com/strait-dev/go-sdk
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    strait "github.com/strait-dev/go-sdk"
    "github.com/strait-dev/go-sdk/operations"
)

func main() {
    client := strait.NewClient(
        strait.WithBaseURL("https://api.strait.dev"),
        strait.WithBearerToken("sk_live_..."),
    )

    jobs := operations.NewJobsService(client)

    result, err := jobs.Trigger(context.Background(), "job_abc", map[string]any{
        "payload": map[string]any{"sku": "ABC-123"},
    })
    if err != nil {
        panic(err)
    }
    fmt.Println("Run ID:", result["id"])
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

```go
// Reads strait.json from working directory + STRAIT_API_KEY from env
client, err := strait.NewClientFromFile(nil)

// Or specify a custom directory
client, err := strait.NewClientFromFile([]strait.ConfigFileOption{
    strait.WithConfigDir("/path/to/project"),
})

// Or an explicit file path
client, err := strait.NewClientFromFile([]strait.ConfigFileOption{
    strait.WithConfigPath("/path/to/custom-config.json"),
})

// Apply additional client options on top
client, err := strait.NewClientFromFile(nil,
    strait.WithMiddleware(myMiddleware),
)
```

The SDK reads the `sdk` section from the file. Auth tokens are **never** read from the file — they always come from the `STRAIT_API_KEY` environment variable.

You can also read just the config without creating a client:

```go
cfg, err := strait.ConfigFromFile()
```

Or extract the project ID:

```go
projectID, err := strait.ProjectIDFromFile()
```

### From environment variables

```go
client, err := strait.NewClientFromEnv()
// Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
```

### Inline

```go
client := strait.NewClient(
    strait.WithBaseURL("https://api.strait.dev"),
    strait.WithAPIKey("sk_live_..."),
    strait.WithTimeout(5000),
)
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
| `WithBaseURL(url)` | API base URL (trailing slashes stripped) |
| `WithBearerToken(token)` | Bearer token auth |
| `WithAPIKey(key)` | API key auth |
| `WithRunToken(token)` | Run token auth |
| `WithAuth(auth)` | Set auth mode directly |
| `WithDefaultHeaders(h)` | Headers sent with every request |
| `WithTimeout(ms)` | Timeout in milliseconds (default: 30000) |
| `WithHTTPClient(doer)` | Custom `HTTPDoer` implementation |
| `WithMiddleware(mw...)` | Request/response/error hooks |

## Authoring DSL

```go
import "github.com/strait-dev/go-sdk/authoring"

job := authoring.DefineJob(authoring.JobOptions[MyPayload]{
    Name:        "Sync Inventory",
    Slug:        "sync-inventory",
    EndpointURL: "https://worker.dev/jobs/sync",
    ProjectID:   "proj_1",
    Run: func(p MyPayload, ctx authoring.RunContext) (any, error) {
        return syncInventory(p.SKU)
    },
})

// Register and trigger
job.Register(ctx, client, "")
job.Trigger(ctx, client, authoring.TriggerJobInput[MyPayload]{
    Payload: MyPayload{SKU: "ABC-123"},
})
```

## Workflow DAG

```go
wf := authoring.DefineWorkflow(authoring.WorkflowOptions[MyPayload]{
    Name:      "Order Pipeline",
    Slug:      "order-pipeline",
    ProjectID: "proj_1",
    Steps: []authoring.Step{
        authoring.Job("validate", "job_validate"),
        authoring.Job("charge", "job_charge", authoring.DependsOn("validate")),
        authoring.Approval("review", func(a *authoring.ApprovalStep) {
            a.DependsOn = []string{"charge"}
        }),
    },
})
```

### AI step builder

Use `AI()` to add an AI-powered step to a workflow DAG. It applies sensible defaults for long-running AI work: 600 s timeout, 5 retries with exponential backoff, and the `large` resource class.

```go
steps := []authoring.Step{
    authoring.Job("extract", "job_extract"),
    authoring.AI("summarize", "job_summarize", func(o *authoring.BaseStepOptions) {
        o.DependsOn = []string{"extract"}
    }),
}
```

### Durable AI agents

`DefineAgent()` creates a durable, cost-aware agent that can run across multiple iterations and survive restarts.

```go
agent := authoring.DefineAgent(authoring.AgentOptions{
    Name:            "Research Agent",
    Slug:            "research-agent",
    EndpointURL:     "https://worker.dev/agents/research",
    MaxCostMicrousd: intPtr(5_000_000),
    Run: func(payload any, ctx *authoring.AgentRunContext) (any, error) {
        fmt.Println("Iteration:", ctx.Iteration())
        // ...
        return result, nil
    },
})
```

`AgentRunContext` exposes:

| Method | Description |
|---|---|
| `Iteration()` | Current iteration index |
| `AccumulatedCostMicrousd()` | Total cost so far in micro-USD |
| `IsBudgetExceeded()` | Whether the cost budget has been exceeded |

### Event definitions

`DefineEvent()` declares a named event with an optional validation function.

```go
event := authoring.DefineEvent("approval.granted", func(input any) (any, error) {
    // validate
    return input, nil
})

parsed, _ := event.Parse(rawData)
```

### Extended RunContext

`CreateRunContext()` builds a `RunContext` bound to an existing run, giving full access to the platform from outside a job handler.

```go
ctx := authoring.CreateRunContext(client.SDKRuns, runID, authoring.WithAttempt(1))
ctx.State.Set("key", value)
```

Available methods on `RunContext`:

| Method | Description |
|---|---|
| `Checkpoint(data)` | Persist a checkpoint for crash recovery |
| `ReportProgress(pct, msg)` | Report execution progress |
| `Heartbeat()` | Signal liveness to the platform |
| `ReportUsage(metrics)` | Report resource usage metrics |
| `LogToolCall(name, input, output)` | Log an external tool invocation |
| `SaveOutput(data)` | Persist step output |
| `State.Get(key)` | Read a value from run state |
| `State.Set(key, value)` | Write a value to run state |
| `State.Delete(key)` | Remove a value from run state |
| `State.List()` | List all keys in run state |
| `StreamChunk(chunk)` | Send a streaming chunk to listeners |
| `WaitForEvent(name, opts)` | Pause until a named event fires |
| `Spawn(opts)` | Spawn a child run |
| `Continue(payload)` | Continue to the next iteration |
| `Annotate(key, value)` | Attach metadata to the run |
| `Complete(output)` | Mark the run as completed |
| `Fail(err)` | Mark the run as failed |

### Test harness

`CreateTestContext()` returns an in-memory `RunContext` and a `TestRecord` that captures every side-effect, so you can unit-test handlers without a running platform.

```go
ctx, record := authoring.CreateTestContext("test-run")
// run your handler with ctx
assert.Equal(t, 1, len(record.Checkpoints))
assert.True(t, record.Completed)
```

## Composition Helpers

```go
import "github.com/strait-dev/go-sdk/composition"

// Retry with backoff
result, err := composition.WithRetry(ctx, func() (string, error) {
    return callAPI()
}, &composition.RetryOptions{Attempts: 5, DelayMs: 100})

// Paginate
for item, err := range composition.Paginate(listFn, nil) {
    // process item
}

// Wait for run
run, err := composition.WaitForRun(ctx, getRun, getStatus, "run_123", nil)
```

### Cost budget

`NewCostTracker()` tracks accumulated cost across retries and iterations. Wrap any composition call with `WithCostBudget()` to automatically abort when the budget is exceeded.

```go
tracker := composition.NewCostTracker(5_000_000) // 5 USD in micro-USD

result, err := composition.WithCostBudget(ctx, tracker, func() (string, error) {
    return callExpensiveModel()
})
if errors.As(err, new(*composition.CostBudgetExceededError)) {
    fmt.Println("Budget exceeded:", err)
}
```

### Checkpoint resume

`WithCheckpointResume()` wraps a function so that completed steps are skipped on retry, picking up from the last successful checkpoint.

```go
result, err := composition.WithCheckpointResume(ctx, runCtx, func(cp *composition.Checkpoint) (string, error) {
    cp.Save("step-1-done", intermediateResult)
    // on retry this function resumes after the last saved checkpoint
    return finalResult, nil
})
```

## FSM State Machines

```go
import "github.com/strait-dev/go-sdk/fsm"

fsm.CanTransitionRun(fsm.RunExecuting, fsm.RunEventComplete)  // true
fsm.IsTerminalRunStatus(fsm.RunCompleted)                       // true
```

## Middleware

```go
client := strait.NewClient(
    strait.WithBaseURL("https://api.strait.dev"),
    strait.WithBearerToken("sk_live_..."),
    strait.WithMiddleware(strait.Middleware{
        OnRequest:  func(ctx strait.MiddlewareRequestContext) { log.Println(ctx.Method, ctx.URL) },
        OnResponse: func(ctx strait.MiddlewareResponseContext) { log.Println(ctx.Status, ctx.DurationMs, "ms") },
    }),
)
```

## Custom HTTP Client

Any type implementing the `HTTPDoer` interface can replace the default `http.Client`:

```go
type HTTPDoer interface {
    Do(req *http.Request) (*http.Response, error)
}

client := strait.NewClient(
    strait.WithBaseURL("https://api.strait.dev"),
    strait.WithAPIKey("sk_live_..."),
    strait.WithHTTPClient(myCustomClient),
)
```

## Error Handling

All errors returned by the SDK are typed. Use `errors.As` to match specific error kinds:

```go
import "errors"

result, err := jobs.Get(ctx, "job_nonexistent")
if err != nil {
    var notFound *strait.NotFoundError
    var unauthorized *strait.UnauthorizedError
    var rateLimited *strait.RateLimitedError

    switch {
    case errors.As(err, &notFound):
        fmt.Println("Not found:", notFound.Message)
    case errors.As(err, &unauthorized):
        fmt.Println("Auth error:", unauthorized.Message)
    case errors.As(err, &rateLimited):
        fmt.Println("Rate limited:", rateLimited.Message)
    default:
        fmt.Println("Error:", err)
    }
}
```

| Error type | HTTP status | Description |
|---|---|---|
| `*TransportError` | — | Network/transport failure |
| `*DecodeError` | — | JSON decode failure |
| `*ValidationError` | — | Config or input validation |
| `*UnauthorizedError` | 401, 403 | Authentication failure |
| `*NotFoundError` | 404 | Resource not found |
| `*ConflictError` | 409 | Conflict (duplicate, etc.) |
| `*RateLimitedError` | 429 | Rate limit exceeded |
| `*ApiError` | other | Generic HTTP error |
| `*TimeoutError` | — | Polling timeout |
| `*DagValidationError` | — | Workflow DAG is invalid |
| `*CostBudgetExceededError` | — | Cost budget exceeded |

## Packages

| Package | Description |
|---------|-------------|
| `strait` | Client, config, errors, HTTP, middleware |
| `authoring` | DefineJob, DefineWorkflow, steps, DAG validation |
| `composition` | Result, retry, wait, paginate, deployments |
| `fsm` | Run, workflow, step state machines |
| `operations` | Domain services for all API endpoints |

## Development

```bash
go test ./...
```
