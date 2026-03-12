#!/usr/bin/env bash
#
# Builds the astro-agent and packs it (binary + assets) into
# cmd/agent/embedded/opencode.gz for go:embed.
#
# The Go side extracts this as a tarball to ~/.astro/agent/.
#
# Environment variables:
#   AGENT_BIN             - Skip build, embed this pre-built binary instead
#   TARGET_OS / TARGET_ARCH - Override platform (for CI cross-compilation)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EMBED_DIR="$ROOT_DIR/cmd/agent/embedded"
AGENT_DIR="$ROOT_DIR/agent"

TARGET_OS="${TARGET_OS:-}"
TARGET_ARCH="${TARGET_ARCH:-}"

# Detect platform if not overridden
if [ -z "$TARGET_OS" ]; then
  TARGET_OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
fi
if [ -z "$TARGET_ARCH" ]; then
  TARGET_ARCH="$(uname -m)"
fi

# Normalize arch
case "$TARGET_ARCH" in
  x86_64|amd64) TARGET_ARCH="x64" ;;
  arm64|aarch64) TARGET_ARCH="arm64" ;;
  386)
    echo "Warning: 32-bit not supported. Embedding placeholder."
    mkdir -p "$EMBED_DIR"
    echo -n "unsupported" | gzip > "$EMBED_DIR/opencode.gz"
    echo -n "unsupported" > "$EMBED_DIR/version.txt"
    exit 0
    ;;
esac

mkdir -p "$EMBED_DIR"

if [ -n "${AGENT_BIN:-}" ]; then
  echo "Using pre-built agent binary: $AGENT_BIN"
  DIST_DIR="$(dirname "$AGENT_BIN")"
else
  # Build the agent
  TARGET_OS="$TARGET_OS" TARGET_ARCH="$TARGET_ARCH" "$SCRIPT_DIR/build-agent.sh"
  DIST_DIR="$AGENT_DIR/dist"
fi

# Get version
VERSION=$(cd "$AGENT_DIR" && node -p "require('./package.json').version" 2>/dev/null || echo "dev")

# Create a tarball of the binary + assets
# The Go side extracts this to ~/.astro/agent/
TMPTAR=$(mktemp)
tar -czf "$TMPTAR" -C "$DIST_DIR" .

cp "$TMPTAR" "$EMBED_DIR/opencode.gz"
rm -f "$TMPTAR"
echo -n "$VERSION" > "$EMBED_DIR/version.txt"

SIZE=$(wc -c < "$EMBED_DIR/opencode.gz" | tr -d ' ')
echo ""
echo "Embedded astro-agent ${VERSION} (${TARGET_OS}/${TARGET_ARCH})"
echo "  Compressed: $(( SIZE / 1024 / 1024 ))MB"
echo "  Output:     $EMBED_DIR/opencode.gz"
