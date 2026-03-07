# Authentication

The orchestrator supports two authentication mechanisms for API endpoints:

### Internal Secret

Set `INTERNAL_SECRET` and pass it as a bearer token:

```
Authorization: Bearer your-secret-here
```

### Per-Project API Keys

Create scoped API keys via the `/v1/api-keys` endpoint. The raw key is returned once on creation. Use it the same way:

```
Authorization: Bearer sk_live_abc123...
```

API keys are SHA-256 hashed at rest, scoped to a single project, and track last used timestamps. Revoke keys via `DELETE /v1/api-keys/{keyID}`.

The server auto-detects which auth method is being used by checking the token against the internal secret first, then looking up API keys by hash.

See [API Reference](api-reference.md) for the API keys endpoints.

### Run Tokens (SDK)

When a job run is triggered, the response includes a `run_token` — a short-lived JWT scoped to that specific run. Your job endpoint uses this token to authenticate SDK calls:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```
