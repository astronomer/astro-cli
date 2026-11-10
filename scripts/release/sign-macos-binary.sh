#!/usr/bin/env bash
# Signs and notarizes one darwin binary. goreleaser runs it as a post-build hook, so it
# happens before anything is archived or published, and a failure here stops the release.
#
# Usage: sign-macos-binary.sh <binary>
#
# Reads from the environment (set by scripts/release/setup-macos-signing.sh and the
# release workflow):
#   MACOS_SIGN_IDENTITY     SHA-1 of the Developer ID Application identity
#   MACOS_SIGN_KEYCHAIN     keychain holding it
#   MACOS_NOTARY_KEY_PATH   App Store Connect API key (.p8) for notarytool
#   APPLE_API_KEY_ID        its key ID
#   APPLE_API_ISSUER_ID     its issuer ID
#   APPLE_TEAM_ID           the team the signature must carry
#
# When MACOS_SIGN_IDENTITY is unset or empty (forks, local snapshots) it signs nothing and
# exits 0; the release workflow requires signing for official releases before goreleaser
# runs.
#
# Uses Apple's codesign rather than a reimplementation so the signature records the Team
# ID (codesign -dv's TeamIdentifier), which is what endpoint tools match Team ID rules on.
set -euo pipefail

bin="${1:?usage: sign-macos-binary.sh <binary>}"

if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
  echo "No macOS signing identity configured; leaving ${bin} unsigned." >&2
  exit 0
fi
: "${MACOS_SIGN_KEYCHAIN:?}" "${MACOS_NOTARY_KEY_PATH:?}" "${APPLE_API_KEY_ID:?}" "${APPLE_API_ISSUER_ID:?}" "${APPLE_TEAM_ID:?}"

here=$(cd "$(dirname "$0")" && pwd)

codesign --force --timestamp --options runtime \
  --identifier com.astronomer.astro \
  --keychain "$MACOS_SIGN_KEYCHAIN" \
  --sign "$MACOS_SIGN_IDENTITY" \
  "$bin"

# Team ID, hardened runtime and a valid signature, before spending time on notarization.
# GITHUB_OUTPUT is unset so this check doesn't write the release step's outputs.
env -u GITHUB_OUTPUT "$here/verify-macos-signatures.sh" "$bin"

# notarytool takes a zip, dmg or pkg. A bare binary can't be stapled; Gatekeeper looks the
# ticket up online.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
ditto -c -k --keepParent "$bin" "$work/astro.zip"

notary_auth=(--key "$MACOS_NOTARY_KEY_PATH" --key-id "$APPLE_API_KEY_ID" --issuer "$APPLE_API_ISSUER_ID")
xcrun notarytool submit "$work/astro.zip" "${notary_auth[@]}" \
  --wait --timeout 20m --output-format json > "$work/result.json"

submission=$(plutil -extract id raw -o - "$work/result.json" 2>/dev/null || echo "")
status=$(plutil -extract status raw -o - "$work/result.json" 2>/dev/null || echo "unknown")
if [ "$status" != "Accepted" ]; then
  echo "::error::Notarization of ${bin} finished with status '${status}' (submission ${submission:-unknown})."
  cat "$work/result.json" >&2
  if [ -n "$submission" ]; then
    xcrun notarytool log "$submission" "${notary_auth[@]}" >&2 || true
  fi
  exit 1
fi
echo "${bin}: notarized (submission ${submission})"
