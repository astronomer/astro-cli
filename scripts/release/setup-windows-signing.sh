#!/usr/bin/env bash
# Prepares Windows (Authenticode) signing for the release job, before goreleaser builds
# anything. Run it after azure/login has logged the Azure CLI in through GitHub OIDC and
# after Java is on PATH.
#
# Reads from the environment: RUNNER_TEMP, GITHUB_ENV (set by GitHub Actions).
#
# Downloads jsign at a pinned version and checks its SHA-256, installs osslsigncode for
# verification if it's missing, and checks the Azure CLI can get a token for Azure
# Artifact Signing. Exports JSIGN_JAR and WINDOWS_SIGNING_READY=true through GITHUB_ENV for
# scripts/release/sign-windows-binary.sh, which goreleaser runs on each Windows binary.
set -euo pipefail

: "${RUNNER_TEMP:?}" "${GITHUB_ENV:?}"

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
# Proves the OIDC login worked and a token for the signing service can be issued. Only the
# expiry is printed; the signing hook gets its own token when it runs.
if ! expires=$(az account get-access-token --resource https://codesigning.azure.net --query expiresOn -o tsv 2>&1); then
  echo "::error::Could not get an Azure Artifact Signing token. Check the release-signing environment's AZURE_CLIENT_ID and AZURE_TENANT_ID variables and the app's federated credential."
  echo "$expires" >&2
  exit 1
fi

{
  echo "JSIGN_JAR=$jsign_jar"
  echo "WINDOWS_SIGNING_READY=true"
} >> "$GITHUB_ENV"
echo "Windows signing ready: jsign ${jsign_version}, $(osslsigncode --version 2>&1 | sed -n '1p'), Azure token valid until ${expires}."
