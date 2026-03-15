# strait

Ruby SDK for the Strait platform API with full feature parity across all five Strait SDKs.

## Install

```bash
gem install strait
```

Or add to your Gemfile:

```ruby
gem "strait"
```

## Quick Start

```ruby
require "strait"

client = Strait::Client.new(
  base_url: "https://api.strait.dev",
  bearer_token: "sk_live_..."
)

run = client.jobs.trigger("job_abc", { payload: { sku: "ABC-123" } })
puts "Run ID: #{run['id']}"
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

```ruby
# Reads strait.json from working directory + STRAIT_API_KEY from env
client = Strait::Client.from_file

# Or specify a custom directory
client = Strait::Client.from_file(dir: "/path/to/project")

# Or an explicit file path
client = Strait::Client.from_file(path: "/path/to/custom-config.json")

# Apply additional client options on top
client = Strait::Client.from_file(middleware: my_middleware)
```

The SDK reads the `sdk` section from the file. Auth tokens are **never** read from the file — they always come from the `STRAIT_API_KEY` environment variable.

You can also read just the config without creating a client:

```ruby
cfg = Strait.config_from_file
```

Or extract the project ID:

```ruby
project_id = Strait.project_id_from_file
```

### From environment variables

```ruby
client = Strait::Client.from_env
# Reads STRAIT_BASE_URL, STRAIT_API_KEY, STRAIT_AUTH_TYPE, STRAIT_TIMEOUT_MS
```

### Inline

```ruby
client = Strait::Client.new(
  base_url: "https://api.strait.dev",
  api_key: "sk_live_...",
  timeout_ms: 5000
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
| `base_url:` | API base URL (trailing slashes stripped) |
| `bearer_token:` | Bearer token auth |
| `api_key:` | API key auth |
| `run_token:` | Run token auth |
| `auth:` | Set auth mode directly |
| `default_headers:` | Headers sent with every request |
| `timeout_ms:` | Timeout in milliseconds (default: 30000) |
| `http_client:` | Custom HTTP client (any object responding to `#call`) |
| `middleware:` | Request/response/error hooks |

## Authoring DSL

```ruby
require "strait/authoring"

job = Strait::Authoring.define_job(
  name: "Sync Inventory",
  slug: "sync-inventory",
  endpoint_url: "https://worker.dev/jobs/sync",
  project_id: "proj_1"
) do |payload, ctx|
  sync_inventory(payload[:sku])
end

# Register and trigger
job.register(client)
job.trigger(client, payload: { sku: "ABC-123" })
```

## Workflow DAG

```ruby
wf = Strait::Authoring.define_workflow(
  name: "Order Pipeline",
  slug: "order-pipeline",
  project_id: "proj_1",
  steps: [
    Strait::Authoring.job("validate", "job_validate"),
    Strait::Authoring.job("charge", "job_charge", depends_on: ["validate"]),
    Strait::Authoring.approval("review", depends_on: ["charge"]),
  ]
)
```

## Composition Helpers

```ruby
require "strait/composition"

# Retry with backoff
result = Strait::Composition.with_retry(attempts: 5, delay_ms: 100) do
  call_api
end

# Paginate
Strait::Composition.paginate(method(:list_fn)).each do |item|
  # process item
end

# Wait for run
run = Strait::Composition.wait_for_run(
  method(:get_run), method(:get_status), "run_123"
)
```

## FSM State Machines

```ruby
require "strait/fsm"

Strait::FSM.can_transition_run?("executing", "COMPLETE")  # true
Strait::FSM.terminal_run_status?("completed")              # true
```

## Middleware

```ruby
client = Strait::Client.new(
  base_url: "https://api.strait.dev",
  bearer_token: "sk_live_...",
  middleware: Strait::Middleware.new(
    on_request: ->(ctx) { puts "#{ctx.method} #{ctx.url}" },
    on_response: ->(ctx) { puts "#{ctx.status} #{ctx.duration_ms}ms" }
  )
)
```

## Custom HTTP Client

Any object responding to `#call(request)` can replace the default `Net::HTTP` client:

```ruby
custom_client = ->(request) {
  Faraday.send(request.method, request.url, request.body, request.headers)
}

client = Strait::Client.new(
  base_url: "https://api.strait.dev",
  api_key: "sk_live_...",
  http_client: custom_client
)
```

## Error Handling

All errors inherit from `Strait::Error`. Use `rescue` to match specific error kinds:

```ruby
begin
  result = jobs.get("nonexistent")
rescue Strait::NotFoundError => e
  puts "Not found: #{e.message}"
rescue Strait::UnauthorizedError => e
  puts "Auth error: #{e.message}"
rescue Strait::RateLimitedError => e
  puts "Rate limited: #{e.message}"
rescue Strait::Error => e
  puts "Error: #{e.message}"
end
```

| Error type | HTTP status | Description |
|---|---|---|
| `Strait::TransportError` | — | Network/transport failure |
| `Strait::DecodeError` | — | JSON decode failure |
| `Strait::ValidationError` | — | Config or input validation |
| `Strait::UnauthorizedError` | 401, 403 | Authentication failure |
| `Strait::NotFoundError` | 404 | Resource not found |
| `Strait::ConflictError` | 409 | Conflict (duplicate, etc.) |
| `Strait::RateLimitedError` | 429 | Rate limit exceeded |
| `Strait::ApiError` | other | Generic HTTP error |
| `Strait::TimeoutError` | — | Polling timeout |
| `Strait::DagValidationError` | — | Workflow DAG is invalid |

## Development

```bash
bundle exec rspec
```
