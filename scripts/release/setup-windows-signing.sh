#!/usr/bin/env bash
# Prepares Windows (Authenticode) signing for the release job, before goreleaser builds
# anything. Run it right after azure/login has logged the Azure CLI in through GitHub OIDC,
# and after Java is on PATH.
#
# Reads from the environment: RUNNER_TEMP, GITHUB_ENV (set by GitHub Actions).
#
# Downloads jsign at a pinned version and checks its SHA-256, and installs osslsigncode for
# verification if it's missing. Gets the Azure Artifact Signing access token now, while the
# OIDC login is fresh (a federated login can't be refreshed later in a long job), masks it
# in the logs, and writes it to a file the signing hook passes to jsign as
# --storepass file:<path>, so it never appears on a command line. Exports JSIGN_JAR,
# WINDOWS_SIGNING_TOKEN_EXPIRES (epoch seconds) and WINDOWS_SIGNING_READY=true through
# GITHUB_ENV for scripts/release/sign-windows-binary.sh. cleanup-windows-signing.sh
# removes the token file.
set -euo pipefail

: "${RUNNER_TEMP:?}" "${GITHUB_ENV:?}"
# shellcheck source=scripts/release/windows-signing-paths.sh
. "$(dirname "$0")/windows-signing-paths.sh"

jsign_version=7.6
jsign_sha256=6cf58aa080e7ff82656f320cff8d4a3bff30408df8766806dcba3740dd839fa5
jsign_jar="$RUNNER_TEMP/jsign-${jsign_version}.jar"

command -v java >/dev/null || { echo "::error::Java is not on PATH; jsign needs it."; exit 1; }
curl -fsSL --retry 3 -o "$jsign_jar" "https://github.com/ebourg/jsign/releases/download/${jsign_version}/jsign-${jsign_version}.jar"
actual=$(shasum -a 256 "$jsign_jar" | cut -d' ' -f1)
if [ "$actual" != "$jsign_sha256" ]; then
  echo "::error::jsign ${jsign_version} checksum mismatch: got ${actual}, expected ${jsign_sha256}."
  exit 1
fi

if ! command -v osslsigncode >/dev/null; then
  brew install osslsigncode >/dev/null
fi

command -v az >/dev/null || { echo "::error::The Azure CLI is not installed."; exit 1; }
if ! token_info=$(az account get-access-token --resource https://codesigning.azure.net \
    --query '[accessToken, expires_on]' -o tsv 2>&1); then
  echo "::error::Could not get an Azure Artifact Signing token. Check the release-signing environment's AZURE_CLIENT_ID and AZURE_TENANT_ID variables and the app's federated credential."
  echo "$token_info" >&2
  exit 1
fi
token=$(sed -n '1p' <<<"$token_info")
expires=$(sed -n '2p' <<<"$token_info")
if [ -z "$token" ] || ! [[ "$expires" =~ ^[0-9]+$ ]]; then
  echo "::error::The Azure CLI returned no usable Artifact Signing token."
  exit 1
fi
# Redact the token from every later log line before anything could print it.
echo "::add-mask::$token"
umask 077
printf '%s' "$token" > "$WINDOWS_SIGNING_TOKEN_FILE"

{
  echo "JSIGN_JAR=$jsign_jar"
  echo "WINDOWS_SIGNING_TOKEN_EXPIRES=$expires"
  echo "WINDOWS_SIGNING_READY=true"
} >> "$GITHUB_ENV"
echo "Windows signing ready: jsign ${jsign_version}, $(osslsigncode --version 2>&1 | sed -n '1p'), Azure token valid for $(( (expires - $(date +%s)) / 60 )) minutes."
