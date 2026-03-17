#!/usr/bin/env bash
set -euo pipefail

# Publishes all 5 SDKs to their respective registries.
# Called by the release workflow after the "Version Packages" PR is merged.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(node -p "require('$ROOT_DIR/packages/typescript-sdk/package.json').version")
echo "Publishing SDKs at version $VERSION"

# 1. TypeScript → npm
echo "--- Publishing TypeScript SDK to npm ---"
cd "$ROOT_DIR/packages/typescript-sdk"
npm publish --access public

# 2. Python → PyPI
echo "--- Publishing Python SDK to PyPI ---"
cd "$ROOT_DIR/packages/python-sdk"
python -m build
twine upload dist/*

# 3. Go → git tag
echo "--- Tagging Go SDK ---"
cd "$ROOT_DIR"
git tag "go-sdk/v${VERSION}"
git push origin "go-sdk/v${VERSION}"

# 4. Ruby → RubyGems
echo "--- Publishing Ruby SDK to RubyGems ---"
cd "$ROOT_DIR/packages/ruby-sdk"
gem build strait.gemspec
gem push strait-"${VERSION}".gem

# 5. Rust → crates.io
echo "--- Publishing Rust SDK to crates.io ---"
cd "$ROOT_DIR/packages/rust-sdk"
cargo publish

echo "All SDKs published at version $VERSION"
