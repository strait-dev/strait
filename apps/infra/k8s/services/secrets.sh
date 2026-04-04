#!/usr/bin/env bash
set -euo pipefail

# Provision K8s secrets from Doppler for all Strait services.
# Usage: ./secrets.sh [env]
#   env: dev, stg, or prd (default: dev)
#
# Prerequisites:
#   - doppler CLI authenticated
#   - kubectl configured with cluster kubeconfig

ENV="${1:-dev}"
NAMESPACE="strait"
KUBECONFIG="${KUBECONFIG:-$(dirname "$0")/../../terraform/kubeconfig}"

echo "Provisioning secrets from Doppler (config: $ENV) into namespace $NAMESPACE..."

# Ensure namespace exists.
kubectl --kubeconfig "$KUBECONFIG" create namespace "$NAMESPACE" 2>/dev/null || true

# Strait server secrets.
echo "Creating strait-env secret..."
doppler secrets download --project strait --config "$ENV" --no-file --format env-no-quotes \
  | kubectl --kubeconfig "$KUBECONFIG" create secret generic strait-env \
      --namespace "$NAMESPACE" \
      --from-env-file=/dev/stdin \
      --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KUBECONFIG" apply -f -

# Sequin secrets (uses the same Doppler project, filter relevant keys).
echo "Creating sequin-env secret..."
doppler secrets download --project strait --config "$ENV" --no-file --format env-no-quotes \
  | grep -E "^(DATABASE_URL|REDIS_URL|SEQUIN_|SECRET_KEY_BASE)" \
  | kubectl --kubeconfig "$KUBECONFIG" create secret generic sequin-env \
      --namespace "$NAMESPACE" \
      --from-env-file=/dev/stdin \
      --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KUBECONFIG" apply -f -

# OTel collector secrets.
echo "Creating otel-env secret..."
doppler secrets download --project strait --config "$ENV" --no-file --format env-no-quotes \
  | grep -E "^(CLICKHOUSE_|GRAFANA_)" \
  | kubectl --kubeconfig "$KUBECONFIG" create secret generic otel-env \
      --namespace "$NAMESPACE" \
      --from-env-file=/dev/stdin \
      --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KUBECONFIG" apply -f -

# GHCR image pull secret (for private strait images).
GITHUB_USER=$(doppler secrets get GITHUB_USERNAME --project strait --config "$ENV" --plain 2>/dev/null || true)
GITHUB_TOKEN=$(doppler secrets get GITHUB_PACKAGES_TOKEN --project strait --config "$ENV" --plain 2>/dev/null || true)
if [ -n "$GITHUB_USER" ] && [ -n "$GITHUB_TOKEN" ]; then
  echo "Creating ghcr-pull-secret..."
  kubectl --kubeconfig "$KUBECONFIG" create secret docker-registry ghcr-pull-secret \
    --namespace "$NAMESPACE" \
    --docker-server=ghcr.io \
    --docker-username="$GITHUB_USER" \
    --docker-password="$GITHUB_TOKEN" \
    --dry-run=client -o yaml \
  | kubectl --kubeconfig "$KUBECONFIG" apply -f -
else
  echo "Skipping ghcr-pull-secret (GITHUB_USERNAME/GITHUB_PACKAGES_TOKEN not set in Doppler)"
fi

echo "All secrets provisioned for environment: $ENV"
