#!/usr/bin/env bash
# Whole-program dead-code check for the root module: every function that no
# path from main() reaches, on any platform we build for.
#
# golangci-lint's `unused` already covers unexported identifiers, but only
# within one package, so an exported function whose last caller went away is
# invisible to it. golang.org/x/tools/cmd/deadcode builds the call graph from
# main() and sees those too. Astro Desktop runs the same tool at the same
# version from its pre-push hook.
#
# Three things keep it from reporting what is not dead:
#
# Each platform compiles different files, so a function can be dead on one and
# alive on another — the proxy daemon, say, which nothing calls on Windows.
# This reports only what is dead on every platform in DEADCODE_GOOS that
# compiles it. Darwin is
# not in the default list because the root cannot be analyzed for it from a
# linux host, which is what CI is (fsevents is cgo-only there; see
# LINT_GOOS_ROOT in the Makefile). Today its result is identical to linux's. A
# function whose only callers are in _darwin.go files would be reported, and
# `DEADCODE_GOOS="linux windows darwin"` on a Mac is the way to confirm that.
#
# The pkg/* sub-modules are libraries, and Astro Desktop is a caller this
# repository cannot see: about twenty of their exported functions are called
# from there and from nowhere here. So they are left out. Astro Desktop checks
# them from its side, where both programs are in view.
#
# Test callers do not count, which is deadcode's default and the right one:
# production code that only its own tests call is dead. Test support that other
# packages' tests import cannot live in a _test.go file, so it is listed in
# TEST_SUPPORT below instead.
#
# Deleting what this reports often exposes more: a function whose only caller
# was just removed. Run it again until it is clean.
set -euo pipefail

cd "$(dirname "$0")/.."

: "${DEADCODE_VERSION:?set DEADCODE_VERSION, or run this through make deadcode}"
DEADCODE_GOOS=${DEADCODE_GOOS:-linux windows}

# Shared test helpers, compiled into non-test files because tests in other
# packages import them.
TEST_SUPPORT=(
  pkg/testing/
  config/config_test_utils.go
)

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# Installed rather than `go run`: GOOS has to say what is ANALYZED, and
# `GOOS=windows go run` reads it as what to build — it cross-compiles deadcode
# and then fails to exec it. Same reason as lint-goos.
GOBIN="$work/bin" go install "golang.org/x/tools/cmd/deadcode@${DEADCODE_VERSION}"

exclude=()
for mod in pkg/*/go.mod; do
  exclude+=("$(dirname "$mod")/")
done
exclude+=("${TEST_SUPPORT[@]}")
printf '%s\n' "${exclude[@]}" | sed 's/^/^/' >"$work/exclude"

for goos in $DEADCODE_GOOS; do
  echo "==> deadcode ($goos)" >&2
  GOOS=$goos "$work/bin/deadcode" ./... >"$work/raw"
  { grep -v -f "$work/exclude" "$work/raw" || true; } | sort >"$work/$goos.dead"
  GOOS=$goos go list -f '{{$d := .Dir}}{{range .GoFiles}}{{$d}}/{{.}}{{"\n"}}{{end}}' ./... |
    sed "s#^$PWD/##" | sort >"$work/$goos.files"
done

# Dead means reported by every platform that compiles the file. Intersecting
# the whole reports instead would keep a function in a _unix.go file forever:
# Windows never compiles it, so Windows never reports it.
sort -u "$work"/*.dead | while IFS= read -r line; do
  file=${line%%:*}
  for goos in $DEADCODE_GOOS; do
    if grep -qxF "$file" "$work/$goos.files" && ! grep -qxF "$line" "$work/$goos.dead"; then
      continue 2
    fi
  done
  echo "$line"
done >"$work/dead"

if [ -s "$work/dead" ]; then
  cat "$work/dead"
  echo >&2
  echo "deadcode: $(wc -l <"$work/dead" | tr -d ' ') unreachable function(s). Delete them — and their tests — or, for shared test support, add the file to TEST_SUPPORT in scripts/deadcode.sh." >&2
  exit 1
fi
echo "deadcode: clean ($DEADCODE_GOOS)" >&2
