#!/usr/bin/env bash
set -euo pipefail

# Reads the version from the TypeScript SDK's package.json (canonical source
# after Changesets bumps) and writes it into every non-JS SDK's native version
# file so they stay in sync.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

VERSION=$(node -p "require('$ROOT_DIR/packages/typescript-sdk/package.json').version")

echo "Syncing SDK versions to $VERSION"

# Python — pyproject.toml
sed -i.bak -E "s/^version = \".*\"/version = \"$VERSION\"/" \
  "$ROOT_DIR/packages/python-sdk/pyproject.toml"
rm -f "$ROOT_DIR/packages/python-sdk/pyproject.toml.bak"

# Ruby — lib/strait/version.rb
sed -i.bak -E "s/VERSION = \".*\"/VERSION = \"$VERSION\"/" \
  "$ROOT_DIR/packages/ruby-sdk/lib/strait/version.rb"
rm -f "$ROOT_DIR/packages/ruby-sdk/lib/strait/version.rb.bak"

# Rust — Cargo.toml (only the top-level version under [package])
sed -i.bak -E '0,/^version = ".*"/{s/^version = ".*"/version = "'"$VERSION"'"/}' \
  "$ROOT_DIR/packages/rust-sdk/Cargo.toml"
rm -f "$ROOT_DIR/packages/rust-sdk/Cargo.toml.bak"

# Go — uses git tags (go-sdk/vX.Y.Z), no file to update

echo "All SDK versions synced to $VERSION"
