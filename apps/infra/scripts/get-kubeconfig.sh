#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INFRA_DIR="$(dirname "$SCRIPT_DIR")"
TF_DIR="$INFRA_DIR/terraform"

cd "$TF_DIR"
MASTER_IP=$(terraform output -raw master_ip 2>/dev/null)

if [ -z "$MASTER_IP" ]; then
  echo "Error: Could not get master IP. Is the cluster running?"
  echo "  Run: make infra-up"
  exit 1
fi

# Determine SSH key path from terraform output.
SSH_KEY_PATH=$(terraform output -raw ssh_command 2>/dev/null | grep -oP '(?<=-i )\S+' || echo "~/.ssh/id_ed25519")

echo "Fetching kubeconfig from $MASTER_IP..."
scp -o StrictHostKeyChecking=no -i "$SSH_KEY_PATH" "root@$MASTER_IP:/etc/rancher/k3s/k3s.yaml" "$INFRA_DIR/kubeconfig"

# Replace localhost with the public IP so kubectl works remotely.
sed -i '' "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig" 2>/dev/null || sed -i "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig"

echo "Kubeconfig saved to: $INFRA_DIR/kubeconfig"
echo ""
echo "Test with:"
echo "  kubectl --kubeconfig $INFRA_DIR/kubeconfig get nodes"
