#!/usr/bin/env bash
# Run the e2e suite: the built CLI, driven through argv and asserted on by its
# stdout and exit code.
#
# It is a module of its own (e2e/go.mod), so `go test ./...` from the root does
# not reach it, and every test file is behind the `e2e` build tag, so even a run
# inside that directory does not spawn CLI processes by accident. Two reasons it
# needs its own invocation, and both are easy to forget.
#
# One script rather than a command written per caller, because it has three: the
# Makefile target and both CI jobs. On Windows the job runs it under
# `shell: bash` — the runner has Git Bash but no GNU make, which is why CI calls
# this directly instead of going through the Makefile. Same shape as
# test-submodules.sh, for the same reason.
#
# ASTRO_E2E_MAX_TIER selects how much the run pays for: 0 is hermetic (temp
# directories and nothing else) and is the default, and each tier above it wants
# a tool — uv, a real Airflow, Docker, cloud credentials. See e2e/doc.go.
set -euo pipefail

cd "$(dirname "$0")/../e2e"

# A -race binary sleeps one second before it exits, to catch races with C
# atexit() handlers that Go does not have (golang/go#20364). See the Makefile's
# `test` target, which made the same change for the same reason.
export GORACE=atexit_sleep_ms=0

exec go test -count=1 -race -shuffle=on -timeout=15m -tags e2e "$@" ./...
