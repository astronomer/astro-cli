#!/usr/bin/env bash
# Prepares macOS signing for the release job, before goreleaser builds anything.
#
# Reads from the environment:
#   MACOS_CERTIFICATE_P12_BASE64  base64 of the Developer ID Application .p12
#   MACOS_CERTIFICATE_PASSWORD    its password
#   APPLE_API_KEY_P8_BASE64       base64 of the App Store Connect API key (.p8) for notarytool
#   APPLE_TEAM_ID                 the team the certificate must belong to
#   RUNNER_TEMP, GITHUB_ENV       set by GitHub Actions
#
# Imports the .p12 into a keychain that lives for the rest of the job, and fails unless it
# holds a valid (private key present, in date, chained to Apple's root) Developer ID
# Application identity for APPLE_TEAM_ID. Warns when it expires within 30 days. Writes the
# .p8 to a file. Exports MACOS_SIGN_KEYCHAIN, MACOS_SIGN_IDENTITY and MACOS_NOTARY_KEY_PATH
# through GITHUB_ENV for scripts/release/sign-macos-binary.sh, which goreleaser runs on
# each darwin binary. Remove the keychain and key file afterwards with
# scripts/release/cleanup-macos-signing.sh.
set -euo pipefail

: "${MACOS_CERTIFICATE_P12_BASE64:?}" "${MACOS_CERTIFICATE_PASSWORD:?}" "${APPLE_API_KEY_P8_BASE64:?}"
: "${APPLE_TEAM_ID:?}" "${RUNNER_TEMP:?}" "${GITHUB_ENV:?}"

keychain="$RUNNER_TEMP/astro-signing.keychain-db"
keychain_password=$(uuidgen)
p12="$RUNNER_TEMP/astro-signing.p12"
notary_key="$RUNNER_TEMP/astro-notary-key.p8"

umask 077
printf '%s' "$MACOS_CERTIFICATE_P12_BASE64" | base64 --decode > "$p12"
printf '%s' "$APPLE_API_KEY_P8_BASE64" | base64 --decode > "$notary_key"
trap 'rm -f "$p12"' EXIT

security create-keychain -p "$keychain_password" "$keychain"
# Stay unlocked for the whole job; a notarization wait can be long.
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
if ! security import "$p12" -k "$keychain" -P "$MACOS_CERTIFICATE_PASSWORD" -f pkcs12 -T /usr/bin/codesign >/dev/null; then
  echo "::error::The signing certificate could not be opened. Check MACOS_CERTIFICATE_P12_BASE64 and MACOS_CERTIFICATE_PASSWORD in the release-signing environment."
  exit 1
fi
# Let codesign use the key without a UI prompt.
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$keychain" >/dev/null
# codesign looks keys up through the search list.
existing=$(security list-keychains -d user | tr -d '"')
# shellcheck disable=SC2086 # word-splitting the existing list is intended
security list-keychains -d user -s "$keychain" $existing

# find-identity -v lists only identities that have their private key, are in date and
# chain to Apple's root. The name ends in "(TEAMID)". Capture the output first: piping into
# grep -q under pipefail can fail a good certificate when grep exits on the first match.
identities=$(security find-identity -v -p codesigning "$keychain")
identity=$(awk -v team="(${APPLE_TEAM_ID})\"" '/"Developer ID Application: / && index($0, team) {print $2; exit}' <<<"$identities")
if [ -z "$identity" ]; then
  echo "::error::The signing certificate is not a valid Developer ID Application identity for team ${APPLE_TEAM_ID}. It may belong to another team, be expired or revoked, or be missing its private key."
  security find-identity -p codesigning "$keychain" | sed 's/^/  /'
  exit 1
fi

cert_pem="$RUNNER_TEMP/astro-signing-cert.pem"
security find-certificate -c "Developer ID Application" -p "$keychain" > "$cert_pem"
expires=$(openssl x509 -in "$cert_pem" -noout -enddate | cut -d= -f2)
if ! openssl x509 -in "$cert_pem" -noout -checkend $((30 * 24 * 3600)) >/dev/null; then
  echo "::warning::The macOS signing certificate expires ${expires}. Renew it and update the release-signing environment before then, or releases will fail."
fi
rm -f "$cert_pem"

{
  echo "MACOS_SIGN_KEYCHAIN=$keychain"
  echo "MACOS_SIGN_IDENTITY=$identity"
  echo "MACOS_NOTARY_KEY_PATH=$notary_key"
} >> "$GITHUB_ENV"
echo "Signing identity ready: Developer ID Application for team ${APPLE_TEAM_ID} (${identity}), expires ${expires}."
