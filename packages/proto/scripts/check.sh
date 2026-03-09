#!/usr/bin/env bash
# Verify that generated proto stubs are up-to-date.
# Run this in CI to catch forgotten regeneration.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
bash "$SCRIPT_DIR/generate.sh"

if ! git diff --quiet; then
  echo "ERROR: Generated proto stubs are out of date."
  echo "Run 'bash packages/proto/scripts/generate.sh' and commit the result."
  git diff --stat
  exit 1
fi

echo "Proto stubs are up-to-date."
