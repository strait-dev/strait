#!/usr/bin/env bash
set -euo pipefail

# Import Grafana alert rules from grafana-alerts.json.
#
# Usage: ./import-alerts.sh
#
# Required environment variables:
#   GRAFANA_URL       — Grafana Cloud instance URL (e.g., https://strait.grafana.net)
#   GRAFANA_API_KEY   — Grafana Cloud API key with Alerting Admin permissions
#
# The alert rules are defined in apps/infra/k8s/monitoring/grafana-alerts.json.

GRAFANA_URL="${GRAFANA_URL:?GRAFANA_URL is required}"
GRAFANA_API_KEY="${GRAFANA_API_KEY:?GRAFANA_API_KEY is required}"
ALERTS_FILE="$(dirname "$0")/../k8s/monitoring/grafana-alerts.json"

if [ ! -f "$ALERTS_FILE" ]; then
  echo "Error: $ALERTS_FILE not found"
  exit 1
fi

echo "Importing alert rules from $ALERTS_FILE..."

# Grafana uses the Prometheus-compatible alerting API.
RESPONSE=$(curl -sf -X POST "${GRAFANA_URL}/api/v1/provisioning/alert-rules" \
  -H "Authorization: Bearer ${GRAFANA_API_KEY}" \
  -H "Content-Type: application/json" \
  -d @"$ALERTS_FILE" 2>&1) || {
  echo "Error importing alerts: $RESPONSE"
  echo ""
  echo "If the provisioning API is not available, import manually:"
  echo "  1. Go to ${GRAFANA_URL}/alerting/list"
  echo "  2. Click 'Import' and paste the contents of $ALERTS_FILE"
  exit 1
}

echo "Alert rules imported successfully."
echo "Verify at: ${GRAFANA_URL}/alerting/list"
