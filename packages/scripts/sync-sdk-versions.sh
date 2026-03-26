#!/usr/bin/env bash
set -euo pipefail

# Reads the version from the TypeScript SDK's package.json (canonical source
# after Changesets bumps) and writes it into every non-JS SDK's native version
# file so they stay in sync.
#
# Ruby and Rust SDKs have moved to dedicated repositories:
#   - Ruby: https://github.com/strait-dev/strait-ruby
#   - Rust: https://github.com/strait-dev/strait-rust

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(node -p "require('$ROOT_DIR/packages/typescript-sdk/package.json').version")

echo "Syncing SDK versions to $VERSION"

echo "All SDK versions synced to $VERSION"
