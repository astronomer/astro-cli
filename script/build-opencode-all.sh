#!/usr/bin/env bash
#
# Builds opencode from the Astronomer fork for ALL platforms.
# Called once in goreleaser's before.hooks. The per-build pick-opencode.sh
# then selects the right binary for each target.
#
# Output: .opencode-build/dist/ contains all platform binaries.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BUILD_DIR="$ROOT_DIR/.opencode-build"
REPO="${OPENCODE_REPO:-astronomer/opencode}"
REF="${OPENCODE_REF:-main}"

if ! command -v bun &>/dev/null; then
  echo "Error: bun is required to build opencode from source."
  echo "Install it: curl -fsSL https://bun.sh/install | bash"
  exit 1
fi

# Get the source
if [ -n "${OPENCODE_REPO_PATH:-}" ]; then
  echo "Using local opencode repo: $OPENCODE_REPO_PATH"
  OPENCODE_DIR="$OPENCODE_REPO_PATH"
else
  OPENCODE_DIR=$(mktemp -d)
  trap 'rm -rf "$OPENCODE_DIR"' EXIT
  echo "Cloning $REPO@$REF..."
  gh repo clone "$REPO" "$OPENCODE_DIR" -- --depth 1 --branch "$REF" --quiet 2>&1
fi

echo "Building opencode (all platforms)..."
cd "$OPENCODE_DIR"
bun install --frozen-lockfile 2>&1 | tail -1
bun run --filter opencode build 2>&1

# Copy dist to a stable location the per-build hook can find
rm -rf "$BUILD_DIR"
mkdir -p "$BUILD_DIR"
cp -r "$OPENCODE_DIR/packages/opencode/dist" "$BUILD_DIR/dist"

# Save version
VERSION_NUM=$(node -p "require('./packages/opencode/package.json').version" 2>/dev/null || echo "dev")
echo -n "$VERSION_NUM" > "$BUILD_DIR/version.txt"

echo ""
echo "Built opencode ${VERSION_NUM} (all platforms)"
echo "  Source: $REPO@$REF"
echo "  Output: $BUILD_DIR/dist/"
ls -d "$BUILD_DIR/dist"/*/ 2>/dev/null | while read d; do echo "    $(basename "$d")"; done
