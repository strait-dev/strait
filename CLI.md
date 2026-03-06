# Orchestrator CLI

Command-line interface for managing jobs, runs, workflows, and server operations.

## Installation

```bash
# Build from source
go build -o orchestrator ./cmd/orchestrator

# Or install to GOPATH/bin
go install ./cmd/orchestrator
```

## Quick Start

```bash
# Initialize project files
orchestrator init --yes --name my-project

# Start local dev environment (Postgres + Redis via Docker)
orchestrator dev

# Authenticate
orchestrator login --server http://localhost:8080

# List jobs
orchestrator jobs list

# Trigger a job
orchestrator trigger my-job --payload '{"key": "value"}'

# Watch the run
orchestrator runs watch <run-id>
```

## Configuration

### Config File

The CLI reads configuration from a YAML file. Search paths (in order):

1. `.orchestrator.yaml` in the current working directory
2. `~/.config/orchestrator/config.yaml`
3. Explicit path via `--config` flag

```yaml
server: http://localhost:8080
project: my-project
format: table
active_context: default
contexts:
  default:
    server: http://localhost:8080
    project: my-project
    format: table
  production:
    server: https://api.example.com
    project: prod-app
    format: json
aliases:
  trig: "trigger"
  lj: "jobs list"
```

### Contexts

Contexts are named configuration profiles for different environments. Each context stores a server URL, default project, and output format. API keys are stored separately in the system keychain, scoped per context.

```bash
# Create a production context
orchestrator context create prod --server https://api.prod.com --project prod-app

# Switch to it
orchestrator context use prod

# Store API key securely
orchestrator login --context prod

# List all contexts
orchestrator context list

# Show active context
orchestrator context current
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ORCHESTRATOR_SERVER` | Server URL |
| `ORCHESTRATOR_API_KEY` | API key |
| `ORCHESTRATOR_PROJECT` | Default project ID |
| `ORCHESTRATOR_FORMAT` | Output format |
| `ORCHESTRATOR_CONTEXT` | Active context name |
| `ORCHESTRATOR_CI` | Enable CI mode (`true`) |
| `NO_COLOR` | Disable color output |
| `DATABASE_URL` | PostgreSQL connection string (for server/migrate/db commands) |
| `REDIS_URL` | Redis connection string (for server commands) |

### Resolution Hierarchy

Settings are resolved from highest to lowest priority:

1. **CLI flags** (`--server`, `--project`, etc.)
2. **Environment variables** (`ORCHESTRATOR_SERVER`, etc.)
3. **Active context** values from config file
4. **Config file** top-level defaults

## Authentication

API keys are stored in the system keychain (macOS Keychain, Windows Credential Manager, Linux secret-service/pass).

```bash
# Interactive login (prompts for API key)
orchestrator login

# Provide key directly
orchestrator login --api-key sk_live_abc123

# Pipe from stdin
echo "sk_live_abc123" | orchestrator login --with-token

# Login to specific context and server
orchestrator login --context prod --server https://api.prod.com

# Check auth status
orchestrator auth status

# Remove stored key
orchestrator logout
```

The login command validates the API key against the server before storing it.

## Global Flags

These flags are available on all commands:

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--server` | | string | | Server URL |
| `--api-key` | | string | | API key |
| `--project` | | string | | Default project ID |
| `--format` | `-o` | string | `table` | Output format |
| `--no-headers` | | bool | `false` | Omit table headers |
| `--output-template` | | string | | Go template for `--format go-template` |
| `--output-jsonpath` | | string | | JSONPath for `--format jsonpath` |
| `--no-color` | | bool | `false` | Disable color output |
| `--quiet` | `-q` | bool | `false` | Minimal output (IDs only) |
| `--verbose` | `-v` | bool | `false` | Verbose output |
| `--context` | | string | | Context name override |
| `--config` | | string | | Config file path |
| `--timeout` | | duration | `30s` | API request timeout |
| `--ci` | | bool | `false` | CI mode (no color, no prompts) |

## Output Formats

The CLI supports seven output formats:

| Format | Description | When Used |
|--------|-------------|-----------|
| `table` | Human-readable table | Default for TTY |
| `json` | Indented JSON | Default for pipes/non-TTY |
| `yaml` | YAML format | `--format yaml` |
| `csv` | Comma-separated values | `--format csv` |
| `wide` | Table with all columns | `--format wide` |
| `go-template` | Go template rendering | `--format go-template --output-template '...'` |
| `jsonpath` | JSONPath extraction | `--format jsonpath --output-jsonpath '...'` |

```bash
# JSON output
orchestrator jobs list --format json

# YAML output
orchestrator runs get run_abc --format yaml

# Go template
orchestrator runs list --format go-template --output-template '{{range .}}{{.id}}: {{.status}}{{"\n"}}{{end}}'

# JSONPath
orchestrator jobs list --format jsonpath --output-jsonpath '$[*].name'

# Quiet mode (IDs only, one per line)
orchestrator runs list --quiet

# Pipe-friendly (auto-switches to JSON)
orchestrator jobs list | jq '.[] | .name'
```

## Commands

### Server Runtime

#### `orchestrator serve`

Start orchestrator server components.

```bash
orchestrator serve
orchestrator serve --mode api
orchestrator serve --mode worker
orchestrator serve --mode all
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode` | string | | Run mode: `api`, `worker`, or `all` (overrides `MODE` env) |

Running `orchestrator` with no subcommand or with `--mode` also starts the server.

#### `orchestrator server start`

Start orchestrator server (equivalent to `serve`).

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--mode` | string | | Run mode: `api`, `worker`, or `all` |

#### `orchestrator dev`

Start local development environment with Docker dependencies and sensible defaults.

```bash
orchestrator dev
orchestrator dev --no-docker --port 9090
orchestrator dev --seed
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-docker` | bool | `false` | Skip docker compose startup |
| `--port` | int | `8080` | API port |
| `--seed` | bool | `false` | Attempt to seed example data |

Automatically sets `DATABASE_URL`, `REDIS_URL`, `INTERNAL_SECRET`, `JWT_SIGNING_KEY`, and `LOG_LEVEL=debug` if not already set. Starts Postgres and Redis via `docker compose up -d`.

#### `orchestrator dev status`

Run local development readiness checks (Docker, env vars, server health).

```bash
orchestrator dev status
```

### Jobs

#### `orchestrator jobs list`

List jobs for a project.

```bash
orchestrator jobs list
orchestrator jobs list --project proj_1
orchestrator jobs list --format json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |

#### `orchestrator jobs get`

Get a job by ID or slug.

```bash
orchestrator jobs get job_abc123
orchestrator jobs get send-email
```

#### `orchestrator jobs create`

Create a new job.

```bash
orchestrator jobs create \
  --name "Send Email" \
  --slug send-email \
  --endpoint http://localhost:3000/jobs/send-email \
  --timeout-secs 60 \
  --max-attempts 3
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--name` | string | | Job name |
| `--slug` | string | | Job slug |
| `--description` | string | | Job description |
| `--cron` | string | | Cron schedule expression |
| `--endpoint` | string | | Job endpoint URL |
| `--timeout-secs` | int | `60` | Execution timeout in seconds |
| `--max-attempts` | int | `3` | Maximum retry attempts |
| `--run-ttl-secs` | int | `0` | Run TTL in seconds (0 = no expiry) |

#### `orchestrator jobs trigger`

Trigger a job run.

```bash
orchestrator jobs trigger send-email
orchestrator jobs trigger send-email --payload '{"to": "user@example.com"}'
orchestrator jobs trigger send-email --payload-file payload.json
orchestrator jobs trigger send-email --priority 10 --idempotency-key req-123
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--payload` | string | | Inline JSON payload |
| `--payload-file` | string | | Path to payload JSON file |
| `--priority` | int | `0` | Run priority |
| `--scheduled-at` | string | | RFC3339 timestamp for delayed execution |
| `--idempotency-key` | string | | Idempotency key |

#### `orchestrator jobs trigger-bulk`

Trigger multiple runs for a job.

```bash
orchestrator jobs trigger-bulk send-email --items-json '[{"payload": {"to": "a@b.com"}}, {"payload": {"to": "c@d.com"}}]'
orchestrator jobs trigger-bulk send-email --items-file items.json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--items-json` | string | | JSON array of bulk trigger items |
| `--items-file` | string | | Path to JSON file with bulk trigger items |

#### `orchestrator jobs delete`

Disable a job by ID or slug.

```bash
orchestrator jobs delete send-email
orchestrator jobs delete send-email --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip confirmation prompt |

#### `orchestrator jobs versions`

List version history for a job.

```bash
orchestrator jobs versions send-email
```

#### `orchestrator jobs describe`

Show rich details and recent runs for a job.

```bash
orchestrator jobs describe send-email
```

#### `orchestrator jobs edit`

Edit a job via `--field` flag or interactive editor.

```bash
orchestrator jobs edit send-email --field timeout_secs=120
orchestrator jobs edit send-email --editor vim
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--field` | string | | Field update in `key=value` form |
| `--editor` | string | | Editor command for interactive mode |

### Runs

#### `orchestrator runs list`

List runs with optional filters.

```bash
orchestrator runs list
orchestrator runs list --status executing --limit 20
orchestrator runs list --project proj_1 --format json
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--status` | string | | Status filter |
| `--limit` | int | `50` | Maximum runs to return |

#### `orchestrator runs get`

Get a run by ID.

```bash
orchestrator runs get run_abc123
```

#### `orchestrator runs cancel`

Cancel one or more runs.

```bash
orchestrator runs cancel run_abc123
orchestrator runs cancel run_abc run_def run_ghi
orchestrator runs cancel --all --project proj_1 --status queued --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool | `false` | Cancel all runs matching filters |
| `--project` | string | | Project ID (for `--all`) |
| `--status` | string | | Status filter (for `--all`) |
| `--limit` | int | `100` | Max runs to consider (for `--all`) |
| `--yes` | bool | `false` | Skip confirmation prompt |

#### `orchestrator runs logs`

Show run events and logs.

```bash
orchestrator runs logs run_abc123
orchestrator runs logs run_abc123 --follow
orchestrator runs logs run_abc123 --level error --type state_change
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--follow` / `-f` | bool | `false` | Stream logs by polling |
| `--interval` | duration | `2s` | Poll interval when following |
| `--level` | string | | Event level filter |
| `--type` | string | | Event type filter |

#### `orchestrator runs watch`

Watch a run until it reaches a terminal state.

```bash
orchestrator runs watch run_abc123
orchestrator runs watch run_abc123 --timeout 10m
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--interval` | duration | `2s` | Poll interval |
| `--timeout` | duration | `5m` | Max watch duration (0 = no limit) |

#### `orchestrator runs replay`

Replay a run using its original payload.

```bash
orchestrator runs replay run_abc123
orchestrator runs replay run_abc123 --wait
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--wait` | bool | `false` | Wait for replayed run to complete |

### Workflows

#### `orchestrator workflows list`

List workflows for a project.

```bash
orchestrator workflows list --project proj_1
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |

#### `orchestrator workflows get`

Get a workflow by ID or slug.

```bash
orchestrator workflows get data-pipeline
```

#### `orchestrator workflows describe`

Show workflow details and step dependency graph.

```bash
orchestrator workflows describe data-pipeline
```

#### `orchestrator workflows create`

Create a workflow with steps.

```bash
orchestrator workflows create \
  --name "Data Pipeline" \
  --slug data-pipeline \
  --project proj_1 \
  --steps-json '[{"job_id": "job_1", "step_ref": "extract"}, {"job_id": "job_2", "step_ref": "transform", "depends_on": ["extract"]}]'
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--name` | string | | Workflow name |
| `--slug` | string | | Workflow slug |
| `--description` | string | | Workflow description |
| `--steps-json` | string | | JSON array of workflow steps |

#### `orchestrator workflows update`

Update an existing workflow.

```bash
orchestrator workflows update data-pipeline --description "Updated pipeline"
orchestrator workflows update data-pipeline --enabled=false
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | | Workflow name |
| `--slug` | string | | Workflow slug |
| `--description` | string | | Workflow description |
| `--enabled` | bool | | Workflow enabled state |
| `--steps-json` | string | | JSON array of workflow steps |

#### `orchestrator workflows delete`

Delete a workflow.

```bash
orchestrator workflows delete data-pipeline --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip confirmation prompt |

#### `orchestrator workflows runs`

List runs for a specific workflow.

```bash
orchestrator workflows runs data-pipeline --limit 20
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--limit` | int | `50` | Maximum runs to return |
| `--offset` | int | `0` | Pagination offset |

#### `orchestrator workflows trigger`

Trigger a workflow run.

```bash
orchestrator workflows trigger data-pipeline --payload '{"source": "s3://bucket/data.csv"}'
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--payload` | string | | Inline JSON payload |
| `--payload-file` | string | | Path to payload JSON file |

### Workflow Runs

#### `orchestrator workflow-runs list`

List workflow runs.

```bash
orchestrator workflow-runs list --project proj_1
orchestrator workflow-runs list --status running --limit 10
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--status` | string | | Status filter |
| `--limit` | int | `50` | Maximum runs |

#### `orchestrator workflow-runs get`

Get a workflow run by ID.

```bash
orchestrator workflow-runs get wfr_abc123
```

#### `orchestrator workflow-runs cancel`

Cancel a workflow run and all its step runs.

```bash
orchestrator workflow-runs cancel wfr_abc123
```

#### `orchestrator workflow-runs steps`

List step runs for a workflow run.

```bash
orchestrator workflow-runs steps wfr_abc123
```

#### `orchestrator workflow-runs watch`

Watch workflow run status and step progression.

```bash
orchestrator workflow-runs watch wfr_abc123
orchestrator workflow-runs watch wfr_abc123 --timeout 10m
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--interval` | duration | `2s` | Poll interval |
| `--timeout` | duration | `5m` | Max watch duration |

### API Keys

#### `orchestrator api-keys create`

Create an API key for a project.

```bash
orchestrator api-keys create --project proj_1 --name production
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--name` | string | | Key name |
| `--scopes` | string | | Comma-separated scopes |

The raw API key is returned only once on creation.

#### `orchestrator api-keys list`

List API keys for a project.

```bash
orchestrator api-keys list --project proj_1
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |

#### `orchestrator api-keys revoke`

Revoke an API key.

```bash
orchestrator api-keys revoke key_abc123
```

#### `orchestrator api-keys rotate`

Rotate an API key (create new, revoke old).

```bash
orchestrator api-keys rotate key_abc123
orchestrator api-keys rotate key_abc123 --name production-v2
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | | Name for the new key |

### Declarative Management

Manage resources using YAML manifest files. Manifests follow this structure:

```yaml
apiVersion: v1
kind: Job
metadata:
  name: send-email
spec:
  project_id: my-project
  slug: send-email
  endpoint_url: http://localhost:3000/jobs/send-email
  timeout_secs: 60
  max_attempts: 3
```

Supported kinds: `Job`, `Workflow`.

#### `orchestrator validate`

Validate manifest files without applying.

```bash
orchestrator validate -f definitions/jobs.yaml
orchestrator validate -f definitions/
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` / `-f` | string[] | | Manifest file, directory, or `-` for stdin |

#### `orchestrator apply`

Apply declarative definitions to the server.

```bash
orchestrator apply -f definitions/jobs.yaml
orchestrator apply -f definitions/ --dry-run
cat manifest.yaml | orchestrator apply -f -
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` / `-f` | string[] | | Manifest file, directory, or `-` for stdin |
| `--dry-run` | bool | `false` | Preview without creating resources |

#### `orchestrator diff`

Show differences between local manifests and server state.

```bash
orchestrator diff -f definitions/jobs.yaml
orchestrator diff -f definitions/
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` / `-f` | string[] | | Manifest file, directory, or `-` for stdin |

#### `orchestrator check`

Validate manifests with deep checks (endpoint reachability, schema validation).

```bash
orchestrator check -f definitions/ --check-endpoints
orchestrator check -f jobs.yaml -f workflows.yaml --endpoint-timeout 5s
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` / `-f` | string[] | | Manifest file, directory, or `-` for stdin |
| `--check-endpoints` | bool | `false` | Test endpoint URL reachability |
| `--endpoint-timeout` | duration | `3s` | Timeout for endpoint checks |

#### `orchestrator export`

Export server state as declarative YAML files.

```bash
orchestrator export jobs --project proj_1
orchestrator export all --project proj_1 --output-dir definitions
orchestrator export workflows --name-contains billing --dry-run
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--output-dir` | string | | Output directory for YAML files |
| `--name-contains` | string | | Filter resources by name |
| `--dry-run` | bool | `false` | Preview without writing files |
| `--force-overwrite` | bool | `false` | Overwrite existing files |

Valid resources: `jobs`, `workflows`, `api-keys`, `all`.

### Monitoring and Diagnostics

#### `orchestrator health`

Check server health.

```bash
orchestrator health
orchestrator health --ready
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ready` | bool | `false` | Check readiness endpoint (verifies Postgres + Redis) |

#### `orchestrator stats`

Show queue statistics (queued, executing, delayed counts).

```bash
orchestrator stats
```

#### `orchestrator verify`

Run post-deployment verification checks.

```bash
orchestrator verify
```

#### `orchestrator diagnose`

Run troubleshooting diagnostics for connectivity and configuration.

```bash
orchestrator diagnose
orchestrator diagnose --verbose
orchestrator diagnose --check-readiness
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--verbose` | bool | `false` | Show full diagnostics context |
| `--check-readiness` | bool | `false` | Include readiness check |

#### `orchestrator diagnose run`

Diagnose a specific run with timeline and event analysis.

```bash
orchestrator diagnose run run_abc123
orchestrator diagnose run run_abc123 --follow --show-payload
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--follow` | bool | `false` | Continuously refresh until terminal state |
| `--interval` | duration | `2s` | Poll interval when following |
| `--level` | string | | Event level filter |
| `--type` | string | | Event type filter |
| `--show-payload` | bool | `false` | Include run payload |
| `--show-result` | bool | `false` | Include run result |
| `--event-limit` | int | `50` | Maximum events to include |

#### `orchestrator top`

Show live resource usage stats. Defaults to queue view.

```bash
orchestrator top
orchestrator top --watch
orchestrator top queue --project proj_1
orchestrator top jobs --project proj_1 --watch --interval 5s
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--watch` | bool | `false` | Refresh continuously |
| `--interval` | duration | `2s` | Refresh interval |

Subcommands: `queue` (queue depth snapshot), `jobs` (job metrics).

#### `orchestrator tui`

Launch interactive terminal dashboard with queue metrics, run explorer, and event timeline.

```bash
orchestrator tui --project proj_1
orchestrator tui --interval 3s --run-limit 30
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--interval` | duration | | Refresh interval |
| `--run-limit` | int | | Max runs to display |
| `--event-limit` | int | | Max events to display |

#### `orchestrator listen`

Watch for new runs in real time. Deduplicates by run ID.

```bash
orchestrator listen --project proj_1
orchestrator listen --project proj_1 --status executing
orchestrator listen --interval 3s
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--status` | string | | Filter by status |
| `--interval` | duration | `2s` | Poll interval |

#### `orchestrator drain`

Wait for all executing runs to complete. Useful before shutdown or maintenance.

```bash
orchestrator drain
orchestrator drain --timeout 5m
orchestrator drain --interval 3s --timeout 10m
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--timeout` | duration | `5m` | Max time to wait |
| `--interval` | duration | `2s` | Poll interval |

Exits 0 when executing count reaches 0, exits 1 on timeout.

#### `orchestrator trace`

Render an ASCII timeline visualization of a run's lifecycle events.

```bash
orchestrator trace run_abc123
orchestrator trace run_abc123 --show-payload --show-result
orchestrator trace run_abc123 --event-limit 100
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--show-payload` | bool | `false` | Include run payload |
| `--show-result` | bool | `false` | Include run result |
| `--event-limit` | int | `50` | Maximum events to show |

#### `orchestrator logs`

View run events (alternative to `runs logs`).

```bash
orchestrator logs --run run_abc123
orchestrator logs --project proj_1 --level error
orchestrator logs --run run_abc123 --follow
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--run` | string | | Run ID |
| `--project` | string | | Project ID |
| `--level` | string | | Event level filter |
| `--type` | string | | Event type filter |
| `--follow` / `-f` | bool | `false` | Follow log stream |
| `--interval` | duration | `2s` | Poll interval in follow mode |

#### `orchestrator events`

Inspect run events.

```bash
orchestrator events --run run_abc123
orchestrator events --run run_abc123 --level error --type state_change
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--run` | string | | Run ID (required) |
| `--level` | string | | Event level filter |
| `--type` | string | | Event type filter |

### Database

#### `orchestrator db shell`

Open a psql shell using `DATABASE_URL`.

```bash
orchestrator db shell
orchestrator db shell --query "SELECT count(*) FROM job_runs"
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--query` | string | | Run one SQL query and exit |

#### `orchestrator db stats`

Show database table sizes and connection statistics.

```bash
orchestrator db stats
```

#### `orchestrator migrate up`

Apply pending database migrations.

```bash
orchestrator migrate up
orchestrator migrate up 3
```

Accepts an optional argument for the number of migrations to apply.

#### `orchestrator migrate down`

Rollback migrations.

```bash
orchestrator migrate down 1
orchestrator migrate down 1 --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Skip confirmation prompt |

#### `orchestrator migrate status`

Show current migration version.

```bash
orchestrator migrate status
```

#### `orchestrator migrate create`

Create a new up/down SQL migration pair.

```bash
orchestrator migrate create add_user_preferences
```

Creates `migrations/<version>_<name>.up.sql` and `migrations/<version>_<name>.down.sql`.

#### `orchestrator backup create`

Create a database backup using pg_dump.

```bash
orchestrator backup create
orchestrator backup create --output backup.sql
orchestrator backup create --format custom --output backup.dump
orchestrator backup create --database-url postgres://user:pass@host:5432/db
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` / `-o` | string | | Output file (default: timestamped filename) |
| `--database-url` | string | | PostgreSQL connection string (default: `DATABASE_URL` env) |
| `--format` | string | `plain` | Dump format: `plain`, `custom`, `directory`, `tar` |
| `--verbose` / `-V` | bool | `false` | Pass `--verbose` to pg_dump |

#### `orchestrator backup restore`

Restore a database from a backup file. Auto-detects format (pg_restore for custom/directory/tar, psql for plain SQL).

```bash
orchestrator backup restore --input backup.sql
orchestrator backup restore --input backup.dump --clean
orchestrator backup restore --input backup.dump --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--input` / `-i` | string | | Backup file to restore (required) |
| `--database-url` | string | | PostgreSQL connection string (default: `DATABASE_URL` env) |
| `--clean` | bool | `false` | Drop objects before restoring |
| `--verbose` / `-V` | bool | `false` | Pass `--verbose` to pg_restore/psql |
| `--yes` | bool | `false` | Skip confirmation prompt |

### Secrets

Secrets are stored in the system keychain, scoped per project.

#### `orchestrator secrets create`

Create or update a secret value.

```bash
orchestrator secrets create DB_PASSWORD
orchestrator secrets create DB_PASSWORD --from-env DATABASE_PASSWORD
orchestrator secrets create DB_PASSWORD --from-file /path/to/secret
orchestrator secrets create DB_PASSWORD --project proj_1
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from-env` | string | | Read value from environment variable |
| `--from-file` | string | | Read value from file |
| `--project` | string | | Project ID |

If no source flag is provided, prompts for the value interactively.

#### `orchestrator secrets list`

List secret names for a project.

```bash
orchestrator secrets list
orchestrator secrets list --project proj_1
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |

#### `orchestrator secrets delete`

Delete a secret.

```bash
orchestrator secrets delete DB_PASSWORD
orchestrator secrets delete DB_PASSWORD --project proj_1
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |

### Utilities

#### `orchestrator trigger`

Shortcut for `jobs trigger`.

```bash
orchestrator trigger send-email --payload '{"to": "user@example.com"}'
orchestrator trigger send-email --payload-file payload.json --priority 5
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--payload` | string | | Inline JSON payload |
| `--payload-file` | string | | Path to payload JSON file |
| `--priority` | int | `0` | Run priority |

Resolves job by ID first, then by slug (requires `--project` for slug resolution).

#### `orchestrator api`

Call raw orchestrator API endpoints.

```bash
orchestrator api GET /v1/jobs?project_id=proj_1
orchestrator api POST /v1/jobs --field name=my-job --field slug=my-job --field project_id=proj_1
orchestrator api GET /v1/runs --header "X-Custom: value"
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--header` | string[] | | Additional headers as `Key:Value` |
| `--field` | string[] | | JSON body fields as `key=value` |

#### `orchestrator run`

Run a local command with orchestrator context environment variables injected.

```bash
orchestrator run -- env | grep ORCHESTRATOR
orchestrator run -- ./scripts/deploy.sh
```

Injects `ORCHESTRATOR_SERVER`, `ORCHESTRATOR_API_KEY`, `ORCHESTRATOR_PROJECT`, `ORCHESTRATOR_FORMAT`, `ORCHESTRATOR_CONTEXT`, and `NO_COLOR` into the child process environment.

#### `orchestrator send`

Send a raw event payload to the orchestrator server.

```bash
orchestrator send deploy.started --data '{"version": "1.2.3"}'
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data` | string | | JSON payload for event data |

#### `orchestrator wait run`

Wait for a run to reach a specific state.

```bash
orchestrator wait run run_abc123
orchestrator wait run run_abc123 --for status=completed --timeout 10m
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--for` | string | `status=completed` | Condition expression |
| `--timeout` | duration | `5m` | Max wait duration |
| `--interval` | duration | `2s` | Poll interval |

#### `orchestrator wait queue`

Wait for queue to drain.

```bash
orchestrator wait queue --empty --timeout 10m
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--empty` | bool | `false` | Wait until queue is empty |
| `--timeout` | duration | `10m` | Max wait duration |
| `--interval` | duration | `2s` | Poll interval |

#### `orchestrator cleanup`

Remove old terminal-state runs.

```bash
orchestrator cleanup --runs-older-than 720h --dry-run
orchestrator cleanup --runs-older-than 720h --yes
orchestrator cleanup --runs-older-than 168h --status failed --yes
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | string | | Project ID |
| `--runs-older-than` | duration | | Remove runs older than this (required) |
| `--status` | string | | Target specific status (default: all terminal) |
| `--dry-run` | bool | `false` | Preview without deleting |
| `--yes` | bool | `false` | Skip confirmation prompt |
| `--limit` | int | `100` | Max runs to fetch per status |

Terminal statuses: `completed`, `failed`, `timed_out`, `crashed`, `system_failed`, `canceled`, `expired`.

#### `orchestrator fixtures create`

Create fixture data for demos and testing.

```bash
orchestrator fixtures create
orchestrator fixtures create --template full
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--template` | string | `minimal` | Fixture template: `minimal` or `full` |

#### `orchestrator fixtures clean`

Clean up fixture data.

```bash
orchestrator fixtures clean
```

### Configuration Commands

#### `orchestrator init`

Initialize project files (config, .env, docker-compose, manifests).

```bash
orchestrator init
orchestrator init --yes --name my-project --template full
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--yes` | bool | `false` | Non-interactive mode |
| `--name` | string | `demo-project` | Project name |
| `--template` | string | `minimal` | Template: `minimal` or `full` |

Creates:
- `.orchestrator.yaml` — CLI config
- `.env` — environment variables
- `docker-compose.yml` — local Postgres + Redis
- `definitions/jobs.yaml` — job manifest
- `definitions/workflows.yaml` — workflow manifest (full template only)

#### `orchestrator context create`

Create or update a named context.

```bash
orchestrator context create prod --server https://api.prod.com --project prod-app
orchestrator context create staging --server https://staging.example.com --api-key sk_test_123
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--server` | string | | Server URL |
| `--project` | string | | Default project |
| `--format` | string | | Output format |
| `--api-key` | string | | API key (stored in keychain) |

#### `orchestrator context use`

Set the active context.

```bash
orchestrator context use prod
```

#### `orchestrator context list`

List all configured contexts.

```bash
orchestrator context list
```

#### `orchestrator context current`

Show the active context and its settings.

```bash
orchestrator context current
```

#### `orchestrator alias set`

Set a command alias.

```bash
orchestrator alias set trig "trigger"
orchestrator alias set lj "jobs list"
orchestrator alias set lr "runs list --status executing"
```

Aliases expand the first argument before command parsing:

```bash
orchestrator trig my-job    # expands to: orchestrator trigger my-job
orchestrator lj             # expands to: orchestrator jobs list
```

#### `orchestrator alias list`

List all configured aliases.

```bash
orchestrator alias list
```

#### `orchestrator alias delete`

Delete a command alias.

```bash
orchestrator alias delete trig
```

#### `orchestrator profile`

Capture a pprof profile from the running server.

```bash
orchestrator profile
orchestrator profile --type cpu --duration 30s --output cpu.prof
orchestrator profile --type heap --output heap.prof
orchestrator profile --type goroutine
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` / `-o` | string | | Output file (default: `<type>-<timestamp>.prof`) |
| `--duration` | duration | `30s` | Profile duration (for CPU profiles) |
| `--type` | string | `cpu` | Profile type: `cpu`, `heap`, `goroutine`, `allocs`, `block`, `mutex`, `threadcreate` |

### Meta Commands

#### `orchestrator version`

Print CLI version information.

```bash
orchestrator version
orchestrator version --short
orchestrator version --json
orchestrator version --check-server --check-update
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--short` | bool | `false` | Print only version number |
| `--json` | bool | `false` | Print as JSON |
| `--check-server` | bool | `false` | Check server health |
| `--check-update` | bool | `false` | Check for newer version |

#### `orchestrator completion`

Generate shell completion scripts.

```bash
# Bash
orchestrator completion bash > /etc/bash_completion.d/orchestrator

# Zsh
orchestrator completion zsh > "${fpath[1]}/_orchestrator"

# Fish
orchestrator completion fish > ~/.config/fish/completions/orchestrator.fish

# PowerShell
orchestrator completion powershell > orchestrator.ps1
```

#### `orchestrator docs`

Generate CLI documentation in man page or markdown format.

```bash
orchestrator docs --man
orchestrator docs --markdown
```

#### `orchestrator upgrade`

Upgrade the CLI to the latest version.

```bash
orchestrator upgrade
```

#### `orchestrator extension list`

List discovered CLI extensions.

```bash
orchestrator extension list
```

#### `orchestrator extension run`

Run a CLI extension.

```bash
orchestrator extension run deploy production
```

## Shell Completion

Generate and install completion scripts for your shell:

```bash
# Bash
orchestrator completion bash > /etc/bash_completion.d/orchestrator
source /etc/bash_completion.d/orchestrator

# Zsh (add to .zshrc)
orchestrator completion zsh > "${fpath[1]}/_orchestrator"
compinit

# Fish
orchestrator completion fish > ~/.config/fish/completions/orchestrator.fish

# PowerShell (add to profile)
orchestrator completion powershell | Out-String | Invoke-Expression
```

Completions include flag names, output format values, context names, and shell names.

## Extensions

Extensions are external executables named `orchestrator-<name>` found in your `PATH`. They integrate as CLI subcommands.

```bash
# Create an extension
cat > /usr/local/bin/orchestrator-deploy << 'EOF'
#!/bin/bash
echo "Deploying to $1..."
EOF
chmod +x /usr/local/bin/orchestrator-deploy

# Discover it
orchestrator extension list

# Run it
orchestrator extension run deploy production
```

Extensions receive full stdin/stdout/stderr passthrough and all remaining arguments after the extension name.

## CI/CD Integration

The CLI is designed for non-interactive use in CI pipelines.

```bash
# Configure via environment
export ORCHESTRATOR_SERVER=https://api.example.com
export ORCHESTRATOR_API_KEY=$SECRET_API_KEY
export ORCHESTRATOR_PROJECT=my-project

# Use --ci flag for non-interactive mode (no color, no prompts)
orchestrator --ci jobs list
orchestrator --ci trigger my-job --payload '{"deploy": true}'

# Wait for completion
RUN_ID=$(orchestrator --ci trigger my-job --quiet)
orchestrator --ci wait run $RUN_ID --for status=completed --timeout 10m

# Drain before shutdown
orchestrator --ci drain --timeout 5m

# Health checks
orchestrator --ci health --ready
```

CI mode activates automatically when `CI=true` or `ORCHESTRATOR_CI=true` is set, or when `--ci` is passed.

## Command Tree

```
orchestrator
├── serve                    Start server (api/worker/all)
├── server
│   └── start                Start server
├── dev                      Local development mode
│   └── status               Dev readiness checks
├── init                     Initialize project files
├── login                    Authenticate with API key
├── logout                   Remove stored API key
├── auth
│   └── status               Show auth status
├── context
│   ├── create               Create/update context
│   ├── use                  Set active context
│   ├── list                 List contexts
│   └── current              Show active context
├── alias
│   ├── set                  Set command alias
│   ├── list                 List aliases
│   └── delete               Delete alias
├── jobs
│   ├── list                 List jobs
│   ├── get                  Get job
│   ├── create               Create job
│   ├── trigger              Trigger job run
│   ├── trigger-bulk         Bulk trigger
│   ├── delete               Delete job
│   ├── versions             Version history
│   ├── describe             Rich job details
│   └── edit                 Edit job
├── runs
│   ├── list                 List runs
│   ├── get                  Get run
│   ├── cancel               Cancel run(s)
│   ├── logs                 Run logs
│   ├── watch                Watch run
│   └── replay               Replay run
├── workflows
│   ├── list                 List workflows
│   ├── get                  Get workflow
│   ├── describe             Describe workflow
│   ├── create               Create workflow
│   ├── update               Update workflow
│   ├── delete               Delete workflow
│   ├── runs                 Workflow runs
│   └── trigger              Trigger workflow
├── workflow-runs
│   ├── list                 List workflow runs
│   ├── get                  Get workflow run
│   ├── cancel               Cancel workflow run
│   ├── steps                List step runs
│   └── watch                Watch workflow run
├── api-keys
│   ├── create               Create API key
│   ├── list                 List API keys
│   ├── revoke               Revoke API key
│   └── rotate               Rotate API key
├── validate                 Validate manifests
├── apply                    Apply manifests
├── diff                     Diff manifests vs server
├── check                    Deep manifest validation
├── export                   Export server state as YAML
├── health                   Server health check
├── stats                    Queue statistics
├── verify                   Post-deploy verification
├── diagnose                 Troubleshooting diagnostics
│   └── run                  Diagnose specific run
├── top                      Live stats
│   ├── queue                Queue depth
│   └── jobs                 Job metrics
├── tui                      Interactive dashboard
├── listen                   Watch for new runs
├── drain                    Wait for runs to complete
├── trace                    ASCII run timeline
├── logs                     View run logs
├── events                   Inspect run events
├── trigger                  Shortcut: trigger job
├── api                      Raw API call
├── run                      Run command with context env
├── send                     Send raw event
├── wait
│   ├── run                  Wait for run condition
│   └── queue                Wait for queue condition
├── cleanup                  Remove old runs
├── migrate
│   ├── up                   Apply migrations
│   ├── down                 Rollback migrations
│   ├── status               Migration version
│   └── create               Create migration pair
├── backup
│   ├── create               Database backup
│   └── restore              Database restore
├── db
│   ├── shell                Open psql shell
│   └── stats                Database statistics
├── secrets
│   ├── create               Create/update secret
│   ├── list                 List secrets
│   └── delete               Delete secret
├── fixtures
│   ├── create               Create fixture data
│   └── clean                Clean fixture data
├── extension
│   ├── list                 List extensions
│   └── run                  Run extension
├── profile                  Capture pprof profile
├── version                  Version info
├── completion               Shell completions
├── docs                     Generate docs
└── upgrade                  Upgrade CLI
```
