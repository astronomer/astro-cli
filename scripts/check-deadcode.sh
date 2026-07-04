#!/usr/bin/env bash
#
# Run `deadcode` on the cli main module and fail if any unreachable functions
# are reported. The library sub-modules (`pkg/airflowrt`, `pkg/astroauth`,
# `pkg/container`, `pkg/proxy`, `pkg/telemetry`) have their own go.mod, are
# independently versioned, and are consumed by external Go modules
# (e.g. astro-desktop), so `./...` skips them automatically and reachability
# from the cli entry point is not a correctness signal for them.
#
# `-test` includes test executables. Without it, helpers that are only
# reached from tests show up as dead.
set -euo pipefail

# Only enforce within the root main package and the cli main module's
# binary-style directories. Library packages under pkg/ that lack a separate
# go.mod are still scanned because they are part of this module, but the
# regex below limits the report to directories whose contents are not
# intended as a stable public API. When adding a new binary-style top-level
# directory, add it here so it gets deadcode coverage.
SCOPE='^github.com/astronomer/astro-cli($|/(cmd|airflow|cloud|software|config|settings|context|houston|internal|version|airflow_versions|airflow-client|astro-client-v1|astro-client-v1alpha1|docker)(/|$))'

# deadcode exits 0 even when it finds dead code, so gate on its output.
output="$(deadcode -test -filter="$SCOPE" ./...)"

if [[ -n "$output" ]]; then
  {
    echo "$output"
    echo
    echo "deadcode: unreachable functions reported above. Either delete them or,"
    echo "if they are intentionally exported public API, move them to a directory"
    echo "outside the SCOPE in scripts/check-deadcode.sh."
  } >&2
  exit 1
fi
