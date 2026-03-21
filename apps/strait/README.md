# Strait CLI

The single command-line interface for the Strait job orchestration platform. A single Go binary with 50+ commands covering job management, workflow orchestration, deployment, local development, and real-time monitoring.

## Installation

### From source

```bash
cd apps/strait && go build -o strait ./cmd/strait
```

### Pre-built binaries

Download from [GitHub Releases](https://github.com/leonardomso/strait/releases).

## Quick Start

```bash
# Initialize a new project
strait init

# Authenticate (opens browser for OAuth)
strait login

# Create a job
strait create job

# Trigger it
strait trigger my-job --payload '{"id": "123"}'

# Watch the run
strait runs watch <run-id>

# Open the dashboard
strait tui
```

## Authentication

Strait CLI supports two authentication methods:

### Browser-based login (recommended)

```bash
strait login
# Opens browser -> approve -> CLI receives token automatically
```

The device code flow works like `gh auth login`: the CLI generates a code, opens your browser to the approval page, and polls until you confirm. The API key is stored in your system keyring.

### Direct token

```bash
# Paste an API key directly
strait login --token strait_abc123...

# Read from stdin (CI/CD)
echo $STRAIT_API_KEY | strait login --with-token
```

### Contexts

Manage multiple environments with contexts:

```bash
strait context create prod --server https://api.strait.dev --project proj-1
strait context create staging --server https://staging.strait.dev --project proj-2
strait context use prod
strait context current
```

## Configuration

### Config files

Strait looks for configuration in this order:
1. `.strait.yaml` in the current directory (local project config)
2. `~/.config/strait/config.yaml` (global user config)

### Environment variables

| Variable | Description |
|----------|-------------|
| `STRAIT_SERVER` | API server URL |
| `STRAIT_API_KEY` | API key for authentication |
| `STRAIT_PROJECT` | Default project ID |
| `STRAIT_FORMAT` | Output format (table, json, yaml, csv) |
| `STRAIT_CONTEXT` | Active context name |
| `NO_COLOR` | Disable color output |

### Global flags

Every command accepts these flags:

```
--server          API server URL
--api-key         API key
--project         Project ID
-o, --format      Output format (table, json, yaml, csv, wide, go-template, jsonpath)
--no-color        Disable color output
-q, --quiet       Minimal output
-v, --verbose     Verbose output
--context         Context name override
--config          Config file path
--timeout         Request timeout (default: 30s)
--ci              CI mode (no color, no interactive prompts)
```

## Project Setup

### Initialize a project

Interactive wizard (TTY):

```bash
strait init
# Walks through: project name, runtime, starter job, deploy target
```

Non-interactive (CI):

```bash
strait init --yes --name my-api --runtime node --with-job \
  --job-name process-payment --job-endpoint http://localhost:3000/jobs/payment
```

### Project config file

The `strait.config.json` file defines your project:

```json
{
  "project": { "id": "my-api", "name": "My API" },
  "runtime": "node",
  "jobs": [
    {
      "slug": "process-payment",
      "name": "Process Payment",
      "endpointUrl": "http://localhost:3000/jobs/process-payment",
      "cron": "*/5 * * * *"
    }
  ]
}
```

Build a deployment manifest from the config:

```bash
strait build --config strait.config.json
strait build --config strait.config.json --dry-run  # preview without writing
```

## Commands

### Job Management

```bash
# List all jobs
strait jobs list --project proj-1

# Create a job (interactive wizard or flags)
strait create job
strait create job --name my-job --endpoint http://localhost:3000/jobs/my-job --project proj-1

# Create from JSON stdin
echo '{"name":"my-job","endpoint_url":"http://localhost:3000"}' | strait create job --json

# Get job details
strait jobs get my-job
strait jobs describe my-job

# Edit a job
strait jobs edit my-job --field "cron=*/10 * * * *"

# Trigger a job
strait trigger my-job --payload '{"key": "value"}'
strait trigger my-job --payload-file input.json
echo '{"key":"value"}' | strait trigger my-job

# Trigger and wait for completion
strait trigger my-job --payload '{}' --wait

# Bulk trigger
strait jobs trigger-bulk my-job --items-json '[{"payload":{"id":"1"}},{"payload":{"id":"2"}}]'

# View version history
strait jobs versions my-job

# Disable a job
strait jobs delete my-job --yes
```

### Run Management

```bash
# List runs
strait runs list --project proj-1
strait runs list --status failed --limit 10

# Get run details
strait runs get run_abc123

# Get the most recent run
strait runs last --project proj-1

# Watch a run in real-time
strait runs watch run_abc123 --timeout 5m

# Cancel runs
strait runs cancel run_abc123
strait runs cancel --all --status executing --project proj-1 --yes

# Replay a failed run
strait runs replay run_abc123

# Compare two runs
strait runs diff run_abc123 run_def456 --show-events
```

### Workflow Orchestration

```bash
# List workflows
strait workflows list --project proj-1

# Create a workflow (interactive wizard)
strait create workflow

# Describe with DAG visualization
strait workflows describe data-pipeline

# Visualize the DAG
strait workflows visualize data-pipeline
strait workflows visualize data-pipeline --run wfr_abc123  # colored by status

# Trigger a workflow
strait workflows trigger data-pipeline --payload '{"date": "2026-03-21"}'

# Manage workflow runs
strait workflow-runs list --project proj-1
strait workflow-runs get wfr_abc123
strait workflow-runs steps wfr_abc123
strait workflow-runs cancel wfr_abc123
```

### Deployment

```bash
# Deploy via Docker image
strait deploy --job my-job --dockerfile ./Dockerfile

# Deploy via manifest config
strait deploy --config strait.config.json

# Deploy a specific job from config
strait deploy --config strait.config.json --job process-payment

# Two-stage deploy (create draft, then finalize)
strait deploy create --config strait.config.json --artifact-uri https://registry.example.com/my-app:v1.0
strait deploy finalize dep_abc123

# Canary deployment
strait deploy --config strait.config.json --strategy canary --canary-percent 10 --canary-duration 5m

# Promote or rollback
strait deploy promote dep_abc123
strait deploy rollback --to dep_abc123

# Preview deployment
strait deploy preview --config strait.config.json

# List deployment history
strait deploy list --project proj-1

# Dry run (see what would happen)
strait deploy --config strait.config.json --dry-run
```

### Logs and Events

```bash
# View run logs
strait logs --run run_abc123

# Stream logs in real-time
strait logs --follow --run run_abc123

# Filter by level and search
strait logs --run run_abc123 --level error --search "timeout"

# Filter by time
strait logs --run run_abc123 --since 1h

# Aggregate logs across jobs
strait logs --project proj-1 --job "process-*" --group

# NDJSON output for piping
strait logs --run run_abc123 --output ndjson | jq '.message'

# Show last N events
strait logs --run run_abc123 --tail 20

# View run events
strait events --run run_abc123
```

When stdout is not a TTY (piped), logs automatically output as NDJSON for pipeline-friendly consumption.

### Secrets Management

```bash
# Server-side secrets
strait secrets list --project proj-1
strait secrets list --project proj-1 --env production
strait secrets create --project proj-1 --name STRIPE_KEY --value sk_live_xxx
strait secrets delete secret_abc123

# Local keyring secrets
strait secrets local set my-secret
strait secrets local get my-secret
strait secrets local delete my-secret
```

### API Keys

```bash
strait api-keys list --project proj-1
strait api-keys create --project proj-1 --name "CI Deploy Key" --scopes "jobs:read,jobs:write,jobs:trigger"
strait api-keys rotate key_abc123
strait api-keys revoke key_abc123
```

### Team Management

```bash
strait team list --project proj-1
strait team add --user user_abc --role operator --project proj-1
strait team remove user_abc --project proj-1
strait team roles --project proj-1
```

### Event Triggers

```bash
strait triggers list --project proj-1
strait triggers get my-event-key
strait triggers send my-event-key --payload '{"data": "value"}'
strait triggers purge --older-than 30 --dry-run
```

## Local Development

### Dev server

```bash
# Start with Docker (Postgres + Redis)
strait dev

# Start without Docker
strait dev --no-docker --port 9090

# Check dev environment readiness
strait dev status
```

### Local job testing

Test jobs locally without a running server:

```bash
# Test a single job
strait dev test process-payment --payload '{"id": "123"}'

# Test with payload from file
strait dev test process-payment --payload-file test.json

# Test all jobs from config
strait dev test --all --config strait.config.json

# Override endpoint
strait dev test process-payment --endpoint http://localhost:3000/jobs/payment

# Pipe payload from stdin
echo '{"id":"1"}' | strait dev test process-payment
```

The test command makes a direct HTTP POST to the job endpoint with the proper Strait headers (`X-Strait-Job-ID`, `X-Strait-Job-Slug`, `X-Strait-Run-ID`, `X-Strait-Attempt`).

### Dev tunnel

Expose your local server via Cloudflare Quick Tunnel:

```bash
strait dev tunnel --port 3000
# Tunnel active: https://abc123.trycloudflare.com -> localhost:3000
```

No Cloudflare account needed. Auto-detects or downloads `cloudflared`.

## Interactive Features

### TUI Dashboard

```bash
strait tui --project proj-1
```

Live terminal dashboard with queue metrics, run explorer, and event timeline.

Keyboard shortcuts:
- `?` — help overlay
- `Tab` — cycle focus
- `j/k` — navigate up/down
- `Enter` — inspect selected item
- `t` — trigger job from selected run
- `c` — cancel selected run
- `r` — refresh data
- `q` — quit

### Interactive Wizards

When run without flags in a TTY, `strait init`, `strait create job`, and `strait create workflow` launch interactive wizards using rich terminal forms:

```bash
$ strait create job
? Job name: sync-inventory
? Endpoint URL: https://api.example.com/jobs/sync
? Schedule (cron, optional): @hourly
? Timeout (seconds) [60]: 120
? Max retry attempts [3]: 3

Created job "sync-inventory" (job_abc123)
```

All interactive features fall back to flag-based input in non-TTY environments (CI/CD).

## CI/CD Integration

### Generate CI pipeline

```bash
# Auto-detect CI provider and generate workflow
strait ci setup

# Validate CI readiness
strait ci check
```

Supports GitHub Actions, GitLab CI, CircleCI, Bitbucket Pipelines, and Jenkins.

### CI mode

Use `--ci` flag or `CI=true` environment variable to disable colors and interactive prompts:

```bash
strait deploy --config strait.config.json --ci
```

## Extensions

### Using extensions

```bash
# List installed extensions
strait extension list

# Install from GitHub
strait extension install github.com/user/strait-ext-foo

# Run an extension
strait extension run foo

# Remove
strait extension remove foo
```

### Creating extensions

```bash
# Scaffold a new plugin
strait extension create my-plugin

# Creates:
#   my-plugin/
#     strait-plugin.json
#     main.go
#     README.md
```

### Hooks

Extensions can register hooks that run before/after CLI actions:

- `pre-deploy` / `post-deploy`
- `pre-trigger` / `post-trigger`
- `pre-build` / `post-build`

Pre-hooks block on failure. Post-hooks warn but don't block. Skip all hooks with `STRAIT_SKIP_HOOKS=1`.

## Diagnostics

```bash
# Comprehensive health check
strait doctor

# System status overview
strait status

# Check server health
strait health

# Trace a run execution
strait trace run_abc123

# Debug bundle for support
strait debug bundle run_abc123

# Performance analytics
strait perf --project proj-1

# Queue depth monitoring
strait top
strait top jobs
strait top queue
```

## Output Formats

All commands support multiple output formats:

```bash
strait jobs list -o json
strait jobs list -o yaml
strait jobs list -o csv
strait jobs list -o wide
strait jobs list -o go-template --output-template '{{.ID}} {{.Status}}'
strait jobs list -o jsonpath --output-jsonpath '$.data[*].id'
```

## Aliases

Create shortcuts for common commands:

```bash
strait alias set rj "runs list --status failed"
strait alias set trig "trigger"

# Now use them:
strait rj --project proj-1
strait trig my-job --payload '{}'

# List all aliases
strait alias list

# Delete an alias
strait alias delete rj
```

Aliases are stored in `~/.config/strait/config.yaml` and expand before command parsing.

## Shell Completion

```bash
# Bash
strait completion bash > /etc/bash_completion.d/strait

# Zsh
strait completion zsh > "${fpath[1]}/_strait"

# Fish
strait completion fish > ~/.config/fish/completions/strait.fish
```

## Database Management

```bash
# Open psql shell
strait db shell
strait db shell --query "SELECT count(*) FROM job_runs"

# Database stats
strait db stats

# Run migrations
strait migrate up
strait migrate status
strait migrate create add-new-feature
```

## Declarative Configuration

```bash
# Export current state as YAML
strait export all --project proj-1 --output-dir ./definitions

# Validate definition files
strait validate

# Preview changes
strait diff

# Apply changes
strait apply
```

## Architecture

The CLI is built with:

- **[Cobra](https://github.com/spf13/cobra)** for command structure
- **[charmbracelet/huh](https://github.com/charmbracelet/huh)** for interactive terminal forms
- **[rivo/tview](https://github.com/rivo/tview)** for the TUI dashboard
- **[lipgloss](https://github.com/charmbracelet/lipgloss)** for terminal styling

### Package structure

```
cmd/strait/              Command definitions (50+ commands)
internal/cli/
  auth/                  Keyring credential storage
  ci/                    CI provider detection and config generation
  client/                HTTP API client (51 methods)
  config/                Config file management and context resolution
  dag/                   Workflow DAG rendering
  deploy/                Docker build/push and manifest deployment
  devtest/               Local job testing engine
  extension/             Plugin manifest, hooks, and lifecycle management
  manifest/              Project config loading and manifest compilation
  output/                Multi-format output rendering
  styles/                Terminal color and formatting
  tui/                   TUI dashboard components
  tunnel/                Cloudflare tunnel integration
  wizard/                Interactive form validation and builders
```

## Development

```bash
# Build
cd apps/strait && go build ./...

# Test
go test ./... -count=1 -timeout=120s

# Test with race detector
go test ./... -race -timeout=120s

# Lint
golangci-lint run --timeout=5m ./...

# Integration tests (requires Docker)
go test -tags=integration ./internal/store/... ./internal/queue/... ./internal/e2e/...
```
