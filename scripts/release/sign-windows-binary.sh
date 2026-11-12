#!/usr/bin/env bash
# Signs one Windows binary with Authenticode through Azure Artifact Signing. goreleaser
# runs it as a post-build hook, so it happens before anything is archived or published,
# and a failure here stops the release.
#
# Usage: sign-windows-binary.sh <exe>
#
# Reads from the environment (set by scripts/release/setup-windows-signing.sh and the
# release workflow):
#   WINDOWS_SIGNING_READY          "true" once setup succeeded
#   JSIGN_JAR                      the pinned jsign jar
#   WINDOWS_SIGNING_TOKEN_EXPIRES  when the access token setup saved expires (epoch seconds)
#   WINDOWS_SIGNING_ENDPOINT       Artifact Signing endpoint host, e.g. eus.codesigning.azure.net
#   WINDOWS_SIGNING_ACCOUNT        Artifact Signing account
#   WINDOWS_SIGNING_PROFILE        its certificate profile
#   OFFICIAL_RELEASE               "true" in the release workflow for astronomer/astro-cli
#   RUNNER_TEMP                    where the token file is (windows-signing-paths.sh)
#
# When signing isn't set up it signs nothing: exit 0 for forks and local snapshots, but
# exit 1 when OFFICIAL_RELEASE is "true", so an official release can't ship unsigned.
#
# Artifact Signing certificates last three days, so the signature must be timestamped;
# jsign timestamps automatically for this service, and verify-windows-signatures.sh
# refuses a signature without a valid timestamp.
set -euo pipefail

exe="${1:?usage: sign-windows-binary.sh <exe>}"

if [ "${WINDOWS_SIGNING_READY:-}" != "true" ]; then
  if [ "${OFFICIAL_RELEASE:-}" = "true" ]; then
    echo "::error::Official release but Windows signing isn't set up; refusing to leave ${exe} unsigned."
    exit 1
  fi
  echo "Windows signing not configured; leaving ${exe} unsigned." >&2
  exit 0
fi
: "${JSIGN_JAR:?}" "${WINDOWS_SIGNING_TOKEN_EXPIRES:?}" "${WINDOWS_SIGNING_ENDPOINT:?}" "${WINDOWS_SIGNING_ACCOUNT:?}" "${WINDOWS_SIGNING_PROFILE:?}"
# shellcheck source=scripts/release/windows-signing-paths.sh
. "$(dirname "$0")/windows-signing-paths.sh"

here=$(cd "$(dirname "$0")" && pwd)
name=$(basename "$(dirname "$exe")")  # e.g. windows-build_windows_amd64_v1, to tell the parallel hooks apart

if [ ! -s "$WINDOWS_SIGNING_TOKEN_FILE" ]; then
  echo "::error::[${name}] No Artifact Signing token at ${WINDOWS_SIGNING_TOKEN_FILE}; setup-windows-signing.sh should have written it."
  exit 1
fi
# setup got the token while the OIDC login was fresh; it can't be refreshed now.
if [ "$(( WINDOWS_SIGNING_TOKEN_EXPIRES - $(date +%s) ))" -lt 120 ]; then
  echo "::error::[${name}] The Artifact Signing token has expired or is about to; the build ran longer than the token's lifetime."
  exit 1
fi

sign() {
  # The token is read from the file (jsign's file: prefix), never put on the command line.
  java -jar "$JSIGN_JAR" \
    --storetype TRUSTEDSIGNING \
    --keystore "$WINDOWS_SIGNING_ENDPOINT" \
    --storepass "file:${WINDOWS_SIGNING_TOKEN_FILE}" \
    --alias "${WINDOWS_SIGNING_ACCOUNT}/${WINDOWS_SIGNING_PROFILE}" \
    --alg SHA-256 \
    "$exe"
}
if ! sign; then
  echo "[${name}] Windows signing failed; retrying once in 30s." >&2
  sleep 30
  sign
fi

# Signer, chain and timestamp, before goreleaser archives it. GITHUB_OUTPUT is unset so
# this check doesn't write the release step's outputs. It skips the CRL download, so a
# Microsoft CRL server blip can't fail the release here; the post-publish check verifies
# revocation.
env -u GITHUB_OUTPUT WINDOWS_VERIFY_SKIP_CRL=true "$here/verify-windows-signatures.sh" "$exe"
echo "[${name}] signed"
