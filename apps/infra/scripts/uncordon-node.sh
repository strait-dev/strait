#!/usr/bin/env bash
set -euo pipefail

if [ -z "${1:-}" ]; then
  echo "Usage: $0 <node-name>"
  echo "  Uncordons a node to resume pod scheduling."
  exit 1
fi

NODE="$1"
KUBECONFIG="${KUBECONFIG:-$(dirname "$0")/../kubeconfig}"

kubectl --kubeconfig "$KUBECONFIG" uncordon "$NODE"
echo "Node $NODE uncordoned. Scheduling resumed."
