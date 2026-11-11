#!/usr/bin/env bash
# Signs one Windows binary with Authenticode through Azure Artifact Signing. goreleaser
# runs it as a post-build hook, so it happens before anything is archived or published,
# and a failure here stops the release.
#
# Usage: sign-windows-binary.sh <exe>
#
# Reads from the environment (set by scripts/release/setup-windows-signing.sh and the
# release workflow):
#   WINDOWS_SIGNING_READY     "true" once setup succeeded
#   JSIGN_JAR                 the pinned jsign jar
#   WINDOWS_SIGNING_ENDPOINT  Artifact Signing endpoint host, e.g. eus.codesigning.azure.net
#   WINDOWS_SIGNING_ACCOUNT   Artifact Signing account
#   WINDOWS_SIGNING_PROFILE   its certificate profile
#   OFFICIAL_RELEASE          "true" in the release workflow for astronomer/astro-cli
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
: "${JSIGN_JAR:?}" "${WINDOWS_SIGNING_ENDPOINT:?}" "${WINDOWS_SIGNING_ACCOUNT:?}" "${WINDOWS_SIGNING_PROFILE:?}"

here=$(cd "$(dirname "$0")" && pwd)
name=$(basename "$(dirname "$exe")")  # e.g. windows-build_windows_amd64_v1, to tell the parallel hooks apart

sign() {
  local token
  # A fresh token per attempt: they last about an hour, and a cold build can take longer.
  token=$(az account get-access-token --resource https://codesigning.azure.net --query accessToken -o tsv)
  java -jar "$JSIGN_JAR" \
    --storetype TRUSTEDSIGNING \
    --keystore "$WINDOWS_SIGNING_ENDPOINT" \
    --storepass "$token" \
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
# this check doesn't write the release step's outputs.
env -u GITHUB_OUTPUT "$here/verify-windows-signatures.sh" "$exe"
echo "[${name}] signed"
