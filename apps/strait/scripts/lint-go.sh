#!/usr/bin/env bash
set -euo pipefail

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
cache_root="${GOLANGCI_LINT_CACHE:-$module_dir/.cache/golangci-lint}"

if [[ "$cache_root" != /* ]]; then
  echo "GOLANGCI_LINT_CACHE must be an absolute path" >&2
  exit 2
fi

cd "$module_dir"

if [[ "$#" -eq 0 ]]; then
  set -- ./...
fi

run_lint() {
  local name="$1"
  shift

  export GOLANGCI_LINT_CACHE="$cache_root/$name"
  mkdir -p "$GOLANGCI_LINT_CACHE"

  echo "==> golangci-lint: $name"
  golangci-lint run "$@"
}

run_lint_parallel() {
  local tmpdir
  tmpdir="$(mktemp -d "${TMPDIR:-/tmp}/strait-lint.XXXXXX")"

  local names=()
  local logs=()
  local pids=()
  local heartbeat_pid=""

  start_lint() {
    local name="$1"
    local log="$2"
    shift 2

    names+=("$name")
    logs+=("$log")

    (
      export GOLANGCI_LINT_CACHE="$cache_root/$name"
      mkdir -p "$GOLANGCI_LINT_CACHE"

      echo "==> golangci-lint: $name"
      golangci-lint run "$@"
    ) >"$log" 2>&1 &

    pids+=("$!")
  }

  start_lint community "$tmpdir/community.log" "$@"
  start_lint cloud "$tmpdir/cloud.log" --build-tags=cloud "$@"

  (
    while true; do
      sleep 30
      echo "==> golangci-lint: waiting for community and cloud checks..."
    done
  ) &
  heartbeat_pid="$!"

  local status=0
  local i
  for i in "${!pids[@]}"; do
    if ! wait "${pids[$i]}"; then
      status=1
    fi
  done

  kill "$heartbeat_pid" 2>/dev/null || true
  wait "$heartbeat_pid" 2>/dev/null || true

  for i in "${!logs[@]}"; do
    echo "==> golangci-lint: ${names[$i]} output"
    cat "${logs[$i]}"
  done

  rm -rf "$tmpdir"
  return "$status"
}

has_build_tags=false
for arg in "$@"; do
  case "$arg" in
    --build-tags | --build-tags=*)
      has_build_tags=true
      ;;
  esac
done

if [[ "$has_build_tags" == "true" ]]; then
  run_lint custom "$@"
  exit
fi

case "${STRAIT_LINT_EDITION:-all}" in
  all)
    if [[ "${STRAIT_LINT_PARALLEL:-1}" == "0" ]]; then
      run_lint community "$@"
      run_lint cloud --build-tags=cloud "$@"
    else
      run_lint_parallel "$@"
    fi
    ;;
  community)
    run_lint community "$@"
    ;;
  cloud)
    run_lint cloud --build-tags=cloud "$@"
    ;;
  *)
    echo "STRAIT_LINT_EDITION must be one of: all, community, cloud" >&2
    exit 2
    ;;
esac
