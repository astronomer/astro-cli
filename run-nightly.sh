#!/bin/sh
set -e
# run-nightly.sh: download (cached) and run an astronomer/astro-cli nightly build
# without installing it — the moral equivalent of `uvx astro-cli@nightly`.
#
# Portable POSIX shell helpers adapted from godownloader.sh in this repo.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/astronomer/astro-cli/main/run-nightly.sh | bash -s -- version
#   curl -sSL https://raw.githubusercontent.com/astronomer/astro-cli/main/run-nightly.sh | bash -s -- -t v1.44.0-nightly.20260710 dev start
#

usage() {
  this=$1
  cat <<EOF
$this: download and run an astronomer/astro-cli nightly build

Usage: $this [-t tag] [-c cachedir] [-f] [-n] [-d] [args...]
  -t tag       release tag to run (e.g. v1.44.0-nightly.20260710).
               Defaults to the most recent nightly release. Any release
               tag from https://github.com/astronomer/astro-cli/releases works.
  -c cachedir  directory to cache downloaded binaries.
               Defaults to \$ASTRO_NIGHTLY_CACHE_DIR, then ~/.cache/astro-nightly
  -f           force re-download even if the binary is already cached
  -n           resolve the tag and print the cached binary path, but do not run it
  -d           turn on debug logging
  -h           show this help

  [args...]    everything else is passed through to the astro binary

Environment:
  ASTRO_NIGHTLY_TAG        same as -t
  ASTRO_NIGHTLY_CACHE_DIR  same as -c
  GITHUB_TOKEN             used for GitHub API requests if set (avoids rate limits)

Cached binaries accumulate under the cache directory (one subdirectory per
tag); remove the directory to reclaim space.
EOF
  exit 2
}

parse_args() {
  TAG=${ASTRO_NIGHTLY_TAG:-}
  CACHE_DIR=${ASTRO_NIGHTLY_CACHE_DIR:-${XDG_CACHE_HOME:-$HOME/.cache}/astro-nightly}
  FORCE=0
  RESOLVE_ONLY=0
  while getopts "t:c:fndxh?" arg; do
    case "$arg" in
      t) TAG="$OPTARG" ;;
      c) CACHE_DIR="$OPTARG" ;;
      f) FORCE=1 ;;
      n) RESOLVE_ONLY=1 ;;
      d) log_set_priority 10 ;;
      x) set -x ;;
      h | \?) usage "$0" ;;
    esac
  done
  shift $((OPTIND - 1))
  # remaining args are passed through to the astro binary
  ASTRO_ARGS_SHIFT=$((OPTIND - 1))
}

# resolve TAG to the most recent nightly release when not specified
resolve_tag() {
  if [ -z "${TAG}" ]; then
    log_info "resolving latest nightly release"
    TAG=$(github_latest_nightly "$OWNER/$REPO")
    if test -z "$TAG"; then
      log_crit "unable to find a nightly release - see https://github.com/${PREFIX}/releases for details"
      exit 1
    fi
    log_info "using latest nightly: ${TAG}"
  else
    # accept tags with or without the leading v
    case "$TAG" in
      v*) : ;;
      *) TAG="v${TAG}" ;;
    esac
  fi
  VERSION=${TAG#v}
}

# list releases and return the tag of the most recent one containing "-nightly."
# (the releases API returns newest first)
github_latest_nightly() {
  owner_repo=$1
  json=$(github_api_copy "https://api.github.com/repos/${owner_repo}/releases?per_page=100") || return 1
  echo "$json" |
  tr ',' '\n' |
  grep -o '"tag_name": *"[^"]*-nightly\.[^"]*"' |
  head -n 1 |
  sed 's/.*"tag_name": *"//;s/"$//'
}

github_api_copy() {
  if [ -n "${GITHUB_TOKEN:-}" ]; then
    http_copy "$1" "Authorization: Bearer ${GITHUB_TOKEN}"
  else
    http_copy "$1"
  fi
}

# download the archive for OS/ARCH into the cache, verifying the checksum.
# no-op if the binary is already cached (unless -f).
fetch() {
  BINDIR="${CACHE_DIR}/${TAG}"
  BINEXE="${BINARY}"
  if [ "$OS" = "windows" ]; then
    BINEXE="${BINEXE}.exe"
  fi
  BIN="${BINDIR}/${BINEXE}"
  if [ -x "${BIN}" ] && [ "$FORCE" != "1" ]; then
    log_debug "using cached binary ${BIN}"
    return 0
  fi

  NAME=${BINARY}_${VERSION}_${OS}_${ARCH}
  ARCHIVE=${NAME}.${FORMAT}
  ARCHIVE_URL=${GITHUB_DOWNLOAD}/${TAG}/${ARCHIVE}
  CHECKSUM=${PROJECT_NAME}_${VERSION}_checksums.txt
  CHECKSUM_URL=${GITHUB_DOWNLOAD}/${TAG}/${CHECKSUM}

  tmpdir=$(mktemp -d)
  trap 'rm -rf "${tmpdir}"' EXIT
  log_info "downloading ${ARCHIVE_URL}"
  http_download "${tmpdir}/${ARCHIVE}" "${ARCHIVE_URL}" || {
    log_crit "failed to download ${ARCHIVE_URL} - does ${TAG} exist for ${OS}/${ARCH}?"
    exit 1
  }
  http_download "${tmpdir}/${CHECKSUM}" "${CHECKSUM_URL}" || {
    log_crit "failed to download ${CHECKSUM_URL}"
    exit 1
  }
  hash_sha256_verify "${tmpdir}/${ARCHIVE}" "${tmpdir}/${CHECKSUM}"
  if [ "$FORMAT" = "exe" ]; then
    # windows binaries are published as bare .exe files; no extraction needed
    mv "${tmpdir}/${ARCHIVE}" "${tmpdir}/${BINEXE}"
  else
    (cd "${tmpdir}" && untar "${ARCHIVE}")
  fi
  test ! -d "${BINDIR}" && install -d "${BINDIR}"
  install "${tmpdir}/${BINEXE}" "${BINDIR}/"
  rm -rf "${tmpdir}"
  trap - EXIT
  log_info "cached ${BIN}"
}

run() {
  if [ "$RESOLVE_ONLY" = "1" ]; then
    echo "${BIN}"
    exit 0
  fi
  log_debug "exec ${BIN} $*"
  # When this script itself is streamed over stdin (curl ... | bash), stdin is
  # not usable by astro. Re-attach the terminal so interactive prompts work.
  # When executed as a saved file, leave stdin alone so piped input still works.
  case "$0" in
    */run-nightly.sh | run-nightly.sh) : ;;
    *)
      if [ ! -t 0 ] && (: </dev/tty) 2>/dev/null; then
        exec "${BIN}" "$@" </dev/tty
      fi
      ;;
  esac
  exec "${BIN}" "$@"
}

cat /dev/null <<EOF
------------------------------------------------------------------------
https://github.com/client9/shlib - portable posix shell functions
Public domain - http://unlicense.org
https://github.com/client9/shlib/blob/master/LICENSE.md
but credit (and pull requests) appreciated.
------------------------------------------------------------------------
EOF
is_command() {
  command -v "$1" >/dev/null
}
echoerr() {
  echo "$@" 1>&2
}
log_prefix() {
  echo "$0"
}
_logp=6
log_set_priority() {
  _logp="$1"
}
log_priority() {
  if test -z "$1"; then
    echo "$_logp"
    return
  fi
  [ "$1" -le "$_logp" ]
}
log_tag() {
  case $1 in
    0) echo "emerg" ;;
    1) echo "alert" ;;
    2) echo "crit" ;;
    3) echo "err" ;;
    4) echo "warning" ;;
    5) echo "notice" ;;
    6) echo "info" ;;
    7) echo "debug" ;;
    *) echo "$1" ;;
  esac
}
log_debug() {
  log_priority 7 || return 0
  echoerr "$(log_prefix)" "$(log_tag 7)" "$@"
}
log_info() {
  log_priority 6 || return 0
  echoerr "$(log_prefix)" "$(log_tag 6)" "$@"
}
log_err() {
  log_priority 3 || return 0
  echoerr "$(log_prefix)" "$(log_tag 3)" "$@"
}
log_crit() {
  log_priority 2 || return 0
  echoerr "$(log_prefix)" "$(log_tag 2)" "$@"
}
uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    cygwin_nt*) os="windows" ;;
    mingw*) os="windows" ;;
    msys_nt*) os="windows" ;;
  esac
  echo "$os"
}
uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}
uname_os_check() {
  os=$(uname_os)
  case "$os" in
    darwin) return 0 ;;
    linux) return 0 ;;
    windows) return 0 ;;
  esac
  log_crit "os '$(uname -s)' ('$os') is not supported by nightly builds.  Make sure this script is up-to-date and file request at https://github.com/${PREFIX}/issues/new"
  return 1
}
uname_arch_check() {
  arch=$(uname_arch)
  case "$arch" in
    386) return 0 ;;
    amd64) return 0 ;;
    arm64) return 0 ;;
  esac
  log_crit "cpu architecture '$(uname -m)' ('$arch') is not supported by nightly builds.  Make sure this script is up-to-date and file request at https://github.com/${PREFIX}/issues/new"
  return 1
}
untar() {
  tarball=$1
  case "${tarball}" in
    *.tar.gz | *.tgz) tar --no-same-owner -xzf "${tarball}" ;;
    *.tar) tar --no-same-owner -xf "${tarball}" ;;
    *.zip) unzip "${tarball}" ;;
    *)
      log_err "untar unknown archive format for ${tarball}"
      return 1
      ;;
  esac
}
http_download_curl() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    code=$(curl -w '%{http_code}' -sL -o "$local_file" "$source_url")
  else
    code=$(curl -w '%{http_code}' -sL -H "$header" -o "$local_file" "$source_url")
  fi
  if [ "$code" != "200" ]; then
    log_debug "http_download_curl received HTTP status $code"
    return 1
  fi
  return 0
}
http_download_wget() {
  local_file=$1
  source_url=$2
  header=$3
  if [ -z "$header" ]; then
    wget -q -O "$local_file" "$source_url"
  else
    wget -q --header "$header" -O "$local_file" "$source_url"
  fi
}
http_download() {
  log_debug "http_download $2"
  if is_command curl; then
    http_download_curl "$@"
    return
  elif is_command wget; then
    http_download_wget "$@"
    return
  fi
  log_crit "http_download unable to find wget or curl"
  return 1
}
http_copy() {
  tmp=$(mktemp)
  http_download "${tmp}" "$1" "$2" || return 1
  body=$(cat "$tmp")
  rm -f "${tmp}"
  echo "$body"
}
hash_sha256() {
  TARGET=${1:-/dev/stdin}
  if is_command gsha256sum; then
    hash=$(gsha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command sha256sum; then
    hash=$(sha256sum "$TARGET") || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command shasum; then
    hash=$(shasum -a 256 "$TARGET" 2>/dev/null) || return 1
    echo "$hash" | cut -d ' ' -f 1
  elif is_command openssl; then
    hash=$(openssl dgst -sha256 "$TARGET") || return 1
    echo "$hash" | rev | cut -d ' ' -f 1 | rev
  else
    log_crit "hash_sha256 unable to find command to compute sha-256 hash"
    return 1
  fi
}
hash_sha256_verify() {
  TARGET=$1
  checksums=$2
  if [ -z "$checksums" ]; then
    log_err "hash_sha256_verify checksum file not specified in arg2"
    return 1
  fi
  BASENAME=${TARGET##*/}
  want=$(grep "${BASENAME}" "${checksums}" 2>/dev/null | tr '\t' ' ' | cut -d ' ' -f 1)
  if [ -z "$want" ]; then
    log_err "hash_sha256_verify unable to find checksum for '${TARGET}' in '${checksums}'"
    return 1
  fi
  got=$(hash_sha256 "$TARGET")
  if [ "$want" != "$got" ]; then
    log_err "hash_sha256_verify checksum for '$TARGET' did not verify ${want} vs $got"
    return 1
  fi
}
cat /dev/null <<EOF
------------------------------------------------------------------------
End of functions from https://github.com/client9/shlib
------------------------------------------------------------------------
EOF

PROJECT_NAME="astro"
OWNER=astronomer
REPO="astro-cli"
BINARY=astro
FORMAT=tar.gz
OS=$(uname_os)
ARCH=$(uname_arch)
PREFIX="$OWNER/$REPO"

# use in logging routines
log_prefix() {
  echo "$PREFIX"
}
PLATFORM="${OS}/${ARCH}"
GITHUB_DOWNLOAD=https://github.com/${OWNER}/${REPO}/releases/download

uname_os_check "$OS"
uname_arch_check "$ARCH"

# windows binaries are published as bare .exe files rather than tarballs
if [ "$OS" = "windows" ]; then
  FORMAT=exe
fi
# nightly builds do not ship a windows/386 binary
if [ "$PLATFORM" = "windows/386" ]; then
  log_crit "platform ${PLATFORM} is not supported by nightly builds"
  exit 1
fi

parse_args "$@"
shift "$ASTRO_ARGS_SHIFT"

resolve_tag

log_info "using version ${VERSION} (${TAG}) for ${OS}/${ARCH}"

fetch

run "$@"
