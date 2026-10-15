#!/usr/bin/env bash
#
# Tests the version resolution in godownloader.sh, the script behind
# install.astronomer.io. Each case runs the whole installer under sh and bash
# with a stubbed curl that answers the GitHub API from fixtures below and
# fails every download, so nothing is installed: the case reads the tag the
# installer settled on from its "found version" log line.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
mkdir -p "$work/stub" "$work/api" "$work/tmp"

# release TAG PRERELEASE CHECKSUMS prints one release as the API pretty-prints
# it, with only the fields the installer reads plus a body that quotes another
# version.
release() {
  local tag=$1 pre=$2 sums=$3 sep=${4:-,}
  cat <<EOF
  {
    "html_url": "https://github.com/astronomer/astro-cli/releases/tag/$tag",
    "tag_name": "$tag",
    "name": "$tag",
    "draft": false,
    "prerelease": $pre,
    "assets": [
EOF
  if [ "$sums" = yes ]; then
    cat <<EOF
      {
        "name": "astro_${tag#v}_checksums.txt",
        "size": 1024
      },
EOF
  fi
  cat <<EOF
      {
        "name": "astro_${tag#v}_linux_amd64.tar.gz",
        "size": 1024
      }
    ],
    "tarball_url": "https://api.github.com/repos/astronomer/astro-cli/tarball/$tag",
    "zipball_url": "https://api.github.com/repos/astronomer/astro-cli/zipball/$tag",
    "body": "Upgrade notes: see v1.99.0"
  }$sep
EOF
}

# Newest first, as the API lists them. Page 1 is all pre-releases, as daily
# nightlies make it; the newest nightly is still uploading its checksums.
{
  echo '['
  release v2.0.0-nightly.20261015 true no
  release v2.0.0-nightly.20261014 true yes
  release v1.47.0-rc.1 true yes
  release v1.46.1-rc.2 true yes
  release v1.46.1-rc.1 true yes ''
  echo ']'
} >"$work/api/releases-page-1"
{
  echo '['
  release v1.46.0 false yes
  release v1.46.0-rc.1 true yes
  release v1.45.2 false yes
  release v1.4.1 false yes ''
  echo ']'
} >"$work/api/releases-page-2"
echo '[]' >"$work/api/releases-page-3"
for tag in v1.46.0 v1.4.1 v2.0.0-nightly.20261014; do
  release "$tag" false yes '' >"$work/api/tags-$tag"
done

cat >"$work/stub/curl" <<'EOF'
#!/bin/sh
# Serves api.github.com from $STUB_API and fails every other request.
url=
for arg in "$@"; do
  case "$arg" in
    https://*) url=$arg ;;
    "Authorization: Bearer "*) echo "$arg" >>"$STUB_API/auth-log" ;;
  esac
done
case "$url" in
  https://api.github.com/*)
    if [ -n "${STUB_RATE_LIMIT:-}" ]; then
      echo '{"message": "API rate limit exceeded for 192.0.2.1."}'
      exit 0
    fi
    case "$url" in
      */releases\?per_page=100\&page=*) file=releases-page-${url##*page=} ;;
      */releases/tags/*) file=tags-${url##*/} ;;
      *) file=none ;;
    esac
    if [ -f "$STUB_API/$file" ]; then
      cat "$STUB_API/$file"
    else
      echo '{"message": "Not Found", "status": "404"}'
    fi
    ;;
  *) printf 404 ;;
esac
EOF
chmod +x "$work/stub/curl"

failures=0
# sh is dash on Debian and Ubuntu, bash on macOS; test both when both exist.
shells=(sh bash)
if command -v dash >/dev/null; then
  shells+=(dash)
fi
# resolve SHELL ARG... runs the installer and prints the tag it resolved, or
# nothing when it gave up before a download.
resolve() {
  local shell=$1
  shift
  PATH="$work/stub:$PATH" STUB_API="$work/api" TMPDIR="$work/tmp" \
    "$shell" "$root/godownloader.sh" -b "$work/bin" "$@" 2>"$work/stderr" >/dev/null || true
  sed -n 's/.*found version: [^ ]* for \([^/]*\)\/.*/\1/p' "$work/stderr"
}

check() {
  local want=$1 arg=$2 shell got
  for shell in "${shells[@]}"; do
    got=$(resolve "$shell" "$arg")
    if [ "$got" != "$want" ]; then
      echo "FAIL [$shell] '$arg': want '${want:-<failure>}', got '${got:-<failure>}'"
      sed 's/^/    /' "$work/stderr"
      failures=$((failures + 1))
    else
      echo "ok   [$shell] '$arg' -> ${got:-<failure>}"
    fi
  done
}

# A bare major: the newest stable, else the newest pre-release with checksums.
check v1.46.0 v1
check v1.46.0 1
check v2.0.0-nightly.20261014 v2
check v2.0.0-nightly.20261014 2
# A major.minor: the same, under that minor. v1.46's stable is on page 2,
# behind its newer rcs; v1.4 does not match v1.46.*; v1.47 has only an rc.
check v1.46.0 v1.46
check v1.46.0 1.46
check v1.4.1 v1.4
check v1.47.0-rc.1 v1.47
# A full version or a pre-release: exactly that tag, with or without the v.
check v1.46.0 v1.46.0
check v1.46.0 1.46.0
check v1.4.1 1.4.1
check v2.0.0-nightly.20261014 v2.0.0-nightly.20261014
# Nothing to find. v1.99.0 appears only in release bodies.
check '' v9
check '' v1.99
check '' v1.99.0
check '' v1.46.
check '' v1..46
check '' latest

# A rate-limited answer says so.
STUB_RATE_LIMIT=1 resolve sh v1.46 >/dev/null
if grep -q 'rate limit exceeded; set GITHUB_TOKEN' "$work/stderr"; then
  echo "ok   rate limit is reported"
else
  echo "FAIL rate limit is not reported"
  failures=$((failures + 1))
fi

# GITHUB_TOKEN is sent on every API request.
rm -f "$work/api/auth-log"
GITHUB_TOKEN=secret resolve sh v1.46 >/dev/null
if [ "$(sort -u "$work/api/auth-log" 2>/dev/null)" = "Authorization: Bearer secret" ]; then
  echo "ok   GITHUB_TOKEN is sent"
else
  echo "FAIL GITHUB_TOKEN is not sent"
  failures=$((failures + 1))
fi

if [ -n "$(ls -A "$work/bin" 2>/dev/null)" ]; then
  echo "FAIL something was installed into $work/bin"
  failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
  echo "$failures failure(s)"
  exit 1
fi
echo "all passed"
