#!/usr/bin/env bash
# Removes what scripts/release/setup-macos-signing.sh left on the runner. Safe to run when
# setup never ran or failed partway.
set -uo pipefail

if [ -n "${MACOS_SIGN_KEYCHAIN:-}" ]; then
  security delete-keychain "$MACOS_SIGN_KEYCHAIN" 2>/dev/null || true
fi
rm -f "${MACOS_NOTARY_KEY_PATH:-}" "${RUNNER_TEMP:-/nonexistent}/astro-signing.p12" "${RUNNER_TEMP:-/nonexistent}/astro-notary-key.p8"
exit 0
