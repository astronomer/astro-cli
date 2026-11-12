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
# Application identity for APPLE_TEAM_ID. Warns when that certificate expires within 30
# days. Writes the .p8 to a file. Exports MACOS_SIGN_KEYCHAIN, MACOS_NOTARY_KEY_PATH and
# MACOS_SIGN_IDENTITY through GITHUB_ENV for scripts/release/sign-macos-binary.sh, which
# goreleaser runs on each darwin binary. scripts/release/cleanup-macos-signing.sh removes
# what this creates, including after a failure partway through.
set -euo pipefail

: "${MACOS_CERTIFICATE_P12_BASE64:?}" "${MACOS_CERTIFICATE_PASSWORD:?}" "${APPLE_API_KEY_P8_BASE64:?}"
: "${APPLE_TEAM_ID:?}" "${GITHUB_ENV:?}"

# shellcheck source=scripts/release/macos-signing-paths.sh
. "$(dirname "$0")/macos-signing-paths.sh"

# Export the locations before creating anything, so cleanup finds them even if a later
# step fails. MACOS_SIGN_IDENTITY, which the signing hook keys off, comes last.
{
  echo "MACOS_SIGN_KEYCHAIN=$MACOS_SIGN_KEYCHAIN"
  echo "MACOS_NOTARY_KEY_PATH=$MACOS_NOTARY_KEY_PATH"
} >> "$GITHUB_ENV"

keychain_password=$(uuidgen)
umask 077
trap 'rm -f "$MACOS_SIGN_P12_PATH"' EXIT
printf '%s' "$MACOS_CERTIFICATE_P12_BASE64" | base64 --decode > "$MACOS_SIGN_P12_PATH"
printf '%s' "$APPLE_API_KEY_P8_BASE64" | base64 --decode > "$MACOS_NOTARY_KEY_PATH"

security create-keychain -p "$keychain_password" "$MACOS_SIGN_KEYCHAIN"
# Stay unlocked for the whole job; a notarization wait can be long.
security set-keychain-settings -lut 21600 "$MACOS_SIGN_KEYCHAIN"
security unlock-keychain -p "$keychain_password" "$MACOS_SIGN_KEYCHAIN"
if ! security import "$MACOS_SIGN_P12_PATH" -k "$MACOS_SIGN_KEYCHAIN" -P "$MACOS_CERTIFICATE_PASSWORD" -f pkcs12 -T /usr/bin/codesign >/dev/null; then
  echo "::error::The signing certificate could not be opened. Check MACOS_CERTIFICATE_P12_BASE64 and MACOS_CERTIFICATE_PASSWORD in the release-signing environment."
  exit 1
fi
# Let codesign use the key without a UI prompt.
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$keychain_password" "$MACOS_SIGN_KEYCHAIN" >/dev/null

# codesign finds keys through the search list. Put ours first, keeping the existing entries
# intact (one per line, quoted, possibly with spaces).
existing=()
while IFS= read -r entry; do
  entry="${entry#"${entry%%[![:space:]]*}"}"
  entry="${entry#\"}"
  entry="${entry%\"}"
  if [ -n "$entry" ]; then
    existing+=("$entry")
  fi
done < <(security list-keychains -d user)
security list-keychains -d user -s "$MACOS_SIGN_KEYCHAIN" ${existing[@]+"${existing[@]}"}

# find-identity -v lists only identities that have their private key, are in date and
# chain to Apple's root. The name ends in "(TEAMID)". Capture the output first: piping into
# grep or awk under pipefail can fail a good certificate when the reader exits early.
identities=$(security find-identity -v -p codesigning "$MACOS_SIGN_KEYCHAIN")
identity=$(awk -v team="(${APPLE_TEAM_ID})\"" '/"Developer ID Application: / && index($0, team) {print $2; exit}' <<<"$identities")
if [ -z "$identity" ]; then
  echo "::error::The signing certificate is not a valid Developer ID Application identity for team ${APPLE_TEAM_ID}. It may belong to another team, be expired or revoked, or be missing its private key."
  security find-identity -p codesigning "$MACOS_SIGN_KEYCHAIN" | sed 's/^/  /'
  exit 1
fi

# Expiry of the certificate we chose, matched by hash (a .p12 can hold more than one).
all_certs=$(security find-certificate -a -Z -p "$MACOS_SIGN_KEYCHAIN")
cert_pem=$(awk -v id="$identity" '/^SHA-1 hash:/ {take = ($3 == id); next} take' <<<"$all_certs" \
  | sed -n '/-----BEGIN CERTIFICATE-----/,/-----END CERTIFICATE-----/p')
if [ -z "$cert_pem" ]; then
  echo "::error::Could not read back the certificate for identity ${identity}."
  exit 1
fi
expires=$(openssl x509 -noout -enddate <<<"$cert_pem" | cut -d= -f2)
if ! openssl x509 -noout -checkend $((30 * 24 * 3600)) <<<"$cert_pem" >/dev/null; then
  echo "::warning::The macOS signing certificate expires ${expires}. Renew it and update the release-signing environment before then, or releases will fail."
fi

echo "MACOS_SIGN_IDENTITY=$identity" >> "$GITHUB_ENV"
echo "Signing identity ready: Developer ID Application for team ${APPLE_TEAM_ID} (${identity}), expires ${expires}."
