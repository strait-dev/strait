#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TF_DIR="$(dirname "$SCRIPT_DIR")/terraform"
INFRA_DIR="$(dirname "$SCRIPT_DIR")"

echo "This will DESTROY all Hetzner servers and the k3s cluster."
echo "All data on the servers will be permanently lost."
echo ""
read -p "Type 'destroy' to confirm: " CONFIRM

if [ "$CONFIRM" != "destroy" ]; then
  echo "Aborted."
  exit 1
fi

cd "$TF_DIR"
terraform destroy -auto-approve

# Clean up local files.
rm -f "$INFRA_DIR/kubeconfig"

echo ""
echo "Infrastructure destroyed."
