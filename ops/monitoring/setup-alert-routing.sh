#!/usr/bin/env bash
set -euo pipefail

# Configure Grafana Cloud alert routing to Better Stack.
#
# Prerequisites:
#   - Better Stack paid plan with Grafana webhook integration
#   - Grafana Cloud service account token with Admin role
#
# Usage:
#   BETTERSTACK_WEBHOOK_URL="https://uptime.betterstack.com/api/v1/incoming-webhook/XXXX" \
#   GRAFANA_SERVICE_ACCOUNT_TOKEN="glsa_..." \
#   ./ops/monitoring/setup-alert-routing.sh

GRAFANA_URL="${GRAFANA_URL:-https://strait.grafana.net}"
GRAFANA_TOKEN="${GRAFANA_SERVICE_ACCOUNT_TOKEN:?GRAFANA_SERVICE_ACCOUNT_TOKEN is required}"
WEBHOOK_URL="${BETTERSTACK_WEBHOOK_URL:?BETTERSTACK_WEBHOOK_URL is required}"

echo "Creating Better Stack contact point in Grafana Cloud..."
RESULT=$(curl -s -w "\n%{http_code}" -X POST "$GRAFANA_URL/api/v1/provisioning/contact-points" \
  -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"Better Stack\",\"type\":\"webhook\",\"settings\":{\"url\":\"$WEBHOOK_URL\",\"httpMethod\":\"POST\"}}")

HTTP_CODE=$(echo "$RESULT" | tail -1)
BODY=$(echo "$RESULT" | head -1)

if [ "$HTTP_CODE" = "202" ] || [ "$HTTP_CODE" = "200" ]; then
  echo "Contact point created successfully."
else
  echo "Failed to create contact point (HTTP $HTTP_CODE): $BODY"
  exit 1
fi

echo "Updating default notification policy to route to Better Stack..."
RESULT=$(curl -s -w "\n%{http_code}" -X PUT "$GRAFANA_URL/api/v1/provisioning/policies" \
  -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"receiver":"Better Stack","group_by":["alertname"],"group_wait":"30s","group_interval":"5m","repeat_interval":"4h"}')

HTTP_CODE=$(echo "$RESULT" | tail -1)
BODY=$(echo "$RESULT" | head -1)

if [ "$HTTP_CODE" = "202" ] || [ "$HTTP_CODE" = "200" ]; then
  echo "Notification policy updated successfully."
else
  echo "Failed to update notification policy (HTTP $HTTP_CODE): $BODY"
  exit 1
fi

echo "Alert routing configured. All Grafana alerts will now route to Better Stack."
