#!/usr/bin/env bash
#
# Downloads the opencode binary for the current platform and compresses it
# into cmd/agent/embedded/opencode.gz for go:embed.
#
# Usage:
#   ./script/embed-opencode.sh [version]
#
# If no version is specified, uses OPENCODE_VERSION env var or defaults to "latest".
# Set OPENCODE_BIN to skip download and embed a local binary instead.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EMBED_DIR="$ROOT_DIR/cmd/agent/embedded"

VERSION="${1:-${OPENCODE_VERSION:-latest}}"
REPO="anomalyco/opencode"

# Detect platform
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$OS" in
  darwin) OS="darwin" ;;
  linux)  OS="linux" ;;
  *)      echo "Unsupported OS: $OS"; exit 1 ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="x64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)             echo "Unsupported arch: $ARCH"; exit 1 ;;
esac

BINARY_NAME="opencode-${OS}-${ARCH}"

mkdir -p "$EMBED_DIR"

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

# Resolve "latest" to an actual tag
if [ "$VERSION" = "latest" ]; then
  echo "Resolving latest opencode release..."
  VERSION=$(gh release view --repo "$REPO" --json tagName -q '.tagName' 2>/dev/null || true)
  if [ -z "$VERSION" ]; then
    echo "Error: Could not resolve latest version. Pass a version explicitly or set OPENCODE_VERSION."
    exit 1
  fi
  echo "Resolved to $VERSION"
fi

# Strip leading 'v' for asset name matching if present
VERSION_TAG="$VERSION"
VERSION_NUM="${VERSION#v}"

# Download
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

ASSET_PATTERN="${BINARY_NAME}"
echo "Downloading opencode ${VERSION_TAG} for ${OS}/${ARCH}..."

if [[ "$OS" == "linux" ]]; then
  ASSET_EXT="tar.gz"
else
  ASSET_EXT="zip"
fi

gh release download "$VERSION_TAG" \
  --repo "$REPO" \
  --pattern "*${BINARY_NAME}*.${ASSET_EXT}" \
  --dir "$TMPDIR" 2>/dev/null || {
    echo "Error: Could not download opencode binary."
    echo "Tried pattern: *${BINARY_NAME}*.${ASSET_EXT} from ${REPO}@${VERSION_TAG}"
    echo ""
    echo "Available assets:"
    gh release view "$VERSION_TAG" --repo "$REPO" --json assets -q '.assets[].name' 2>/dev/null || true
    exit 1
  }

# Extract
ARCHIVE=$(ls "$TMPDIR"/*.${ASSET_EXT} | head -1)
echo "Extracting $ARCHIVE..."

if [[ "$ASSET_EXT" == "tar.gz" ]]; then
  tar -xzf "$ARCHIVE" -C "$TMPDIR"
else
  unzip -qo "$ARCHIVE" -d "$TMPDIR"
fi

# Find the opencode binary
OPENCODE_BIN=$(find "$TMPDIR" -name "opencode" -type f -not -name "*.gz" -not -name "*.zip" -not -name "*.tar.gz" | head -1)

if [ -z "$OPENCODE_BIN" ]; then
  echo "Error: Could not find opencode binary in extracted archive"
  ls -la "$TMPDIR"
  exit 1
fi

# Compress and place
gzip -c "$OPENCODE_BIN" > "$EMBED_DIR/opencode.gz"

# Write version file for runtime version checking
echo -n "$VERSION_NUM" > "$EMBED_DIR/version.txt"

ORIGINAL_SIZE=$(wc -c < "$OPENCODE_BIN" | tr -d ' ')
COMPRESSED_SIZE=$(wc -c < "$EMBED_DIR/opencode.gz" | tr -d ' ')
echo ""
echo "Embedded opencode ${VERSION_TAG} (${OS}/${ARCH})"
echo "  Original:   $(( ORIGINAL_SIZE / 1024 / 1024 ))MB"
echo "  Compressed: $(( COMPRESSED_SIZE / 1024 / 1024 ))MB"
echo "  Output:     $EMBED_DIR/opencode.gz"
