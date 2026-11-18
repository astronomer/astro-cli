# shellcheck shell=bash disable=SC2034 # defines variables for the scripts that source it
# Where the release job keeps macOS signing material while it runs. Sourced by
# setup-macos-signing.sh, which creates these, and cleanup-macos-signing.sh, which
# removes them, so the two can't drift apart. Everything lives under RUNNER_TEMP.

: "${RUNNER_TEMP:?RUNNER_TEMP must be set}"
MACOS_SIGN_KEYCHAIN="$RUNNER_TEMP/astro-signing.keychain-db"
MACOS_NOTARY_KEY_PATH="$RUNNER_TEMP/astro-notary-key.p8"
MACOS_SIGN_P12_PATH="$RUNNER_TEMP/astro-signing.p12"
