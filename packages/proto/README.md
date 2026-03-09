# @strait/proto

Shared Protocol Buffer definitions for communication between Strait services.

## Structure

```
proto/
└── sandbox/
    └── v1/
        └── sandbox.proto    # Forge sandbox execution service
```

## Services

### `sandbox.v1.SandboxExecutor`

Used by Strait (Go orchestrator) to dispatch code execution to Forge (Elixir sandbox service).

| RPC | Type | Description |
|-----|------|-------------|
| `Execute` | server-streaming | Run code in sandbox, stream events back |

See [`sandbox/v1/sandbox.proto`](sandbox/v1/sandbox.proto) for full message definitions.

## Code Generation

### Prerequisites

- [protoc](https://grpc.io/docs/protoc-installation/)
- Go plugins: `protoc-gen-go`, `protoc-gen-go-grpc`
- Elixir plugin: `protoc-gen-elixir` (via `protobuf` hex package)

### Generate

From the repo root:

```bash
bun run proto:gen
```

Or directly:

```bash
cd packages/proto && bash scripts/generate.sh
```

### Output

| Language | Output path | Package |
|----------|-------------|---------|
| Go | `apps/strait/internal/sandbox/v1/` | `sandboxv1` |
| Elixir | `apps/forge/lib/proto/` | `Sandbox.V1.*` |

## Adding a New Service

1. Create a new directory under `packages/proto/` (e.g. `packages/proto/myservice/v1/`)
2. Add your `.proto` file
3. Update `scripts/generate.sh` to include the new proto
4. Run `bun run proto:gen`
