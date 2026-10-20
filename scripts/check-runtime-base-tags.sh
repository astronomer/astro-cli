#!/usr/bin/env bash
# Check that every runtime base a generated build can start FROM is published.
#
#   scripts/check-runtime-base-tags.sh
#
# A generated build (a project with no Dockerfile of its own) builds FROM the
# runtime's -base tag and writes the install steps out itself, rather than
# relying on the ONBUILD tag's triggers (pkg/imagebuild). So the -base tag of
# every build the CLI can pick has to exist, in the registry the CLI pulls it
# from:
#
#   - Airflow 3, astrocrpublic.azurecr.io/runtime: each non-yanked build in the
#     catalog's runtimeVersionsV3, each build's -python-X.Y flavor for every
#     Python it lists, and each series tag (3.1), which a pin with no
#     [tool.astro] runtime floats on.
#   - Airflow 2, quay.io/astronomer/astro-runtime: each non-yanked build from
#     runtime 9 (Airflow 2.7, the floor in pkg/imagebuild/runtimeversions.go).
#
# Not covered:
#
#   - A runtime a platform offers that the public catalog does not list. A
#     deploy builds on the newest build Houston (APC) or the Deployment's
#     options (Astro) offer (internal/deploybuild/offered.go), and a build the
#     platform has but the catalog lacks is outside what this reads.
#   - Airflow 2 -python-X.Y flavors. The catalog lists no Python for an
#     Airflow 2 build, so the CLI never picks one (RuntimeImageForPython builds
#     an Airflow 2 image on its runtime's default Python), and there is no
#     list of them to check.
#
# Prints each missing tag, says whether the ONBUILD tag beside it exists, and
# exits 1 when any -base tag is missing. Needs curl and jq, and the network.
set -euo pipefail

catalog_url="${ASTRO_RUNTIME_CATALOG_URL:-https://updates.astronomer.io/astronomer-runtime}"

accept='application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json'

# token prints an anonymous pull token for repo from realm.
token() {
  local realm="$1" service="$2" repo="$3"
  curl -fsS --max-time 30 "${realm}?service=${service}&scope=repository:${repo}:pull" |
    jq -r '.token // .access_token'
}

# status prints the HTTP status of a manifest HEAD for host/repo:tag.
status() {
  local host="$1" repo="$2" tag="$3" tok="$4"
  curl -s -o /dev/null -w '%{http_code}' --max-time 30 -I \
    -H "Accept: ${accept}" -H "Authorization: Bearer ${tok}" \
    "https://${host}/v2/${repo}/manifests/${tag}"
}

catalog="$(curl -fsS --max-time 30 "$catalog_url")"

af3_host=astrocrpublic.azurecr.io
af3_repo=runtime
af3_tok="$(token "https://${af3_host}/oauth2/token" "$af3_host" "$af3_repo")"

af2_host=quay.io
af2_repo=astronomer/astro-runtime
af2_tok="$(token "https://quay.io/v2/auth" quay.io "$af2_repo")"

# Every tag a build can start FROM, as "<registry> <tag>" lines.
af3_tags="$(jq -r '
  .runtimeVersionsV3 | to_entries | map(select(.value.metadata.yanked != true)) |
  ( map(.key) + map(.key as $b | (.value.metadata.pythonVersions // [])[] | "\($b)-python-\(.)")
    + (map(.key | split("-")[0]) | unique) ) | .[]' <<<"$catalog")"
af2_tags="$(jq -r '
  .runtimeVersions | to_entries |
  map(select(.value.metadata.yanked != true and ((.key | split(".")[0] | tonumber) >= 9))) |
  .[].key' <<<"$catalog")"

checked=0
missing=0
check() {
  local host="$1" repo="$2" tok="$3" tag="$4"
  checked=$((checked + 1))
  if [[ "$(status "$host" "$repo" "${tag}-base" "$tok")" == 200 ]]; then
    return
  fi
  missing=$((missing + 1))
  if [[ "$(status "$host" "$repo" "$tag" "$tok")" == 200 ]]; then
    echo "missing ${host}/${repo}:${tag}-base (${tag} is published)"
  else
    echo "missing ${host}/${repo}:${tag}-base (${tag} is not published either)"
  fi
}

while read -r tag; do
  [[ -n "$tag" ]] && check "$af3_host" "$af3_repo" "$af3_tok" "$tag"
done <<<"$af3_tags"
while read -r tag; do
  [[ -n "$tag" ]] && check "$af2_host" "$af2_repo" "$af2_tok" "$tag"
done <<<"$af2_tags"

echo "checked ${checked} -base tags, ${missing} missing"
[[ "$missing" -eq 0 ]]
