#!/usr/bin/env bash
set -euo pipefail

# Publishes monorepo SDKs to their respective registries.
# The following SDKs have moved to dedicated repositories:
#   - Python: https://github.com/strait-dev/strait-python
#   - Go:     https://github.com/strait-dev/strait-go
#   - Ruby:   https://github.com/strait-dev/strait-ruby
#   - Rust:   https://github.com/strait-dev/strait-rust

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(node -p "require('$ROOT_DIR/packages/typescript-sdk/package.json').version")
echo "Publishing SDKs at version $VERSION"

# 1. TypeScript → npm
echo "--- Publishing TypeScript SDK to npm ---"
cd "$ROOT_DIR/packages/typescript-sdk"
npm publish --access public

echo "All SDKs published at version $VERSION"
