#!/usr/bin/env bash
# Checks the macOS signing certificate before goreleaser builds and publishes anything.
#
# Reads from the environment:
#   MACOS_CERTIFICATE_P12_BASE64  base64 of the Developer ID Application .p12
#   MACOS_CERTIFICATE_PASSWORD    its password
#   APPLE_TEAM_ID                 the team the certificate must belong to
#
# Fails when the .p12 can't be opened with the password, when it holds no valid
# "Developer ID Application" identity for APPLE_TEAM_ID (wrong team, expired, or an
# untrusted chain), and warns when the certificate expires within 30 days. Uses Apple's
# own `security` tool, which reads every .p12 format Keychain Access exports.
set -euo pipefail

: "${MACOS_CERTIFICATE_P12_BASE64:?}" "${MACOS_CERTIFICATE_PASSWORD:?}" "${APPLE_TEAM_ID:?}"

work=$(mktemp -d)
keychain="$work/check.keychain-db"
keychain_password=$(uuidgen)
cleanup() {
  security delete-keychain "$keychain" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT

printf '%s' "$MACOS_CERTIFICATE_P12_BASE64" | base64 --decode > "$work/cert.p12"

security create-keychain -p "$keychain_password" "$keychain"
security unlock-keychain -p "$keychain_password" "$keychain"
if ! security import "$work/cert.p12" -k "$keychain" -P "$MACOS_CERTIFICATE_PASSWORD" -f pkcs12 >/dev/null; then
  echo "::error::The signing certificate could not be opened. Check MACOS_CERTIFICATE_P12_BASE64 and MACOS_CERTIFICATE_PASSWORD in the release-signing environment."
  exit 1
fi

# find-identity -v lists only identities that have their private key, are in date and
# chain to Apple's root. The name ends in "(TEAMID)".
if ! security find-identity -v -p codesigning "$keychain" | grep -q "\"Developer ID Application: .* (${APPLE_TEAM_ID})\""; then
  echo "::error::The signing certificate is not a valid Developer ID Application identity for team ${APPLE_TEAM_ID}. It may belong to another team, be expired or revoked, or be missing its private key."
  security find-identity -p codesigning "$keychain" | sed 's/^/  /'
  exit 1
fi

cert_pem="$work/cert.pem"
security find-certificate -c "Developer ID Application" -p "$keychain" > "$cert_pem"
expires=$(openssl x509 -in "$cert_pem" -noout -enddate | cut -d= -f2)
if ! openssl x509 -in "$cert_pem" -noout -checkend $((30 * 24 * 3600)) >/dev/null; then
  echo "::warning::The macOS signing certificate expires ${expires}. Renew it and update the release-signing environment before then, or releases will fail."
fi
echo "Signing certificate OK: Developer ID Application for team ${APPLE_TEAM_ID}, expires ${expires}."
