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
# By default osslsigncode also downloads Microsoft's CRLs to check the signing and
# timestamp certificates haven't been revoked, so a CRL server outage fails the check.
# WINDOWS_VERIFY_SKIP_CRL=true skips that (-ignore-crl): the signing hook sets it, so a
# blip there can't fail the release before publishing, while the post-publish check
# still verifies revocation.
#
# When GITHUB_OUTPUT is set, writes failed=true or failed=false to it so later jobs can
# hold back announcements for a release that failed this check.
set -uo pipefail

here=$(cd "$(dirname "$0")" && pwd)
root="$here/certs/microsoft-identity-verification-root-2020.pem"
root_sha256="53:67:F2:0C:7A:DE:0E:2B:CA:79:09:15:05:6D:08:6B:72:0C:33:C1:FA:2A:26:61:AC:F7:87:E3:29:2E:12:70"
# The signer's CN, as a whole attribute anywhere in the subject: osslsigncode prints
# "CN=Astronomer\, Inc.,O=..." (RFC 2253, comma escaped); older or other builds may print
# "/C=US/.../CN=Astronomer, Inc." or another order.
publisher_re='(^|[,/])[[:space:]]*CN=Astronomer\\?, Inc\.([,/]|$)'

fail() {
  echo "::error::$1"
  if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "failed=true" >> "$GITHUB_OUTPUT"; fi
  exit 1
}

command -v osslsigncode >/dev/null || fail "osslsigncode is not installed."
# The checks below read osslsigncode's report; record the version that produced it.
echo "Verifying with $(osslsigncode --version 2>&1 | sed -n '1p')"
fingerprint=$(openssl x509 -in "$root" -noout -fingerprint -sha256 2>/dev/null | cut -d= -f2)
[ "$fingerprint" = "$root_sha256" ] || fail "${root} is not the pinned Microsoft Identity Verification root (got '${fingerprint}')."
[ "$#" -gt 0 ] || fail "No Windows executables found to verify."

crl_args=()
if [ "${WINDOWS_VERIFY_SKIP_CRL:-}" = "true" ]; then
  crl_args=(-ignore-crl)
  echo "Skipping the CRL revocation check here; the post-publish check runs it."
fi

for exe in "$@"; do
  [ -f "$exe" ] || fail "${exe} does not exist."
  # Capture first and grep a here-string: piping into grep -q under pipefail can fail a
  # good result when grep exits early.
  report=$(osslsigncode verify ${crl_args[@]+"${crl_args[@]}"} -CAfile "$root" -TSA-CAfile "$root" -in "$exe" 2>&1)
  status=$?
  if [ "$status" -ne 0 ] && grep -q '^Failed to get CRL from' <<<"$report"; then
    grep '^Failed to get CRL from' <<<"$report" >&2
    fail "${exe}: couldn't download Microsoft's certificate revocation list, so revocation wasn't checked. This is usually a temporary outage of Microsoft's CRL server, not a signature problem; re-run the check."
  fi
  if [ "$status" -ne 0 ] || ! grep -q '^Signature verification: ok' <<<"$report"; then
    printf '%s\n' "$report" | tail -20 >&2
    fail "${exe} has no valid Authenticode signature chaining to Microsoft's Identity Verification root."
  fi
  grep -q '^Timestamp Server Signature verification: ok' <<<"$report" \
    || fail "${exe} has no valid timestamp, so its signature would stop being valid when the 3-day certificate expires."
  # The first "Subject:" after "Signer's certificate:" is the signing certificate's.
  signer=$(awk '/^Signer.s certificate:/ {want = 1} want && /Subject:/ {sub(/^[[:space:]]*Subject: /, ""); print; exit}' <<<"$report")
  grep -Eq "$publisher_re" <<<"$signer" || fail "${exe} is signed by '${signer}', expected Astronomer, Inc."
  when=$(sed -n 's/^[[:space:]]*Timestamp time: //p;' <<<"$report" | sed -n '1p')
  echo "${exe}: signed by Astronomer, Inc., timestamped ${when}"
done

if [ -n "${GITHUB_OUTPUT:-}" ]; then echo "failed=false" >> "$GITHUB_OUTPUT"; fi
exit 0
