#!/usr/bin/env bash
# Move the uv image the install stage copies uv from to a newer uv release.
#
#   scripts/bump-uv-image.sh                 the newest release past the cooldown
#   scripts/bump-uv-image.sh --force 0.12.3  that release, cooldown or not
#
# The pin is one line, ghcr.io/astral-sh/uv:<version>@sha256:<index digest>, in
# the uv-image.txt the CLI embeds (UVImage) and every install stage's
# ASTRO_UV_IMAGE build argument defaults to. A release is adopted only once it
# is COOLDOWN_DAYS old, or MAJOR_COOLDOWN_DAYS for a major bump (a minor one,
# while uv is 0.x), the supply-chain cooldown Astro Desktop's bump-uv.yml uses:
# updates land weekly, never younger than that. --force (or FORCE_VERSION) is
# the security fast-track around it. The digest is the multi-arch index's, read
# from ghcr.io, so a build pulls exactly the image this script saw.
#
# Prints the new pin when it moved, nothing when it is already current.
# .github/workflows/bump-uv-image.yml opens the PR. Needs curl and gh, the
# latter authenticated (GH_TOKEN) to read astral-sh/uv's releases.
set -euo pipefail

cd "$(dirname "$0")/.."

repo=astral-sh/uv
image=ghcr.io/astral-sh/uv
cooldown_days="${COOLDOWN_DAYS:-7}"
major_cooldown_days="${MAJOR_COOLDOWN_DAYS:-14}"
force="${FORCE_VERSION:-}"
if [[ "${1:-}" == "--force" ]]; then
  force="${2:?bump-uv-image: --force wants a version, such as 0.12.3}"
fi
force="${force#v}"

pin_file=""
for f in pkg/airflowrt/uv-image.txt pkg/imagebuild/uv-image.txt; do
  if [[ -f "$f" ]]; then
    pin_file="$f"
    break
  fi
done
if [[ -z "$pin_file" ]]; then
  echo "bump-uv-image: no uv-image.txt found" >&2
  exit 1
fi

pattern="^${image//./\\.}:([0-9]+\\.[0-9]+\\.[0-9]+)@sha256:[0-9a-f]{64}\$"
current_pin="$(tr -d '\r\n' <"$pin_file")"
if ! [[ "$current_pin" =~ $pattern ]]; then
  echo "bump-uv-image: $pin_file is not ${image}:<version>@sha256:<digest>: '$current_pin'" >&2
  exit 1
fi
current="${BASH_REMATCH[1]}"

# epoch_of reads an ISO-8601 UTC time as epoch seconds, with GNU date (CI) or
# BSD date (macOS).
epoch_of() {
  date -u -d "$1" +%s 2>/dev/null || date -u -jf "%Y-%m-%dT%H:%M:%SZ" "$1" +%s
}

# is_major_bump is true for a new major, and, while uv is 0.x, a new minor:
# uv breaks things in 0.x minors.
is_major_bump() {
  local cur="$1" new="$2"
  [[ "${cur%%.*}" != "${new%%.*}" ]] && return 0
  if [[ "${cur%%.*}" == 0 ]]; then
    [[ "$(cut -d. -f2 <<<"$cur")" != "$(cut -d. -f2 <<<"$new")" ]] && return 0
  fi
  return 1
}

target="$force"
if [[ -z "$target" ]]; then
  now="$(date -u +%s)"
  # Published, stable releases, newest first, as "version<TAB>published_at".
  while IFS=$'\t' read -r ver published; do
    ver="${ver#v}"
    [[ "$ver" == "$current" ]] && break
    cooldown="$cooldown_days"
    is_major_bump "$current" "$ver" && cooldown="$major_cooldown_days"
    if (((now - $(epoch_of "$published")) / 86400 >= cooldown)); then
      target="$ver"
      break
    fi
  done < <(gh api "repos/$repo/releases?per_page=30" \
    --jq '[.[] | select(.prerelease == false and .draft == false)]
          | sort_by(.published_at) | reverse | .[] | [.tag_name, .published_at] | @tsv')
fi

if [[ -z "$target" || "$target" == "$current" ]]; then
  exit 0
fi

# The digest of the tag's multi-arch index, from ghcr.io's registry API with an
# anonymous pull token.
token="$(curl -fsSL "https://ghcr.io/token?scope=repository:${image#ghcr.io/}:pull" |
  sed -E 's/.*"token":"([^"]+)".*/\1/')"
digest="$(curl -fsSI \
  -H "Authorization: Bearer $token" \
  -H "Accept: application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json" \
  "https://ghcr.io/v2/${image#ghcr.io/}/manifests/$target" |
  tr -d '\r' | sed -nE 's/^[Dd]ocker-[Cc]ontent-[Dd]igest: *(sha256:[0-9a-f]{64})$/\1/p')"
if ! [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
  echo "bump-uv-image: no index digest for ${image}:$target: '$digest'" >&2
  exit 1
fi

new_pin="${image}:${target}@${digest}"
printf '%s\n' "$new_pin" >"$pin_file"
echo "$new_pin"
