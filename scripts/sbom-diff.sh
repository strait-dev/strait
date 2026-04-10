#!/usr/bin/env bash
#
# sbom-diff.sh — compare two SPDX JSON SBOMs and print added/removed packages.
#
# Consumed by .github/workflows/publish-images.yml (sbom-diff job) to surface
# supply-chain changes between releases. Output is markdown, suitable for
# $GITHUB_STEP_SUMMARY or a GitHub Release body.

set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "usage: $0 <current-sbom.spdx.json> <previous-sbom.spdx.json>" >&2
  exit 1
fi

CURRENT="$1"
PREVIOUS="$2"

for f in "$CURRENT" "$PREVIOUS"; do
  if [ ! -s "$f" ]; then
    echo "sbom-diff: input $f is missing or empty" >&2
    exit 1
  fi
done

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

jq -r '.packages[]? | "\(.name)@\(.versionInfo // "unknown")"' "$CURRENT"  | sort -u > "$tmp/cur.txt"
jq -r '.packages[]? | "\(.name)@\(.versionInfo // "unknown")"' "$PREVIOUS" | sort -u > "$tmp/prev.txt"

added="$(comm -23 "$tmp/cur.txt" "$tmp/prev.txt")"
removed="$(comm -13 "$tmp/cur.txt" "$tmp/prev.txt")"

echo "## SBOM diff: $(basename "$PREVIOUS") -> $(basename "$CURRENT")"
echo

if [ -z "$added" ] && [ -z "$removed" ]; then
  echo "No package changes."
  exit 0
fi

if [ -n "$added" ]; then
  echo "### Added"
  echo '```'
  echo "$added"
  echo '```'
  echo
fi

if [ -n "$removed" ]; then
  echo "### Removed"
  echo '```'
  echo "$removed"
  echo '```'
fi
