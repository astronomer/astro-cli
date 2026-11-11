#!/usr/bin/env bash
# Verifies the Authenticode signatures on Windows executables.
#
# Usage: verify-windows-signatures.sh <exe>...
#
# Each file must carry a valid signature whose signer is "Astronomer, Inc.", chaining to
# Microsoft's Identity Verification root, with a valid RFC 3161 timestamp. The timestamp
# matters: Azure Artifact Signing certificates last three days, and only a timestamped
# signature stays valid after that.
#
# Needs osslsigncode. macOS's and most Linux CA bundles don't include Microsoft's
# Identity Verification Root CA 2020 (Windows trusts it), so it's kept in this repo,
# pinned by fingerprint, and passed for both the signing and the timestamp chain.
#
# When GITHUB_OUTPUT is set, writes failed=true or failed=false to it so later jobs can
# hold back announcements for a release that failed this check.
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root="$here/certs/microsoft-identity-verification-root-2020.pem"
root_sha256="53:67:F2:0C:7A:DE:0E:2B:CA:79:09:15:05:6D:08:6B:72:0C:33:C1:FA:2A:26:61:AC:F7:87:E3:29:2E:12:70"
publisher='CN=Astronomer\, Inc.,'

fail() {
  echo "::error::$1"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "failed=true" >> "$GITHUB_OUTPUT"; fi
  exit 1
}

command -v osslsigncode >/dev/null || fail "osslsigncode is not installed."
fingerprint=$(openssl x509 -in "$root" -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)
[ "$fingerprint" = "$root_sha256" ] || fail "${root} is not the pinned Microsoft Identity Verification root (got '${fingerprint}')."
[ "$#" -gt 0 ] || fail "No Windows executables found to verify."

for exe in "$@"; do
  [ -f "$exe" ] || fail "${exe} does not exist."
  # Capture first and grep a here-string: piping into grep -q under pipefail can fail a
  # good result when grep exits early.
  report=$(osslsigncode verify -CAfile "$root" -TSA-CAfile "$root" -in "$exe" 2>&1)
  status=$?
  if [ "$status" -ne 0 ] || ! grep -q '^Signature verification: ok' <<<"$report"; then
    printf '%s\n' "$report" | tail -20 >&2
    fail "${exe} has no valid Authenticode signature chaining to Microsoft's Identity Verification root."
  fi
  grep -q '^Timestamp Server Signature verification: ok' <<<"$report" \
    || fail "${exe} has no valid timestamp, so its signature would stop being valid when the 3-day certificate expires."
  # The first "Subject:" after "Signer's certificate:" is the signing certificate's.
  signer=$(awk '/^Signer.s certificate:/ {want = 1} want && /Subject:/ {sub(/^[[:space:]]*Subject: /, ""); print; exit}' <<<"$report")
  case "$signer" in
    "$publisher"*) ;;
    *) fail "${exe} is signed by '${signer}', expected Astronomer, Inc." ;;
  esac
  when=$(sed -n 's/^[[:space:]]*Timestamp time: //p;' <<<"$report" | sed -n '1p')
  echo "${exe}: signed by Astronomer, Inc., timestamped ${when}"
done

if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "failed=false" >> "$GITHUB_OUTPUT"; fi
exit 0
