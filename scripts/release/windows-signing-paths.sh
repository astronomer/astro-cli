# shellcheck shell=bash disable=SC2034 # defines variables for the scripts that source it
# Where the release job keeps Windows signing material while it runs. Sourced by
# setup-windows-signing.sh, which creates these, sign-windows-binary.sh, which uses them,
# and cleanup-windows-signing.sh, which removes them. Everything lives under RUNNER_TEMP.

: "${RUNNER_TEMP:?RUNNER_TEMP must be set}"
WINDOWS_SIGNING_TOKEN_FILE="$RUNNER_TEMP/astro-codesigning-token"
