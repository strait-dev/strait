# @strait/go

Go SDK for the Strait platform API with full feature parity to `@strait/ts`.

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

## Configuration from Environment

```go
client, err := strait.NewClientFromEnv()
// Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
```

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

## Packages

| Package | Description |
|---------|-------------|
| `strait` | Client, config, errors, HTTP, middleware |
| `authoring` | DefineJob, DefineWorkflow, steps, DAG validation |
| `composition` | Result, retry, wait, paginate, deployments |
| `fsm` | Run, workflow, step state machines |
| `operations` | Domain services for all API endpoints |
