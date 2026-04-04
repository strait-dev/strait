#!/usr/bin/env bash
set -euo pipefail

# Run all Litmus chaos experiments sequentially.
#
# Prerequisites:
#   1. Install Litmus:
#      kubectl apply -f https://litmuschaos.github.io/litmus/litmus-operator-v3.0.0.yaml
#   2. Create litmus-admin ServiceAccount:
#      kubectl -n strait create sa litmus-admin
#      kubectl create clusterrolebinding litmus-admin --clusterrole=cluster-admin --serviceaccount=strait:litmus-admin
#
# Usage: ./run-chaos.sh

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
KUBECONFIG="${KUBECONFIG:-$SCRIPT_DIR/../../terraform/kubeconfig}"
export KUBECONFIG

echo "=== Strait Chaos Testing ==="
echo ""

run_experiment() {
  local name="$1"
  local file="$2"
  echo "--- Running: $name ---"
  kubectl apply -f "$file"

  # Wait for experiment to complete (max 5 minutes).
  local timeout=300
  local elapsed=0
  while [ $elapsed -lt $timeout ]; do
    status=$(kubectl -n strait get chaosengine "$name" -o jsonpath='{.status.engineStatus}' 2>/dev/null || echo "pending")
    if [ "$status" = "completed" ] || [ "$status" = "stopped" ]; then
      break
    fi
    sleep 10
    elapsed=$((elapsed + 10))
  done

  # Check result.
  verdict=$(kubectl -n strait get chaosresult "${name}-${name}" -o jsonpath='{.status.experimentStatus.verdict}' 2>/dev/null || echo "unknown")
  if [ "$verdict" = "Pass" ]; then
    echo "  PASS"
  else
    echo "  FAIL (verdict: $verdict)"
  fi

  # Cleanup.
  kubectl delete -f "$file" --ignore-not-found 2>/dev/null
  echo ""
}

run_experiment "strait-pod-kill" "$SCRIPT_DIR/pod-kill.yaml"
run_experiment "strait-network-loss" "$SCRIPT_DIR/network-loss.yaml"
run_experiment "strait-cpu-stress" "$SCRIPT_DIR/cpu-stress.yaml"
run_experiment "strait-node-drain" "$SCRIPT_DIR/node-drain.yaml"

echo "=== Chaos testing complete ==="
