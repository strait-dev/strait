#!/usr/bin/env bash
set -euo pipefail

required_vars=(
  STRAIT_API_URL
  STRAIT_API_KEY
  PROJECT_ID
)

for var in "${required_vars[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "missing required env var: $var" >&2
    exit 1
  fi
done

base_url="${STRAIT_API_URL%/}"

create_response="$(
  curl -sS "$base_url/v1/agents" \
    -H "Authorization: Bearer $STRAIT_API_KEY" \
    -H "Content-Type: application/json" \
    -d '{
      "project_id":"'"$PROJECT_ID"'",
      "name":"Cloudflare Smoke Agent",
      "slug":"cloudflare-smoke-agent",
      "model":"gpt-5.4",
      "config":{
        "sandbox":{
          "policy":{
            "allow_hosts":["api.openai.com"],
            "default_action":"deny",
            "network_class":"restricted",
            "policy_tag":"smoke"
          }
        }
      }
    }'
)"

agent_id="$(printf '%s' "$create_response" | jq -r '.id')"
if [[ -z "$agent_id" || "$agent_id" == "null" ]]; then
  echo "failed to create smoke agent" >&2
  printf '%s\n' "$create_response" >&2
  exit 1
fi

echo "created agent: $agent_id"

curl -sS -X POST "$base_url/v1/agents/$agent_id/deploy" \
  -H "Authorization: Bearer $STRAIT_API_KEY" >/dev/null
echo "deployed agent"

happy_run="$(
  curl -sS -X POST "$base_url/v1/agents/$agent_id/run" \
    -H "Authorization: Bearer $STRAIT_API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"payload":{"prompt":"hello from cloudflare"}}'
)"
echo "happy-path run: $(printf '%s' "$happy_run" | jq -r '.id')"

blocked_run="$(
  curl -sS -X POST "$base_url/v1/agents/$agent_id/run" \
    -H "Authorization: Bearer $STRAIT_API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"payload":{"_network_url":"https://blocked.example.com"}}'
)"
echo "blocked-egress run: $(printf '%s' "$blocked_run" | jq -r '.id')"

echo "smoke script submitted both runs; inspect run details for usage, checkpoints, and blocked egress telemetry"
