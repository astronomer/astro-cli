#!/usr/bin/env bash
#
# Tests the version resolution in godownloader.sh, the script behind
# install.astronomer.io. Each case runs the whole installer under sh, bash and
# dash with a stubbed curl that answers the GitHub API from fixtures below and
# fails every download, so nothing is installed: the case reads the tag the
# installer settled on from its "found version" log line, and the stub's
# request log says how many API calls it took.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
api=$work/api
mkdir -p "$work/stub" "$api" "$work/tmp"

# release TAG [CHECKSUMS] writes the releases/tags answer for TAG, with its
# checksums asset unless CHECKSUMS is "no", and a body that would break echo
# under dash and quotes another release's asset name.
release() {
  local tag=$1 sums=${2:-yes}
  {
    cat <<EOF
{
  "tag_name": "$tag",
  "name": "$tag",
  "prerelease": false,
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
  "body": "Fixes \\\\c in paths. See \\"name\\": \\"astro_${tag#v}_checksums.txt\\""
}
EOF
  } >"$api/tags-$tag"
}
# Every tag in the fixture repository and the state of its release: a release
# with checksums, a release still uploading them, or no release at all.
# Rows are in matching-refs text order: v1.46.10 before v1.46.9, v1.10.0 before v1.9.0.
while read -r tag state; do
  echo "$tag" >>"$api/all-tags"
  case "$state" in
    release) release "$tag" ;;
    uploading) release "$tag" no ;;
  esac
done <<'EOF'
v0.6.0 release
v0.6.1 release
v0.6.2 none
v1.10.0 release
v1.4.1 release
v1.46.0 release
v1.46.0-rc.1 release
v1.46.10 uploading
v1.46.11-rc.1 release
v1.46.9 release
v1.47.0-rc.1 release
v1.47.0-rc.2 uploading
v1.9.0 release
v2.0.0-nightly.20261014 release
v2.0.0-nightly.20261015 uploading
v3.0.0 none
v3.0.0-rc.1 release
v3.0.1 none
v3.0.2 none
v3.0.3 none
v3.0.4 none
v3.0.5 none
EOF

cat >"$work/stub/curl" <<'EOF'
#!/bin/sh
# Serves api.github.com from $STUB_API and fails every other request. A file
# $STUB_API/status-KEY makes the request for KEY answer with that status.
url=
for arg in "$@"; do
  case "$arg" in
    https://*) url=$arg ;;
    "Authorization: Bearer "*) echo "$arg" >>"$STUB_API/auth-log" ;;
  esac
done
case "$url" in
  https://api.github.com/*) ;;
  *)
    printf 404
    exit 0
    ;;
esac
echo "${url#https://api.github.com/}" >>"$STUB_API/requests"
case "$url" in
  */git/matching-refs/tags/*) key=refs-${url##*/} ;;
  */releases/tags/*) key=tags-${url##*/} ;;
  *) key=none ;;
esac
if [ -f "$STUB_API/status-$key" ]; then
  status=$(cat "$STUB_API/status-$key")
  case "$status" in
    403) echo '{"message": "API rate limit exceeded for 192.0.2.1."}' ;;
    *) echo '{"message": "Server Error"}' ;;
  esac
  printf '%s' "$status"
  exit 0
fi
case "$key" in
  refs-*)
    want=${key#refs-}
    echo '['
    awk -v want="$want" 'index($0, want) == 1' "$STUB_API/all-tags" | LC_ALL=C sort |
      awk '{ if (NR > 1) print "  },"; print "  {"; print "    \"ref\": \"refs/tags/" $0 "\"," ; print "    \"object\": { \"type\": \"tag\" }" }
        END { if (NR) print "  }" }'
    echo ']'
    printf 200
    ;;
  tags-*)
    if [ -f "$STUB_API/$key" ]; then
      cat "$STUB_API/$key"
      printf 200
    else
      echo '{"message": "Not Found", "status": "404"}'
      printf 404
    fi
    ;;
  *)
    echo '{"message": "Not Found", "status": "404"}'
    printf 404
    ;;
esac
EOF
# linux/amd64 everywhere, so a host that is darwin/arm64 does not refuse v0.6.
cat >"$work/stub/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo x86_64 ;;
  *) echo Linux ;;
esac
EOF
chmod +x "$work/stub/curl" "$work/stub/uname"

failures=0
fail() {
  echo "FAIL $*"
  sed 's/^/    /' "$work/stderr"
  failures=$((failures + 1))
}

# sh is dash on Debian and Ubuntu, bash on macOS; test both when both exist.
shells=(sh bash)
if command -v dash >/dev/null; then
  shells+=(dash)
fi

# resolve SHELL ARG runs the installer and prints the tag it resolved, or
# nothing when it gave up before a download.
resolve() {
  rm -f "$api/requests"
  PATH="$work/stub:$PATH" STUB_API="$api" TMPDIR="$work/tmp" \
    "$1" "$root/godownloader.sh" -b "$work/bin" "$2" 2>"$work/stderr" >/dev/null || true
  sed -n 's/.*found version: [^ ]* for \([^/]*\)\/.*/\1/p' "$work/stderr"
}
requests() {
  if [ -f "$api/requests" ]; then wc -l <"$api/requests" | tr -d ' '; else echo 0; fi
}

# check WANT ARG [CALLS] [STDERR]: ARG resolves to WANT ('' for a failure)
# under every shell, in CALLS API requests, saying STDERR.
check() {
  local want=$1 arg=$2 calls=${3:-} says=${4:-} shell got n
  for shell in "${shells[@]}"; do
    got=$(resolve "$shell" "$arg")
    n=$(requests)
    if [ "$got" != "$want" ]; then
      fail "[$shell] '$arg': want '${want:-<failure>}', got '${got:-<failure>}'"
    elif [ -n "$calls" ] && [ "$n" != "$calls" ]; then
      fail "[$shell] '$arg': want $calls API requests, made $n"
    elif [ -n "$says" ] && ! grep -qF "$says" "$work/stderr"; then
      fail "[$shell] '$arg': stderr does not say '$says'"
    else
      echo "ok   [$shell] '$arg' -> ${got:-<failure>} ($n requests)"
    fi
  done
}

# vX.Y: the highest stable by version, not by text or creation order, in one
# listing and one confirmation.
check v1.4.1 v1.4 2
check v1.4.1 1.4 2
check v1.10.0 v1.10 2
# v1.46.10 is still uploading, so v1.46.9; never the rcs around it.
check v1.46.9 v1.46 3
check v1.46.9 1.46 3
# v0.6.2 has no release; an old minor still resolves.
check v0.6.1 v0.6 3
# No stable tag: the highest pre-release with checksums.
check v1.47.0-rc.1 v1.47 3
# vX: the same over the whole major. v1.10.0 is not the highest, v1.46.10 is.
check v1.46.9 v1 3
check v1.46.9 1 3
check v2.0.0-nightly.20261014 v2 3
check v2.0.0-nightly.20261014 2 3
# Five stable tags without a release stop the search: no rc is installed in
# their place, and v3.0.0 is never asked about.
check '' v3 6 'no release has its checksums uploaded'
# A full version or a pre-release: exactly that tag, with or without the v.
check v1.46.0 v1.46.0 1
check v1.46.0 1.46.0 1
check v2.0.0-nightly.20261014 v2.0.0-nightly.20261014 1
# Nothing to find.
check '' v9 1 'no tag starts with v9.'
check '' v1.99 1
check '' v1.99.0 1
check '' v1.46. 1
check '' v1..46 1
check '' latest 1

# An error from the API fails the install instead of settling for another tag.
echo 403 >"$api/status-tags-v1.46.10"
check '' v1.46 2 'rate limit exceeded; set GITHUB_TOKEN'
rm "$api/status-tags-v1.46.10"
echo 502 >"$api/status-refs-v1.46."
check '' v1.46 1 'HTTP 502'
rm "$api/status-refs-v1.46."
echo 503 >"$api/status-tags-v1.46.0"
check '' v1.46.0 1 'HTTP 503'
rm "$api/status-tags-v1.46.0"

# GITHUB_TOKEN is sent on every API request.
rm -f "$api/auth-log"
GITHUB_TOKEN=secret resolve sh v1.46 >/dev/null
if [ "$(sort -u "$api/auth-log" 2>/dev/null)" = "Authorization: Bearer secret" ] &&
  [ "$(wc -l <"$api/auth-log" | tr -d ' ')" = "$(requests)" ]; then
  echo "ok   GITHUB_TOKEN is sent"
else
  fail "GITHUB_TOKEN is not sent on every request"
fi

if [ -n "$(ls -A "$work/bin" 2>/dev/null)" ]; then
  fail "something was installed into $work/bin"
fi

if [ "$failures" -ne 0 ]; then
  echo "$failures failure(s)"
  exit 1
fi
echo "all passed"
