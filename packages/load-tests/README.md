# Load Test Jobs

Real workloads for Strait load testing. These jobs run in Docker containers and interact with the Strait SDK endpoints.

## Building

```bash
make build
```

This builds 4 Docker images:
- `strait-loadtest-python` - Python 3.12 jobs
- `strait-loadtest-ts` - TypeScript (Node 22) jobs
- `strait-loadtest-go` - Go binary
- `strait-loadtest-errors` - Error scenario runner

## Jobs

### Python

| Script | Duration | What It Does |
| --- | --- | --- |
| `fast_processor.py` | <1s | Fetches payload, reports usage, completes. Baseline throughput job. |
| `slow_cpu_work.py` | 30-120s | Prime computation. Heartbeats every 5s, checkpoints every 30s, progress every 10s. |
| `ai_agent_simulation.py` | 5-25s | Multi-iteration agent loop: LLM usage, tool calls, streaming, checkpoints. |
| `error_scenarios.py` | varies | 12 failure modes controlled by `ERROR_SCENARIO` env var. |

### TypeScript

| Script | Duration | What It Does |
| --- | --- | --- |
| `data-pipeline.ts` | <2s | Generates 10K records, groups by category, computes aggregates, reports output. |

### Go

| Binary | Duration | What It Does |
| --- | --- | --- |
| `memory-hog` | 10-30s | Allocates configurable memory (50MB-2GB), touches every page, holds for N seconds. |

## Environment Variables

All jobs receive these from Strait:

| Variable | Description |
| --- | --- |
| `STRAIT_SDK_URL` | Base URL of the Strait SDK API |
| `STRAIT_RUN_ID` | The run ID for this execution |
| `STRAIT_RUN_TOKEN` | JWT token for SDK authentication |

### Per-Job Variables

| Job | Variable | Default | Description |
| --- | --- | --- | --- |
| `slow_cpu_work.py` | `WORK_DURATION` | 60 | Duration in seconds |
| `ai_agent_simulation.py` | `AGENT_ITERATIONS` | 5 | Number of agent loop iterations |
| `error_scenarios.py` | `ERROR_SCENARIO` | `clean_exit` | Failure mode (see below) |
| `data-pipeline.ts` | `RECORD_COUNT` | 10000 | Number of records to process |
| `memory-hog` | `MEMORY_TARGET_MB` | 512 | Target allocation in MB |
| `memory-hog` | `HOLD_DURATION_SECS` | 10 | How long to hold memory |

## Error Scenarios

Set `ERROR_SCENARIO` env var to one of:

| Scenario | What It Tests |
| --- | --- |
| `clean_exit` | Happy path baseline |
| `exit_code_1` | Application error detection |
| `exit_code_137` | OOM kill signal detection |
| `oom` | Real OOM (allocate until killed) |
| `segfault` | Crash without error message |
| `infinite_loop` | Timeout enforcement |
| `slow_death` | Failure after 5 min of execution |
| `panic_after_checkpoint` | Checkpoint recovery on retry |
| `sdk_timeout` | Late SDK output reporting |
| `fork_bomb` | Process isolation via cgroups |
| `disk_fill` | Container disk limits |
| `network_abuse` | Outbound connection limits |

## SDK Endpoints Used

All jobs call real SDK endpoints:

- `GET /sdk/v1/runs/{runID}/payload` - Fetch input
- `POST /sdk/v1/runs/{runID}/heartbeat` - Keep alive
- `POST /sdk/v1/runs/{runID}/progress` - Report progress
- `POST /sdk/v1/runs/{runID}/checkpoint` - Save state
- `POST /sdk/v1/runs/{runID}/usage` - Report LLM token usage
- `POST /sdk/v1/runs/{runID}/tool-call` - Record tool calls
- `POST /sdk/v1/runs/{runID}/stream` - Stream output chunks
- `POST /sdk/v1/runs/{runID}/output` - Submit final output
- `POST /sdk/v1/runs/{runID}/complete` - Mark run complete
- `POST /sdk/v1/runs/{runID}/fail` - Mark run failed
