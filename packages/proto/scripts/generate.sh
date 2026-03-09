#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROTO_ROOT="$(dirname "$SCRIPT_DIR")"
REPO_ROOT="$(dirname "$(dirname "$PROTO_ROOT")")"

# ── Go ────────────────────────────────────────────────────────────────
GO_OUT="${REPO_ROOT}/apps/strait/internal/sandbox"
mkdir -p "$GO_OUT"

echo "Generating Go protobuf + gRPC stubs..."
protoc \
  --proto_path="$PROTO_ROOT" \
  --go_out="$GO_OUT" \
  --go_opt=module=strait/internal/sandbox \
  --go-grpc_out="$GO_OUT" \
  --go-grpc_opt=module=strait/internal/sandbox \
  "$PROTO_ROOT/sandbox/v1/sandbox.proto"

echo "Go stubs generated at: $GO_OUT"

# ── Elixir (optional – skipped if protoc-gen-elixir is not installed) ─
ELIXIR_OUT="${REPO_ROOT}/apps/forge/lib/proto"
if command -v protoc-gen-elixir &>/dev/null; then
  mkdir -p "$ELIXIR_OUT"
  echo "Generating Elixir protobuf stubs..."
  protoc \
    --proto_path="$PROTO_ROOT" \
    --elixir_out=plugins=grpc:"$ELIXIR_OUT" \
    "$PROTO_ROOT/sandbox/v1/sandbox.proto"
  echo "Elixir stubs generated at: $ELIXIR_OUT"
else
  echo "Skipping Elixir generation (protoc-gen-elixir not found)"
fi

echo "Done."
