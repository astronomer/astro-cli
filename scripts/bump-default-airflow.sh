#!/usr/bin/env bash
# Keep runtimeversions.FallbackAirflowSeries on the newest supported Airflow
# series, the answer astro init gives when it cannot read the runtime catalog.
#
#   scripts/bump-default-airflow.sh              raise the constant to the live catalog's answer
#   scripts/bump-default-airflow.sh --latest F   print the rule's answer for catalog file F
#
# The rule is LatestAirflowSeries's in pkg/runtimeversions/catalog.go, written
# again in jq: the highest Airflow 3 series with at least one build that is
# stable, not yanked, and released on or before today (UTC). Compared by
# version, never by date, and with no cooldown. CI runs --latest against
# pkg/runtimeversions/testdata/catalog.json, the fixture the Go tests pin the
# same rule to, so the two cannot drift apart unseen.
#
# The constant only ever moves up. When the catalog holds a series back (yanks
# or deprecates its builds) after the constant reached it, every client with a
# current catalog follows the catalog anyway; the constant only decides what an
# offline client, or one holding a stale copy older than it, starts on. Lowering
# it for a hold-back is a judgment call left to a person.
#
# Prints the series now in the file. .github/workflows/bump-default-airflow.yml
# opens the PR when the file changed.
set -euo pipefail

cd "$(dirname "$0")/.."

const_file=pkg/runtimeversions/default.go
url="${ASTRO_RUNTIME_VERSIONS_URL:-https://updates.astronomer.io/astronomer-runtime}"
# TODAY lets a check stand on a fixed day; the default is the real one.
today="${TODAY:-$(date -u +%Y-%m-%d)}"

latest() {
  jq -r --arg today "$today" '
    def ver: split(".") | map(tonumber? // -1);
    [ (.runtimeVersions // {}), (.runtimeVersionsV3 // {})
      | to_entries[] | .value.metadata
      | select(.channel == "stable")
      | select((.yanked // false) | not)
      | select((.releaseDate // "") | test("^[0-9]{4}-[0-9]{2}-[0-9]{2}$"))
      | select(.releaseDate <= $today)
      | (.airflowVersion // "") | split(".")
      | select(length >= 2 and .[0] == "3" and .[1] != "")
      | .[0:2] | join(".")
    ] | unique | sort_by(ver) | last // empty
  ' "$1"
}

if [ "${1:-}" = "--latest" ]; then
  if [ -z "${2:-}" ]; then
    echo "usage: $0 --latest <catalog.json>" >&2
    exit 2
  fi
  latest "$2"
  exit 0
fi

current="$(sed -n 's/^const FallbackAirflowSeries = "\(.*\)"$/\1/p' "$const_file")"
if [ -z "$current" ]; then
  echo "error: no FallbackAirflowSeries constant in $const_file" >&2
  exit 1
fi

catalog="$(mktemp)"
trap 'rm -f "$catalog"' EXIT
curl -fsSL --max-time 30 -A "astro-cli-bump-default-airflow" "$url" -o "$catalog"

next="$(latest "$catalog")"
if [ -z "$next" ]; then
  echo "error: the catalog at $url names no qualifying Airflow 3 series" >&2
  exit 1
fi

# Move up only: sort -V puts the higher of the two last.
if [ "$next" != "$current" ] && [ "$(printf '%s\n%s\n' "$current" "$next" | sort -V | tail -1)" = "$next" ]; then
  sed -i.bak "s/^const FallbackAirflowSeries = \".*\"$/const FallbackAirflowSeries = \"$next\"/" "$const_file"
  rm -f "$const_file.bak"
  echo "$next"
else
  echo "$current"
fi
