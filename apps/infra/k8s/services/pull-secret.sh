#!/usr/bin/env bash
set -euo pipefail

# Create GHCR image pull secret for the strait namespace.
# The strait deployment needs this to pull private images from ghcr.io.
#
# Usage: ./pull-secret.sh
#
# Required environment variables:
#   GITHUB_USERNAME         — GitHub username or org (e.g., strait-dev)
#   GITHUB_PACKAGES_TOKEN   — GitHub PAT with read:packages scope
#
# Or pass via Doppler:
#   doppler run -- ./pull-secret.sh

NAMESPACE="strait"
KUBECONFIG="${KUBECONFIG:-$(dirname "$0")/../../terraform/kubeconfig}"
USERNAME="${GITHUB_USERNAME:?GITHUB_USERNAME is required}"
TOKEN="${GITHUB_PACKAGES_TOKEN:?GITHUB_PACKAGES_TOKEN is required}"

echo "Creating GHCR pull secret in namespace $NAMESPACE..."

kubectl --kubeconfig "$KUBECONFIG" create secret docker-registry ghcr-pull-secret \
  --namespace "$NAMESPACE" \
  --docker-server=ghcr.io \
  --docker-username="$USERNAME" \
  --docker-password="$TOKEN" \
  --dry-run=client -o yaml \
| kubectl --kubeconfig "$KUBECONFIG" apply -f -

echo "Pull secret created: ghcr-pull-secret"
