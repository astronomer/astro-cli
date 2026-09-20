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

# The suite budget scales with the tier, because the point of the per-command
# bounds in the harness is that a hung command fails its own test with its own
# output. That only holds while the suite outlives one of them: a -timeout
# panic dumps goroutines instead, and does not run t.Cleanup, so tier 2 would
# also leave a real Airflow holding a port. Tier 2's bound is ten minutes (see
# slowCommandTimeout), so the suite gets thirty.
#
# Tier 3 gets more again. Its cases carry the same ten-minute bound, and the
# first of them pays for a 1.34 GB image pull inside that bound on a runner
# that starts with nothing cached — and a -timeout panic there leaves
# containers, a volume and the image behind, which is the state this tier is
# least able to afford leaking.
timeout=15m
case "${ASTRO_E2E_MAX_TIER:-0}" in
  0 | 1) ;;
  2) timeout=30m ;;
  *) timeout=45m ;;
esac

exec go test -count=1 -race -shuffle=on -timeout="$timeout" -tags e2e "$@" ./...
