#!/usr/bin/env bash
# Workflow 5, the part that can run unattended: pre-flight the demo project
# against MWAA and Composer, then build all three artifacts.
#
#   ./demo/workflows/bin/package-all.sh
#
# Needs Docker for the astro (image) target; skip it with --no-image. The mwaa
# and composer targets need no Docker. The MWAA constraints resolution needs
# the network and is skipped, not failed, offline.

set -euo pipefail

ASTRO="${ASTRO:-astro}"
PROJECT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../project" && pwd)"
WITH_IMAGE=1
[ "${1:-}" = "--no-image" ] && WITH_IMAGE=0

cd "$PROJECT"

echo "==> pre-flight: check the DAGs against the Airflow each platform runs"
"$ASTRO" local check --target mwaa,composer

echo
echo "==> astro package mwaa — an S3-shaped tree"
"$ASTRO" package mwaa

echo
echo "==> astro package composer — a GCS-shaped tree"
"$ASTRO" package composer

if [ "$WITH_IMAGE" -eq 1 ]; then
  echo
  echo "==> astro package — a deployable image, built from the manifest"
  "$ASTRO" package
fi

echo
echo "==> what got built"
find dist -maxdepth 2 -mindepth 1 | sort
