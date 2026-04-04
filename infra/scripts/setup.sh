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

# Check tfvars exists.
if [ ! -f "$TF_DIR/terraform.tfvars" ]; then
  echo "Error: terraform.tfvars not found."
  echo "  cp $TF_DIR/terraform.tfvars.example $TF_DIR/terraform.tfvars"
  echo "  Then fill in your Hetzner token and SSH key paths."
  exit 1
fi

# Terraform init + apply.
echo "Provisioning Hetzner servers..."
cd "$TF_DIR"
terraform init -input=false
terraform apply -auto-approve

# Get master IP.
MASTER_IP=$(terraform output -raw master_ip)
SSH_KEY=$(terraform output -raw ssh_command | grep -oP '(?<=-i )\S+' || echo "~/.ssh/id_ed25519")

echo ""
echo "Waiting for k3s to be ready..."
sleep 30

# Retry until kubectl works.
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
sed -i '' "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig" 2>/dev/null || sed -i "s/127.0.0.1/$MASTER_IP/g" "$INFRA_DIR/kubeconfig"

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
if [ -f "$K8S_DIR/network-policy.yaml" ]; then
  kubectl apply -f "$K8S_DIR/network-policy.yaml"
fi

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
