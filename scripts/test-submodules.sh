#!/usr/bin/env bash
# Run every pkg/* sub-module's tests.
#
# They need their own invocation because each is a separate Go module: `go test
# ./...` from the repository root does not descend into them. That is easy to
# forget, and it hid a real gap — the Linux CI job ran this and the Windows one
# did not, so pkg/localrt, pkg/secrets and the rest were never tested on Windows
# at all, despite holding the GOOS-specific filepath and keyring code most likely
# to differ there.
#
# One script rather than a loop written per caller, because it now has three:
# the Makefile target and both CI jobs. On Windows the job runs it under
# `shell: bash` — the runner has Git Bash but no GNU make, which is why CI calls
# this directly instead of going through the Makefile.
set -euo pipefail

cd "$(dirname "$0")/.."

# A -race binary sleeps one second before it exits, to catch races with C
# atexit() handlers that Go does not have (golang/go#20364). One second per
# test binary was most of this script's runtime.
export GORACE=atexit_sleep_ms=0

status=0
for mod in pkg/*/go.mod; do
  dir="$(dirname "$mod")"
  echo "==> ${dir}"
  # Not `set -e` on the first failure: one sub-module failing should not hide
  # whether the other fourteen are also broken, which is exactly the report
  # wanted the first time these run on a new platform.
  if ! (cd "${dir}" && go test -count=1 -race -shuffle=on -timeout=15m "$@" ./...); then
    status=1
    echo "!!! ${dir} FAILED"
  fi
done

exit "${status}"
