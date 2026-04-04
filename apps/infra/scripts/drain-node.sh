#!/usr/bin/env bash
set -euo pipefail

if [ -z "${1:-}" ]; then
  echo "Usage: $0 <node-name>"
  echo "  Drains and cordons a node for maintenance."
  echo "  Running pods are gracefully evicted (120s timeout)."
  echo ""
  echo "Example: $0 strait-general-1"
  exit 1
fi

NODE="$1"
KUBECONFIG="${KUBECONFIG:-$(dirname "$0")/../kubeconfig}"

echo "Draining node $NODE..."
kubectl --kubeconfig "$KUBECONFIG" drain "$NODE" \
  --ignore-daemonsets \
  --delete-emptydir-data \
  --timeout=120s \
  --force

echo ""
echo "Node $NODE drained and cordoned."
echo "To resume scheduling: $0/../uncordon-node.sh $NODE"
