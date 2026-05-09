#!/usr/bin/env bash
#
# Run `deadcode` on the cli main module and fail if any unreachable functions
# are reported. The library sub-modules (`pkg/airflowrt`, `pkg/astroauth`,
# `pkg/proxy`, `pkg/telemetry`, `astro-client-platform-core`) are excluded:
# they are independently versioned and consumed by external Go modules
# (e.g. astro-desktop), so reachability from `cmd/astro/main` is not a
# correctness signal for them.
#
# `-test` includes test executables. Without it, helpers like cloud's `List`
# functions show up as dead because only tests reach them.
set -euo pipefail

# Allow override of the deadcode binary via $DEADCODE (used by the prek hook
# which installs it into a hook-managed GOPATH).
DEADCODE_BIN="${DEADCODE:-deadcode}"

# Only enforce within the cli main module's binary-style directories. Library
# packages under pkg/ that lack a separate go.mod are still scanned because
# they are part of this module — but the regex below limits the report to
# directories whose contents are not intended as a stable public API.
SCOPE='^github.com/astronomer/astro-cli/(cmd|airflow|cloud|software|config|settings|context|houston|internal|version|airflow_versions|airflow-client|astro-client-core|astro-client-iam-core|docker)(/|$)'

output="$("$DEADCODE_BIN" -test -filter="$SCOPE" ./...)"

if [[ -n "$output" ]]; then
  echo "$output"
  echo
  echo "deadcode: unreachable functions reported above. Either delete them or," >&2
  echo "if they are intentionally exported public API, move them to a directory" >&2
  echo "outside the SCOPE in scripts/check-deadcode.sh." >&2
  exit 1
fi
