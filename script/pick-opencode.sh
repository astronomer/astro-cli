#!/usr/bin/env bash
#
# Picks the right opencode binary from .opencode-build/dist/ for the
# given TARGET_OS and TARGET_ARCH, then compresses it into cmd/agent/embedded/.
#
# Called by goreleaser's per-build hooks with {{ .Os }} and {{ .Arch }}.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EMBED_DIR="$ROOT_DIR/cmd/agent/embedded"
BUILD_DIR="$ROOT_DIR/.opencode-build"

TARGET_OS="${TARGET_OS:-}"
TARGET_ARCH="${TARGET_ARCH:-}"

# Normalize arch names (goreleaser uses amd64, opencode uses x64)
case "$TARGET_ARCH" in
  x86_64|amd64) TARGET_ARCH="x64" ;;
  arm64|aarch64) TARGET_ARCH="arm64" ;;
  386)
    echo "Warning: opencode does not support 32-bit (386). Embedding placeholder."
    mkdir -p "$EMBED_DIR"
    echo -n "unsupported" | gzip > "$EMBED_DIR/opencode.gz"
    echo -n "unsupported" > "$EMBED_DIR/version.txt"
    exit 0
    ;;
esac

if [ ! -d "$BUILD_DIR/dist" ]; then
  echo "Error: .opencode-build/dist/ not found. Run build-opencode-all.sh first."
  exit 1
fi

mkdir -p "$EMBED_DIR"

# Find the binary for this platform
if [[ "$TARGET_OS" == "windows" ]]; then
  OPENCODE_BIN=$(find "$BUILD_DIR/dist" -path "*${TARGET_OS}*${TARGET_ARCH}*" -name "opencode.exe" -type f | head -1)
else
  OPENCODE_BIN=$(find "$BUILD_DIR/dist" -path "*${TARGET_OS}*${TARGET_ARCH}*" -name "opencode" -type f \
    -not -name "*.gz" -not -name "*.zip" | head -1)
fi

if [ -z "$OPENCODE_BIN" ]; then
  echo "Error: No opencode binary found for ${TARGET_OS}/${TARGET_ARCH}"
  echo "Available in build dir:"
  find "$BUILD_DIR/dist" -name "opencode*" -type f 2>/dev/null
  exit 1
fi

gzip -c "$OPENCODE_BIN" > "$EMBED_DIR/opencode.gz"
cp "$BUILD_DIR/version.txt" "$EMBED_DIR/version.txt"

SIZE=$(wc -c < "$EMBED_DIR/opencode.gz" | tr -d ' ')
echo "Embedded opencode for ${TARGET_OS}/${TARGET_ARCH} ($(( SIZE / 1024 / 1024 ))MB compressed)"
