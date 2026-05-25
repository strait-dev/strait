#!/usr/bin/env bash
set -euo pipefail

usage() {
	cat <<'EOF'
Usage: scripts/test-integration-fast.sh [--all] [--keep-containers] [shard...]

Runs integration tests in controlled shards with shared persistent
testcontainers during the run. Containers are cleaned at exit by default.

Shards:
  smoke       Fast fixture and Redis smoke packages
  db          Store, queue, and scheduler integration packages
  api         HTTP API and gRPC API integration packages
  services    Worker, webhook, logdrain, notification, ratelimit
  migration   Config and migration lint integration packages
  e2e         End-to-end package

Examples:
  scripts/test-integration-fast.sh
  scripts/test-integration-fast.sh db api
  scripts/test-integration-fast.sh --all
  scripts/test-integration-fast.sh --keep-containers smoke
EOF
}

cleanup=true
declare -a requested=()

while [[ $# -gt 0 ]]; do
	case "$1" in
		--all)
			requested=(smoke db api services migration e2e)
			shift
			;;
		--keep-containers)
			cleanup=false
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			requested+=("$1")
			shift
			;;
	esac
done

if [[ ${#requested[@]} -eq 0 ]]; then
	requested=(smoke)
fi

repo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
strait_dir="${repo_dir}/apps/strait"
clean_script="${strait_dir}/scripts/test-integration-clean.sh"

if [[ "${cleanup}" == true ]]; then
	trap '"${clean_script}" >/dev/null' EXIT
fi

export GOMAXPROCS="${GOMAXPROCS:-2}"
export STRAIT_TEST_PERSIST_CONTAINERS=1
export STRAIT_TEST_TIMING="${STRAIT_TEST_TIMING:-1}"

go_flags=(-p "${GO_TEST_P:-1}" -tags integration -count=1 -timeout "${GO_TEST_TIMEOUT:-20m}")

run_shard() {
	local name="$1"
	shift
	local -a packages=("$@")
	local start
	start="$(date +%s)"

	echo "==> integration shard: ${name}"
	(
		cd "${strait_dir}"
		go test "${go_flags[@]}" "${packages[@]}"
	)
	local elapsed
	elapsed="$(( $(date +%s) - start ))"
	echo "==> integration shard complete: ${name} (${elapsed}s)"
}

for shard in "${requested[@]}"; do
	case "${shard}" in
		smoke)
			run_shard smoke ./internal/testutil ./internal/pubsub
			;;
		db)
			run_shard db ./internal/store ./internal/queue ./internal/scheduler
			;;
		api)
			run_shard api ./internal/api ./internal/api/grpc
			;;
		services)
			run_shard services ./internal/worker ./internal/webhook ./internal/logdrain ./internal/notification ./internal/ratelimit
			;;
		migration)
			run_shard migration ./internal/config ./internal/migrationlint
			;;
		e2e)
			run_shard e2e ./internal/e2e
			;;
		*)
			echo "unknown integration shard: ${shard}" >&2
			usage >&2
			exit 2
			;;
	esac
done
