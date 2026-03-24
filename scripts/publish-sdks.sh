#!/usr/bin/env bash
set -euo pipefail

# Publishes monorepo SDKs to their respective registries.
# Python and Go SDKs have moved to dedicated repositories:
#   - Python: https://github.com/strait-dev/strait-python
#   - Go:     https://github.com/strait-dev/strait-go

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(node -p "require('$ROOT_DIR/packages/typescript-sdk/package.json').version")
echo "Publishing SDKs at version $VERSION"

# 1. TypeScript → npm
echo "--- Publishing TypeScript SDK to npm ---"
cd "$ROOT_DIR/packages/typescript-sdk"
npm publish --access public

# 2. Ruby → RubyGems
echo "--- Publishing Ruby SDK to RubyGems ---"
cd "$ROOT_DIR/packages/ruby-sdk"
gem build strait.gemspec
gem push strait-"${VERSION}".gem

# 3. Rust → crates.io
echo "--- Publishing Rust SDK to crates.io ---"
cd "$ROOT_DIR/packages/rust-sdk"
cargo publish

echo "All SDKs published at version $VERSION"
