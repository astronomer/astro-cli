#!/usr/bin/env bash
# Verifies the darwin binaries goreleaser built and signed.
#
# Usage: verify-macos-signatures.sh <binary>...
# Reads APPLE_TEAM_ID from the environment. Each binary must have a valid signature
# from that team with the hardened runtime enabled, which notarization requires.
#
# When GITHUB_OUTPUT is set, writes failed=true or failed=false to it so later jobs
# can hold back announcements for a release that failed this check.
set -uo pipefail

: "${APPLE_TEAM_ID:?}"

fail() {
  echo "::error::$1"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "failed=true" >> "$GITHUB_OUTPUT"
  exit 1
}

[ "$#" -gt 0 ] || fail "No darwin binaries found to verify."

for bin in "$@"; do
  [ -f "$bin" ] || fail "${bin} does not exist."
  codesign --verify --strict --verbose=2 "$bin" || fail "${bin} has no valid code signature."

  details=$(codesign -dv --verbose=2 "$bin" 2>&1)
  team=$(printf '%s\n' "$details" | sed -n 's/^TeamIdentifier=//p')
  [ "$team" = "$APPLE_TEAM_ID" ] || fail "${bin} is signed by team '${team}', expected ${APPLE_TEAM_ID}."

  # e.g. "CodeDirectory v=20500 size=... flags=0x10000(runtime) hashes=..."
  # A here-string rather than a pipe, so grep -q exiting early can't fail it under pipefail.
  grep -Eq '^CodeDirectory .*flags=0x[0-9a-f]+\([^)]*runtime' <<<"$details" \
    || fail "${bin} is signed without the hardened runtime, so notarization would not hold."

  echo "${bin}: signed by team ${team}, hardened runtime on"
done

[ -n "${GITHUB_OUTPUT:-}" ] && echo "failed=false" >> "$GITHUB_OUTPUT"
exit 0
