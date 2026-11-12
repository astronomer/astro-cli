#!/usr/bin/env bash
# Removes what scripts/release/setup-macos-signing.sh created on the runner. Safe to run
# when setup never ran or failed partway: the paths come from macos-signing-paths.sh, not
# from anything setup exported. Deleting the keychain also takes it off the search list.
set -uo pipefail

[ -n "${RUNNER_TEMP:-}" ] || exit 0
# shellcheck source=scripts/release/macos-signing-paths.sh
. "$(dirname "$0")/macos-signing-paths.sh"

if [ -e "$MACOS_SIGN_KEYCHAIN" ]; then
  security delete-keychain "$MACOS_SIGN_KEYCHAIN" || echo "::warning::Could not delete the signing keychain at ${MACOS_SIGN_KEYCHAIN}."
fi
rm -f "$MACOS_NOTARY_KEY_PATH" "$MACOS_SIGN_P12_PATH"
exit 0
