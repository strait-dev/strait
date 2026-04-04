#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
INFRA_DIR="$(dirname "$SCRIPT_DIR")"
TF_DIR="$INFRA_DIR/terraform"
K8S_DIR="$INFRA_DIR/../apps/strait/k8s"

echo "=== Strait Infrastructure Setup ==="
echo ""

# Check prerequisites.
for cmd in terraform ssh scp kubectl; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Error: $cmd is required but not installed."
    exit 1
  fi
done

# Resolve configuration: Doppler env vars > terraform.tfvars > error.
# Doppler injects env vars when run via: doppler run -- make infra-up
if [ -n "${HCLOUD_TOKEN:-}" ]; then
  echo "Using credentials from environment (Doppler)."

  # Write a temporary tfvars from Doppler env vars.
  cat > "$TF_DIR/terraform.tfvars" <<TFVARS
hcloud_token         = "${HCLOUD_TOKEN}"
ssh_key_path         = "${SSH_KEY_PATH:-~/.ssh/id_ed25519.pub}"
ssh_private_key_path = "${SSH_PRIVATE_KEY_PATH:-~/.ssh/id_ed25519}"
location             = "${HETZNER_LOCATION:-fsn1}"
general_count        = ${HETZNER_GENERAL_COUNT:-1}
perf_count           = ${HETZNER_PERF_COUNT:-1}
heavy_count          = ${HETZNER_HEAVY_COUNT:-0}
TFVARS

elif [ -f "$TF_DIR/terraform.tfvars" ]; then
  echo "Using credentials from terraform.tfvars."
else
  echo "Error: No credentials found."
  echo ""
  echo "Option 1 (recommended): Use Doppler"
  echo "  doppler secrets set HCLOUD_TOKEN --project strait --config dev"
  echo "  doppler run --project strait --config dev -- make infra-up"
  echo ""
  echo "Option 2: Use terraform.tfvars"
  echo "  cp terraform/terraform.tfvars.example terraform/terraform.tfvars"
  echo "  # Fill in your Hetzner token and SSH key paths"
  echo "  make infra-up"
  exit 1
fi

# Terraform init + apply.
echo ""
echo "Provisioning Hetzner servers..."
cd "$TF_DIR"
terraform init -input=false
terraform apply -auto-approve

# Get master IP and SSH key.
MASTER_IP=$(terraform output -raw master_ip)
SSH_KEY="${SSH_PRIVATE_KEY_PATH:-~/.ssh/id_ed25519}"
SSH_KEY=$(eval echo "$SSH_KEY")

echo ""
echo "Waiting for k3s to be ready..."
sleep 30

# Retry until kubectl works on the master.
for i in $(seq 1 30); do
  if ssh -o StrictHostKeyChecking=no -o ConnectTimeout=5 -i "$SSH_KEY" "root@$MASTER_IP" "kubectl get nodes" &>/dev/null; then
    break
  fi
  echo "  Waiting for k3s... ($i/30)"
  sleep 10
done

# Fetch kubeconfig.
echo "Fetching kubeconfig..."
scp -o StrictHostKeyChecking=no -i "$SSH_KEY" "root@$MASTER_IP:/etc/rancher/k3s/k3s.yaml" "$INFRA_DIR/kubeconfig"

# Replace localhost with public IP so kubectl works remotely.
if [[ "$OSTYPE" == "darwin"* ]]; then
  sed -i '' "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig"
else
  sed -i "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig"
fi

export KUBECONFIG="$INFRA_DIR/kubeconfig"

# Wait for all nodes to be Ready.
echo "Waiting for all nodes to be Ready..."
for i in $(seq 1 30); do
  NOT_READY=$(kubectl get nodes --no-headers 2>/dev/null | grep -cv "Ready" || echo "999")
  if [ "$NOT_READY" = "0" ]; then
    break
  fi
  echo "  Nodes not ready yet... ($i/30)"
  sleep 10
done

# Apply K8s manifests.
echo "Applying K8s manifests..."
kubectl apply -f "$K8S_DIR/priority-classes.yaml"
kubectl apply -f "$K8S_DIR/service-account.yaml"
kubectl apply -f "$K8S_DIR/resource-quota.yaml"
[ -f "$K8S_DIR/network-policy.yaml" ] && kubectl apply -f "$K8S_DIR/network-policy.yaml"

echo ""
echo "=== Setup Complete ==="
echo ""
kubectl get nodes -o wide
echo ""
echo "Kubeconfig: $INFRA_DIR/kubeconfig"
echo ""
echo "Connect Strait:"
echo "  export COMPUTE_RUNTIME=k8s"
echo "  export K8S_KUBECONFIG=$INFRA_DIR/kubeconfig"
echo "  export K8S_NAMESPACE=default"
echo ""
echo "Or with Doppler:"
echo "  doppler run -- make dev"
