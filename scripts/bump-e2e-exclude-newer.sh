#!/usr/bin/env bash
# Move the date the e2e suite resolves Python packages as of.
#
#   scripts/bump-e2e-exclude-newer.sh             to today, 00:00 UTC
#   scripts/bump-e2e-exclude-newer.sh 2026-10-01  to a chosen day
#
# The constant is pinnedExcludeNewer in e2e/excludenewer_test.go, handed to
# every uv the suite runs as UV_EXCLUDE_NEWER; its comment says when to move
# it. Prints the value now in the file. .github/workflows/bump-e2e-exclude-newer.yml
# opens the PR when the file changed.
set -euo pipefail

cd "$(dirname "$0")/.."

const_file=e2e/excludenewer_test.go
day="${1:-$(date -u +%Y-%m-%d)}"

if ! [[ "$day" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
  echo "bump-e2e-exclude-newer: want a day as YYYY-MM-DD, got '$day'" >&2
  exit 2
fi

pattern='^const pinnedExcludeNewer = "[0-9]{4}-[0-9]{2}-[0-9]{2}T00:00:00Z"$'
if ! grep -Eq "$pattern" "$const_file"; then
  echo "bump-e2e-exclude-newer: no pinnedExcludeNewer line in the expected shape in $const_file" >&2
  exit 1
fi

# A temp file and a move rather than sed -i, whose spelling differs between GNU
# and BSD sed.
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
sed -E "s/^const pinnedExcludeNewer = \"[^\"]*\"$/const pinnedExcludeNewer = \"${day}T00:00:00Z\"/" "$const_file" >"$tmp"
cat "$tmp" >"$const_file"

sed -En 's/^const pinnedExcludeNewer = "(.*)"$/\1/p' "$const_file"
