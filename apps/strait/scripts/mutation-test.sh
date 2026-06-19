#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."

usage() {
  cat <<'USAGE'
Usage:
  ./scripts/mutation-test.sh ./internal/errors [./internal/otherpkg]

Runs Gremlins against explicit Go packages only. Broad package patterns such as
./... and any path containing ... are refused to avoid large memory spikes.

Environment:
  GREMLINS_VERSION              Gremlins version to run. Default: v0.6.0
  GREMLINS_WORKERS              Concurrent mutation workers. Default: 1
  GREMLINS_TEST_CPU             CPUs available to each go test process. Default: 1
  GREMLINS_TIMEOUT_COEFFICIENT  Test timeout multiplier. Default: 60
  GREMLINS_OUTPUT_DIR           JSON output directory. Default: .cache/gremlins
  GREMLINS_OUTPUT_LABEL         Optional output filename suffix for package slices.
  GREMLINS_OUTPUT_STATUSES      Optional stdout status filter, such as lctv.
  GREMLINS_EXCLUDE_FILES        Optional comma-separated filepath regexes to exclude.
  GREMLINS_DRY_RUN              Set to 1 to discover mutants without testing.
  GREMLINS_TAGS                 Optional comma-separated Go build tags.
USAGE
}

if [[ $# -eq 0 ]]; then
  usage >&2
  exit 2
fi

gremlins_version="${GREMLINS_VERSION:-v0.6.0}"
workers="${GREMLINS_WORKERS:-1}"
test_cpu="${GREMLINS_TEST_CPU:-1}"
timeout_coefficient="${GREMLINS_TIMEOUT_COEFFICIENT:-60}"
output_dir="${GREMLINS_OUTPUT_DIR:-.cache/gremlins}"
output_label="${GREMLINS_OUTPUT_LABEL:-}"
output_statuses="${GREMLINS_OUTPUT_STATUSES:-}"
exclude_files="${GREMLINS_EXCLUDE_FILES:-}"
dry_run="${GREMLINS_DRY_RUN:-0}"
tags="${GREMLINS_TAGS:-}"

case "$workers" in
  ''|*[!0-9]*)
    echo "GREMLINS_WORKERS must be a positive integer" >&2
    exit 2
    ;;
esac
if (( workers < 1 || workers > 4 )); then
  echo "GREMLINS_WORKERS must be between 1 and 4 for local mutation runs" >&2
  exit 2
fi

case "$test_cpu" in
  ''|*[!0-9]*)
    echo "GREMLINS_TEST_CPU must be a positive integer" >&2
    exit 2
    ;;
esac
if (( test_cpu < 1 || test_cpu > 4 )); then
  echo "GREMLINS_TEST_CPU must be between 1 and 4 for local mutation runs" >&2
  exit 2
fi

case "$timeout_coefficient" in
  ''|*[!0-9]*)
    echo "GREMLINS_TIMEOUT_COEFFICIENT must be a positive integer" >&2
    exit 2
    ;;
esac
if (( timeout_coefficient < 1 )); then
  echo "GREMLINS_TIMEOUT_COEFFICIENT must be at least 1" >&2
  exit 2
fi

if [[ -n "$output_label" && ! "$output_label" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  echo "GREMLINS_OUTPUT_LABEL may only contain letters, numbers, dots, underscores, and hyphens" >&2
  exit 2
fi

exclude_args=()
if [[ -n "$exclude_files" ]]; then
  IFS=',' read -r -a exclude_patterns <<< "$exclude_files"
  for pattern in "${exclude_patterns[@]}"; do
    if [[ -z "$pattern" ]]; then
      echo "GREMLINS_EXCLUDE_FILES contains an empty pattern" >&2
      exit 2
    fi
    exclude_args+=("--exclude-files=${pattern}")
  done
fi

mkdir -p "$output_dir"

for pkg in "$@"; do
  if [[ "$pkg" == *"..."* ]]; then
    echo "Refusing broad package pattern: $pkg" >&2
    exit 2
  fi

  if ! go list "$pkg" >/dev/null; then
    echo "Package does not resolve: $pkg" >&2
    exit 2
  fi

  safe_pkg="$(printf '%s' "$pkg" | sed -E 's#^\./##; s#[^A-Za-z0-9_.-]+#_#g; s#_+$##')"
  if [[ -n "$output_label" ]]; then
    safe_pkg="${safe_pkg}_${output_label}"
  fi
  output_file="${output_dir}/gremlins_${safe_pkg}.json"

  args=(
    "github.com/go-gremlins/gremlins/cmd/gremlins@${gremlins_version}"
    unleash
    "--workers=${workers}"
    "--test-cpu=${test_cpu}"
    "--timeout-coefficient=${timeout_coefficient}"
    "--output=${output_file}"
  )

  if [[ -n "$output_statuses" ]]; then
    args+=("--output-statuses=${output_statuses}")
  fi
  if [[ "$dry_run" == "1" ]]; then
    args+=("--dry-run")
  fi
  if [[ -n "$tags" ]]; then
    args+=("--tags=${tags}")
  fi
  if (( ${#exclude_args[@]} > 0 )); then
    args+=("${exclude_args[@]}")
  fi

  args+=("$pkg")

  echo "==> Gremlins ${pkg}"
  echo "    output: ${output_file}"
  go run "${args[@]}"
done
