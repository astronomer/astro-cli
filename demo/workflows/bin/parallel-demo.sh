#!/usr/bin/env bash
# Set up workflow 3: two copies of demo/project plus a git worktree, all
# running at once. Prints `astro local list` at the end.
#
#   ./demo/workflows/bin/parallel-demo.sh          # set up and start
#   ./demo/workflows/bin/parallel-demo.sh --clean  # stop and remove
#
# Everything lands under $WORKDIR (default /tmp/astro-parallel-demo).

set -euo pipefail

WORKDIR="${WORKDIR:-/tmp/astro-parallel-demo}"
ASTRO="${ASTRO:-astro}"
PROJECT_SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../project" && pwd)"

# The three values demo/project declares with no default. Fake but well-formed;
# the SQLite warehouse is a real file the DAGs write to.
set_env_values() {
  "$ASTRO" local env variable set WAREHOUSE_URI --value 'sqlite:///include/warehouse.db' >/dev/null
  "$ASTRO" local env connection set warehouse --value 'sqlite:///include/warehouse.db' >/dev/null
  "$ASTRO" local env variable set ORDERS_API_URL --value 'https://catalog.example.com/products' >/dev/null
}

stop_all() {
  for d in "$WORKDIR/orders-demo" "$WORKDIR/orders-experiment" "$WORKDIR/orders-wt"; do
    [ -d "$d" ] || continue
    (cd "$d" && "$ASTRO" local stop >/dev/null 2>&1) || true
  done
  "$ASTRO" local list --clean >/dev/null 2>&1 || true
}

if [ "${1:-}" = "--clean" ]; then
  stop_all
  rm -rf "$WORKDIR"
  echo "removed $WORKDIR"
  exit 0
fi

stop_all
rm -rf "$WORKDIR"
mkdir -p "$WORKDIR"

echo "==> project 1: orders-demo"
cp -R "$PROJECT_SRC" "$WORKDIR/orders-demo"
cd "$WORKDIR/orders-demo"
set_env_values
"$ASTRO" local start

echo
echo "==> project 2: orders-experiment (same code, different directory)"
cp -R "$PROJECT_SRC" "$WORKDIR/orders-experiment"
cd "$WORKDIR/orders-experiment"
set_env_values
"$ASTRO" local start

echo
echo "==> a git worktree of project 1"
cd "$WORKDIR/orders-demo"
# The project .gitignore already excludes .venv/, include/out/, the SQLite
# file, and dist/ — so this commits the source and nothing derived. Do not
# delete .venv here: Airflow is running out of it.
git init -q
git add -A
git -c user.email=demo@example.com -c user.name=demo commit -qm "demo project"
git worktree add -q "$WORKDIR/orders-wt" -b feature-x
cd "$WORKDIR/orders-wt"
set_env_values
"$ASTRO" local start

echo
echo "==> three Airflows, one machine"
"$ASTRO" local list

echo
echo "Stop them all with: $0 --clean"
