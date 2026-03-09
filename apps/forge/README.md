# Forge

Secure sandboxed code execution service for [Strait](../strait/).

Forge receives code execution requests from the Strait orchestrator over gRPC, runs the code in isolated BEAM processes with resource limits, and streams execution events (logs, checkpoints, results) back.

## Architecture

```
Strait (Go)                    Forge (Elixir)
    │                              │
    │── Execute(code, limits) ────▶│
    │                              ├── Sandbox.Supervisor
    │◀── stream LogEntry ──────────│      └── Sandbox.Runner (supervised)
    │◀── stream LogEntry ──────────│             ├── OS process (python3)
    │◀── stream Checkpoint ────────│             ├── timeout enforcement
    │◀── stream ExecutionResult ───│             └── resource limits
    │                              │
    │── cancel context ───────────▶│ → kills runner process
```

### Key modules

| Module | Purpose |
|--------|---------|
| `Forge.Application` | OTP application, starts gRPC server + sandbox supervisor |
| `Forge.GRPC.SandboxServer` | gRPC service implementation (`SandboxExecutor.Execute`) |
| `Forge.Sandbox` | Public API — starts supervised execution and waits for completion |
| `Forge.Sandbox.Supervisor` | DynamicSupervisor for runner processes |
| `Forge.Sandbox.Runner` | GenServer that spawns OS process, streams events, enforces limits |

## Supported Languages

| Language | Runtime | Status |
|----------|---------|--------|
| Python | `python3` | ✅ Supported |
| JavaScript | — | 🔜 Planned |

## Configuration

| Env var | Default | Description |
|---------|---------|-------------|
| `GRPC_PORT` | `50051` | Port the gRPC server listens on |
| `MAX_SANDBOXES` | `50` | Max concurrent sandbox executions |

## Development

### Prerequisites

- Elixir 1.17+ / OTP 27+
- Python 3 (for sandbox execution)

### Setup

```bash
mix deps.get
mix compile
```

### Run

```bash
mix run --no-halt
```

### Test

```bash
mix test
```

### Lint

```bash
mix credo --strict
```

## Resource Limits

Each sandbox execution runs with configurable limits:

- **Timeout**: Maximum execution time (enforced via `Process.send_after`)
- **Memory**: Max memory for the OS process (planned — currently BEAM process level)
- **Network**: Whether outbound network access is allowed (planned — currently no restriction)

## gRPC API

Defined in [`packages/proto/sandbox/v1/sandbox.proto`](../../packages/proto/sandbox/v1/sandbox.proto).

### `Execute` RPC

Server-streaming RPC. Sends an `ExecuteRequest`, receives a stream of `ExecutionEvent` messages:

- `LogEntry` — stdout/stderr lines from the sandbox
- `Checkpoint` — intermediate state snapshots
- `ToolCall` — external tool invocations made by the code
- `ExecutionResult` — terminal event with success/failure and output

### Cancellation

Canceling the gRPC context triggers immediate sandbox termination. The BEAM process is killed and resources are cleaned up by the supervisor.
