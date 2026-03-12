#!/usr/bin/env bash
#
# Builds the astro-agent binary from the agent/ TypeScript project.
# Uses bun build --compile to produce a single binary.
#
# Output: agent/dist/astro-agent (for current platform)
#         or agent/dist/astro-agent-{os}-{arch} (for cross-compilation)
#
# Environment variables:
#   TARGET_OS   - Override target OS (darwin, linux, windows)
#   TARGET_ARCH - Override target arch (arm64, x64)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
AGENT_DIR="$ROOT_DIR/agent"

if ! command -v bun &>/dev/null; then
  echo "Error: bun is required to build astro-agent."
  echo "Install it: curl -fsSL https://bun.sh/install | bash"
  exit 1
fi

cd "$AGENT_DIR"

# Install dependencies
echo "Installing dependencies..."
bun install --frozen-lockfile 2>&1 | tail -1

TARGET_OS="${TARGET_OS:-}"
TARGET_ARCH="${TARGET_ARCH:-}"

if [ -z "$TARGET_OS" ] && [ -z "$TARGET_ARCH" ]; then
  # Build for current platform
  echo "Building astro-agent for current platform..."
  bun build --compile ./src/main.ts --outfile dist/astro-agent
else
  # Normalize OS/arch for bun's target format
  OS="${TARGET_OS:-$(uname -s | tr '[:upper:]' '[:lower:]')}"
  ARCH="${TARGET_ARCH:-$(uname -m)}"

  case "$ARCH" in
    x86_64|amd64) ARCH="x64" ;;
    arm64|aarch64) ARCH="arm64" ;;
  esac

  case "$OS" in
    darwin|linux|windows) ;;
    *) echo "Unsupported OS: $OS"; exit 1 ;;
  esac

  TARGET="bun-${OS}-${ARCH}"
  OUTFILE="dist/astro-agent-${OS}-${ARCH}"
  [ "$OS" = "windows" ] && OUTFILE="${OUTFILE}.exe"

  echo "Building astro-agent for ${OS}/${ARCH} (target: ${TARGET})..."
  bun build --compile --target="$TARGET" ./src/main.ts --outfile "$OUTFILE"
fi

# Copy binary assets (Pi needs these at runtime next to the binary)
echo "Copying binary assets..."
cp "$AGENT_DIR/package.json" "$AGENT_DIR/dist/package.json"
THEME_SRC="$AGENT_DIR/node_modules/@mariozechner/pi-coding-agent/dist/modes/interactive/theme"
mkdir -p "$AGENT_DIR/dist/theme"
cp "$THEME_SRC"/*.json "$AGENT_DIR/dist/theme/"

echo "Done."
ls -lh dist/astro-agent* 2>/dev/null
