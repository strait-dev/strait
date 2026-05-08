#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE="${GRAFANA_IMAGE:-grafana/grafana:12.1.1}"
PORT="${GRAFANA_PORT:-13000}"
CONTAINER="strait-grafana-smoke-${PORT}"
PROM_URL="${PROMETHEUS_URL:-http://prometheus:9090}"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

cleanup() {
  docker rm -f "$CONTAINER" >/dev/null 2>&1 || true
}

require docker
require curl
require jq

expected_count="$(find "$ROOT" -maxdepth 1 -name '*.json' | wc -l | tr -d ' ')"
if [[ "$expected_count" != "9" ]]; then
  echo "expected 9 dashboard JSON files, found $expected_count" >&2
  exit 1
fi

cleanup
trap cleanup EXIT

docker run --rm -d \
  --name "$CONTAINER" \
  -p "127.0.0.1:${PORT}:3000" \
  -e GF_AUTH_ANONYMOUS_ENABLED=true \
  -e GF_AUTH_ANONYMOUS_ORG_ROLE=Admin \
  -e GF_USERS_ALLOW_SIGN_UP=false \
  -e PROMETHEUS_URL="$PROM_URL" \
  -v "$ROOT:/var/lib/grafana/dashboards/strait:ro" \
  -v "$ROOT/provisioning:/etc/grafana/provisioning:ro" \
  "$IMAGE" >/dev/null

for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${PORT}/api/health" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

curl -fsS "http://127.0.0.1:${PORT}/api/health" >/dev/null
curl -fsS "http://127.0.0.1:${PORT}/api/datasources/uid/prometheus" | jq -e '.type == "prometheus"' >/dev/null

loaded_count="$(
  curl -fsS "http://127.0.0.1:${PORT}/api/search?query=Strait&type=dash-db" |
    jq '[.[] | select(.folderUid == "strait")] | length'
)"
if [[ "$loaded_count" != "$expected_count" ]]; then
  echo "loaded dashboard count = $loaded_count, want $expected_count" >&2
  curl -fsS "http://127.0.0.1:${PORT}/api/search?query=Strait&type=dash-db" | jq .
  exit 1
fi

uids="$(
  curl -fsS "http://127.0.0.1:${PORT}/api/search?query=Strait&type=dash-db" |
    jq -r '.[] | select(.folderUid == "strait") | .uid'
)"
for uid in $uids; do
  dashboard="$(curl -fsS "http://127.0.0.1:${PORT}/api/dashboards/uid/${uid}")"
  echo "$dashboard" |
    jq -e '
      (.dashboard.templating.list // [] | map(.name) | index("datasource")) and
      (.dashboard.templating.list // [] | map(.name) | index("interval"))
    ' >/dev/null
  echo "$dashboard" |
    jq -e '
      [
        .dashboard.panels[]? |
        select((.targets // []) | length > 0) |
        select(.datasource.uid != "${datasource}")
      ] | length == 0
    ' >/dev/null
  echo "$dashboard" |
    jq -e '
      [
        .dashboard.panels[]?.targets[]? |
        select((.expr // "") | test("\\[(5m|1h)\\]"))
      ] | length == 0
    ' >/dev/null
done

echo "Grafana smoke validation passed: ${loaded_count} dashboards loaded with datasource and interval variables."
