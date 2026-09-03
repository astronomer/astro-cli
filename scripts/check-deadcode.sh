#!/usr/bin/env bash
#
# Run `deadcode` on the cli main module and fail if any unreachable functions
# are reported. deadcode exits 0 even when it finds dead code, so gate on its
# output.
#
# `-test` includes test executables, so functions reached only from tests are
# not reported as dead.
#
# Everything in the main module is checked except the `pkg/` tree: the
# sub-modules under `pkg/` with their own go.mod are independently versioned
# and consumed by external Go modules (e.g. astro-desktop), and `./...` skips
# them automatically; the rest of `pkg/` is import-style library code whose
# exported surface may have external consumers, so reachability from the CLI
# entry point is not a correctness signal for it.
set -euo pipefail

report="$(deadcode -test ./...)"
output="$(grep -vE '^pkg/' <<<"$report" || true)"

if [[ -n "$output" ]]; then
  {
    echo "$output"
    echo
    echo "deadcode: unreachable functions reported above. Either delete them or,"
    echo "if they are intentionally exported public API with no in-module callers,"
    echo "add an exclusion in scripts/check-deadcode.sh."
  } >&2
  exit 1
fi
