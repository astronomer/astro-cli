#!/usr/bin/env bash
# Removes what scripts/release/setup-windows-signing.sh left on the runner: the Azure
# Artifact Signing token file. Safe to run when setup never ran or failed partway.
set -uo pipefail

[ -n "${RUNNER_TEMP:-}" ] || exit 0
# shellcheck source=scripts/release/windows-signing-paths.sh
. "$(dirname "$0")/windows-signing-paths.sh"
rm -f "$WINDOWS_SIGNING_TOKEN_FILE"
exit 0
