#!/usr/bin/env bash
set -euo pipefail

containers="$(docker ps -aq --filter 'name=strait-test-' || true)"
if [[ -n "${containers}" ]]; then
	docker rm -f ${containers}
fi

networks="$(docker network ls -q --filter 'name=strait-test-' || true)"
if [[ -n "${networks}" ]]; then
	docker network rm ${networks} >/dev/null 2>&1 || true
fi

echo "Cleaned Strait integration testcontainers resources."
