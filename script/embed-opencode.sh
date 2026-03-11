#!/usr/bin/env bash
#
# Builds the opencode binary from the Astronomer fork and compresses it
# into cmd/agent/embedded/opencode.gz for go:embed.
#
# Usage:
#   ./script/embed-opencode.sh
#
# Environment variables:
#   OPENCODE_BIN          - Skip build, embed this local binary instead
#   OPENCODE_REPO         - Git repo to clone (default: astronomer/opencode)
#   OPENCODE_REF          - Branch/tag to build (default: main)
#   OPENCODE_REPO_PATH    - Use a local checkout instead of cloning
#   TARGET_OS / TARGET_ARCH - Override platform (for CI cross-compilation)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EMBED_DIR="$ROOT_DIR/cmd/agent/embedded"
REPO="${OPENCODE_REPO:-astronomer/opencode}"
REF="${OPENCODE_REF:-main}"

# Allow overriding platform (useful for CI cross-compilation)
TARGET_OS="${TARGET_OS:-}"
TARGET_ARCH="${TARGET_ARCH:-}"

# Detect platform if not overridden
if [ -z "$TARGET_OS" ]; then
  TARGET_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$TARGET_OS" in
    darwin)               TARGET_OS="darwin" ;;
    linux)                TARGET_OS="linux" ;;
    mingw*|msys*|cygwin*) TARGET_OS="windows" ;;
    *)                    echo "Unsupported OS: $TARGET_OS"; exit 1 ;;
  esac
fi

if [ -z "$TARGET_ARCH" ]; then
  TARGET_ARCH="$(uname -m)"
fi

# Normalize arch names (goreleaser uses amd64/386, opencode uses x64/arm64)
case "$TARGET_ARCH" in
  x86_64|amd64)  TARGET_ARCH="x64" ;;
  arm64|aarch64) TARGET_ARCH="arm64" ;;
  386)
    echo "Warning: opencode does not support 32-bit (386). Embedding placeholder."
    echo "The 'astro agent' command will show an error on this platform."
    mkdir -p "$EMBED_DIR"
    echo -n "unsupported" | gzip > "$EMBED_DIR/opencode.gz"
    echo -n "unsupported" > "$EMBED_DIR/version.txt"
    exit 0
    ;;
  *)
    echo "Unsupported arch: $TARGET_ARCH"; exit 1
    ;;
esac

mkdir -p "$EMBED_DIR"

# ──────────────────────────────────────────────────────────
# Option 1: Use a pre-built binary
# ──────────────────────────────────────────────────────────
if [ -n "${OPENCODE_BIN:-}" ]; then
  echo "Using local opencode binary: $OPENCODE_BIN"
  if [ ! -f "$OPENCODE_BIN" ]; then
    echo "Error: $OPENCODE_BIN does not exist"
    exit 1
  fi
  gzip -c "$OPENCODE_BIN" > "$EMBED_DIR/opencode.gz"
  echo "Compressed $OPENCODE_BIN -> $EMBED_DIR/opencode.gz"
  exit 0
fi

# ──────────────────────────────────────────────────────────
# Option 2: Build from source (Astronomer fork)
# ──────────────────────────────────────────────────────────

# Check for bun
if ! command -v bun &>/dev/null; then
  echo "Error: bun is required to build opencode from source."
  echo "Install it: curl -fsSL https://bun.sh/install | bash"
  exit 1
fi

# Get the source
if [ -n "${OPENCODE_REPO_PATH:-}" ]; then
  echo "Using local opencode repo: $OPENCODE_REPO_PATH"
  OPENCODE_DIR="$OPENCODE_REPO_PATH"
  CLEANUP_OPENCODE=false
else
  OPENCODE_DIR=$(mktemp -d)
  CLEANUP_OPENCODE=true
  echo "Cloning $REPO@$REF..."
  gh repo clone "$REPO" "$OPENCODE_DIR" -- --depth 1 --branch "$REF" --quiet 2>&1
fi

# Clean up on exit if we cloned
if [ "$CLEANUP_OPENCODE" = true ]; then
  trap 'rm -rf "$OPENCODE_DIR"' EXIT
fi

# Build
echo "Building opencode for ${TARGET_OS}/${TARGET_ARCH}..."
cd "$OPENCODE_DIR"
bun install --frozen-lockfile 2>&1 | tail -1

# --single builds only for the current platform
# For cross-compilation, we'd need to build all and pick the right one
if [ "$TARGET_OS" = "$(uname -s | tr '[:upper:]' '[:lower:]' | sed 's/darwin/darwin/;s/linux/linux/')" ] && \
   [ "$TARGET_ARCH" = "$(uname -m | sed 's/x86_64/x64/;s/aarch64/arm64/')" ]; then
  bun run --filter opencode build --single 2>&1
else
  bun run --filter opencode build 2>&1
fi

# Find the built binary
BINARY_NAME="opencode-${TARGET_OS}-${TARGET_ARCH}"
# opencode uses @opencode-ai/opencode as the npm name prefix
PKG_NAME="@opencode-ai/opencode"
DIST_DIR="$OPENCODE_DIR/packages/opencode/dist"

if [[ "$TARGET_OS" == "windows" ]]; then
  OPENCODE_BIN=$(find "$DIST_DIR" -path "*${BINARY_NAME}*" -name "opencode.exe" -type f | head -1)
else
  OPENCODE_BIN=$(find "$DIST_DIR" -path "*${BINARY_NAME}*" -name "opencode" -type f \
    -not -name "*.gz" -not -name "*.zip" -not -name "*.tar.gz" | head -1)
fi

# Fallback: try matching with the full npm package name prefix
if [ -z "$OPENCODE_BIN" ]; then
  FULL_NAME="${PKG_NAME}-${TARGET_OS}-${TARGET_ARCH}"
  if [[ "$TARGET_OS" == "windows" ]]; then
    OPENCODE_BIN=$(find "$DIST_DIR" -path "*${TARGET_OS}*${TARGET_ARCH}*" -name "opencode.exe" -type f | head -1)
  else
    OPENCODE_BIN=$(find "$DIST_DIR" -path "*${TARGET_OS}*${TARGET_ARCH}*" -name "opencode" -type f \
      -not -name "*.gz" -not -name "*.zip" | head -1)
  fi
fi

if [ -z "$OPENCODE_BIN" ]; then
  echo "Error: Could not find opencode binary in build output"
  echo "Expected pattern: *${BINARY_NAME}*"
  echo "Available in dist/:"
  find "$DIST_DIR" -type f -name "opencode*" 2>/dev/null || echo "  (empty)"
  exit 1
fi

# Get version from the built package
VERSION_NUM=$(cd "$OPENCODE_DIR/packages/opencode" && node -p "require('./package.json').version" 2>/dev/null || echo "dev")

# Compress and place
cd "$ROOT_DIR"
gzip -c "$OPENCODE_BIN" > "$EMBED_DIR/opencode.gz"
echo -n "$VERSION_NUM" > "$EMBED_DIR/version.txt"

ORIGINAL_SIZE=$(wc -c < "$OPENCODE_BIN" | tr -d ' ')
COMPRESSED_SIZE=$(wc -c < "$EMBED_DIR/opencode.gz" | tr -d ' ')
echo ""
echo "Built and embedded opencode ${VERSION_NUM} (${TARGET_OS}/${TARGET_ARCH})"
echo "  Source:     $REPO@$REF"
echo "  Original:   $(( ORIGINAL_SIZE / 1024 / 1024 ))MB"
echo "  Compressed: $(( COMPRESSED_SIZE / 1024 / 1024 ))MB"
echo "  Output:     $EMBED_DIR/opencode.gz"
