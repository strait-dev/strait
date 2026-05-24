#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
METRICS_URL="${METRICS_URL:-http://127.0.0.1:8080/metrics}"
STRICT="${STRICT:-0}"

require() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

normalize_metric() {
  sed -E 's/_(bucket|sum)$//'
}

require curl
require rg
require sort
require comm

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

rg -o --no-filename 'strait_[a-z0-9_]+' "$ROOT/grafana" "$ROOT/prometheus-rules.yaml" |
  sort -u >"$tmpdir/referenced.raw"

normalize_metric <"$tmpdir/referenced.raw" | sort -u >"$tmpdir/referenced"

curl -fsS "$METRICS_URL" |
  awk '
    /^# (HELP|TYPE) / { print $3; next }
    /^[a-zA-Z_:][a-zA-Z0-9_:]*(\{|[[:space:]]|$)/ {
      name=$1
      sub(/\{.*/, "", name)
      print name
    }
  ' |
  sort -u >"$tmpdir/scraped.raw"

{
  cat "$tmpdir/scraped.raw"
  normalize_metric <"$tmpdir/scraped.raw"
  sed -E 's/_total$//' "$tmpdir/scraped.raw"
} | sort -u >"$tmpdir/scraped"

missing="$(comm -23 "$tmpdir/referenced" "$tmpdir/scraped" || true)"
referenced_count="$(wc -l <"$tmpdir/referenced" | tr -d ' ')"
scraped_count="$(wc -l <"$tmpdir/scraped" | tr -d ' ')"

if [[ -n "$missing" ]]; then
  echo "Monitoring references with no matching scraped metric from ${METRICS_URL}:"
  echo "$missing"
  echo
  echo "Referenced metrics: ${referenced_count}; scraped metrics: ${scraped_count}."
  if [[ "$STRICT" == "1" ]]; then
    exit 1
  fi
  echo "Set STRICT=1 to fail on missing references."
  exit 0
fi

echo "Scrape coverage passed: ${referenced_count} monitoring references matched ${scraped_count} scraped metrics."
