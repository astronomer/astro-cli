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

# release TAG [CHECKSUMS] [PRERELEASE] writes the releases/tags answer for
# TAG, with its checksums asset unless CHECKSUMS is "no" and GitHub's
# prerelease flag set to PRERELEASE (default false). Its body, ahead of the
# flag, would break echo under dash and quotes another release's asset name
# and the opposite flag.
release() {
  local tag=$1 sums=${2:-yes} pre=${3:-false} other=true
  if [ "$pre" = true ]; then other=false; fi
  {
    cat <<EOF
{
  "tag_name": "$tag",
  "name": "$tag",
  "body": "Fixes \\\\c in paths. See \\"name\\": \\"astro_${tag#v}_checksums.txt\\" and \\"prerelease\\": $other",
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
  ]
}
EOF
  } >"$api/tags-$tag"
}
# Every tag in the fixture repository and the state of its release: a release
# with checksums, a release still uploading them, a release with checksums
# that GitHub marks as a prerelease, or no release at all.
# Rows are in matching-refs text order: v1.46.10 before v1.46.9, v1.10.0 before v1.9.0.
while read -r tag state; do
  echo "$tag" >>"$api/all-tags"
  case "$state" in
    release) release "$tag" ;;
    uploading) release "$tag" no ;;
    flagged) release "$tag" yes true ;;
  esac
done <<'EOF'
v0.6.0 release
v0.6.1 release
v0.6.2 none
v0.100.0 release
v0.27.1 release
v0.27.2 release
v0.27.2-rc.1 release
v0.28.0 release
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
v5.0.0 release
v5.1.0 flagged
v6.0.0 flagged
v6.0.0-rc.1 release
v7.0.0 flagged
v7.1.0-rc.1 release
v8.0.0 uploading
v8.0.0-rc.1 release
v10.0.0-rc.1 release
v10.0.0-rc.2 uploading
v10.0.0-rc.3 uploading
v10.0.0-rc.4 uploading
v10.0.0-rc.5 uploading
v10.0.0-rc.6 uploading
v11.0.0 flagged
v11.0.1 flagged
v11.0.2 flagged
v11.0.3 flagged
v11.0.4 flagged
v11.0.5 flagged
v11.1.0-rc.1 release
v12.0.0 uploading
v12.1.0 flagged
v13.0.0-nightly.202610201230 release
v13.0.0-nightly.2999999999 release
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
    # No answer: curl prints 000 and exits 7 (could not connect).
    000)
      printf 000
      exit 7
      ;;
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
# linux/amd64 unless STUB_ARCH says otherwise, so a host that is darwin/arm64
# does not refuse v0.6.
cat >"$work/stub/uname" <<'EOF'
#!/bin/sh
case "$1" in
  -s) echo Linux ;;
  -m) echo "${STUB_ARCH:-x86_64}" ;;
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
check '' v3 6 'no stable release under v3. is ready'
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

echo 000 >"$api/status-refs-v1.46."
check '' v1.46 1 'could not reach GitHub (api.github.com)'
rm "$api/status-refs-v1.46."

# GitHub's prerelease flag outranks the tag's shape: a hyphen-less tag it
# marks is passed over for the next stable tag,
check v5.0.0 v5 3
# and, when no stable tag qualifies, competes with the pre-releases by semver.
check v6.0.0 v6 2
check v7.1.0-rc.1 v7 3

# A stable tag still publishing holds the pre-releases back, however ready
# its rc is,
check '' v8 2 'still uploading its checksums: v8.0.0'
# and the message tells it apart from one GitHub marks as a pre-release.
check '' v12 3 'marked as a pre-release on GitHub: v12.1.0'
# Five stable tags all marked pre-release move on to the pre-release tier,
# where v11.1.0-rc.1 outranks them.
check v11.1.0-rc.1 v11 7
# The pre-release tier checks 5 tags too: rc.6 to rc.2 are uploading, and
# rc.1 is never asked about.
check '' v10 6 'no pre-release under v10. is ready among the 5 highest'
# Numeric identifiers compare as numbers at any length, past the 2^31 where
# mawk's and busybox's %d stop.
check v13.0.0-nightly.202610201230 v13 2

# Among pre-releases the highest by semver wins: an rc outranks every nightly
# of its version, and a stable release outranks both.
cp "$api/all-tags" "$work/all-tags.bak"
for tag in v2.0.0-nightly.20261020 v2.0.0-nightly.20261030 v2.0.0-rc.1; do
  echo "$tag" >>"$api/all-tags"
  release "$tag"
done
check v2.0.0-rc.1 v2 2
echo v2.0.0 >>"$api/all-tags"
release v2.0.0
check v2.0.0 v2 2
cp "$work/all-tags.bak" "$api/all-tags"

# linux/arm64 builds start at 0.27.2, which version_gte decides: equal,
# greater (by number, not text: 0.100.0), less, and a pre-release of 0.27.2,
# which is below it.
export STUB_ARCH=aarch64
check v0.27.2 v0.27.2 1
check v0.28.0 v0.28.0 1
check v0.100.0 v0.100.0 1
check v1.46.0 v1.46.0 1
check '' v0.27.1 1 'platform linux/arm64 is not supported for 0.27.1'
check '' v0.27.2-rc.1 1 'platform linux/arm64 is not supported for 0.27.2-rc.1'
unset STUB_ARCH

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
