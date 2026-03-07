# CDC (Change Data Capture)

The orchestrator includes an optional CDC consumer that integrates with [Sequin Stream](https://sequinstream.com) to capture real-time database changes from Postgres WAL.

### How It Works

1. Sequin reads your Postgres WAL (logical replication) and buffers changes
2. The orchestrator's CDC consumer polls Sequin's HTTP API for batches of change events
3. Events are routed to table-specific handlers based on the table name
4. Handlers publish structured change events to Redis pub/sub channels
5. Processed events are acknowledged; failed events are nacked for redelivery

### Monitored Tables

| Table | Pub/Sub Channel |
|-------|----------------|
| `job_runs` | `cdc:project:{project_id}:job_runs` |
| `workflow_runs` | `cdc:project:{project_id}:workflow_runs` |
| `workflow_step_runs` | `cdc:workflow_run:{workflow_run_id}:steps` |

### Setup

1. Deploy Sequin and connect it to your Postgres database (see [Sequin docs](https://sequinstream.com/docs))
2. Create a Sequin Stream sink for the tables you want to monitor
3. Set the `SEQUIN_BASE_URL`, `SEQUIN_CONSUMER_NAME`, and `SEQUIN_API_TOKEN` environment variables
4. The CDC consumer starts automatically alongside the API/worker

CDC is disabled when `SEQUIN_BASE_URL` is not set.

Sequin is included in the development docker-compose.yml. See [Configuration](configuration.md) for Sequin environment variables.
