#!/usr/bin/env bash
# Put the freshly built astro binary where the shell will find it, or take it
# back off again.
#
# A script rather than a Makefile recipe, like scripts/test-submodules.sh: it
# needs `set -euo pipefail` and several path comparisons that a
# backslash-continued recipe cannot carry legibly.
#
# Usage: install.sh install|uninstall
# The Makefile owns the defaults and passes them in the environment.

set -euo pipefail

mode=${1:-}
case "$mode" in
  install | uninstall) ;;
  *)
    echo "usage: install.sh install|uninstall" >&2
    exit 1
    ;;
esac

name=${NAME:?the Makefile passes this}

# Compare paths in canonical form: a trailing slash, a relative INSTALL_DIR and a
# symlinked HOME all have to compare equal, or a good install calls itself
# shadowed. Fails when the directory does not exist, which doubles as an
# existence test.
canonicalize() {
  (cd "$1" 2>/dev/null && pwd -P)
}

# True when the shell actually searches this directory. PATH entries are
# canonicalized too, since PATH is written by hand and carries the same trailing
# slashes and symlinks a candidate does.
is_on_path() {
  local want entry entries
  want=$(canonicalize "$1") || return 1
  IFS=: read -ra entries <<<"$PATH"
  for entry in "${entries[@]}"; do
    if [ -n "$entry" ] && [ "$(canonicalize "$entry")" = "$want" ]; then
      return 0
    fi
  done
  return 1
}

# Where the binary goes. Two rules: an explicit choice is taken at its word,
# otherwise the first candidate the shell already searches wins.
#
# Choosing for PATH membership is the whole point. $GOPATH/bin is where `go
# install` writes and the obvious pick for a Go repository, but it is on almost
# nobody's PATH who does not already write Go, so choosing it unconditionally
# produces a binary that installs cleanly and cannot be run. ~/.local/bin leads
# the candidates for the same reason it is the last resort: it is the per-user
# convention systemd, pipx and uv already default to, so it earns a place on
# PATH beyond this one binary, where $GOPATH/bin does not.
#
# /usr/local/bin is not a candidate, tempting though it is for being on PATH
# nearly everywhere. It is root-owned on stock macOS and on Linux, so every
# install would need the sudo this script tells you not to use, and it is shared
# with whatever a real install put there -- the wrong home for a snapshot built
# off somebody's branch. INSTALL_DIR= still points at it.
#
# Prints nothing when there is nowhere to go.
resolve_dir() {
  local gobin gopath go_env candidate
  local candidates=()

  if [ -n "${INSTALL_DIR:-}" ]; then
    printf '%s\n' "$INSTALL_DIR"
    return
  fi

  # The environment beats `go env`, so uninstall still finds what install wrote
  # on a machine with no Go toolchain. One fork covers both fields; `go env`
  # prints a blank line for an unset one.
  gobin=${GOBIN:-}
  gopath=${GOPATH:-}
  if [ -z "$gobin" ] || [ -z "$gopath" ]; then
    go_env=$(go env GOBIN GOPATH 2>/dev/null || true)
    [ -n "$gobin" ] || gobin=${go_env%%$'\n'*}
    [ -n "$gopath" ] || gopath=${go_env#*$'\n'}
  fi

  # Setting GOBIN is already a statement about where Go binaries belong.
  if [ -n "$gobin" ]; then
    printf '%s\n' "$gobin"
    return
  fi

  # GOPATH is a colon-separated list; Go reads only the first entry.
  gopath=${gopath%%:*}

  if [ -n "${HOME:-}" ]; then
    candidates+=("$HOME/.local/bin")
  fi
  if [ -n "$gopath" ]; then
    candidates+=("$gopath/bin")
  fi

  for candidate in ${candidates+"${candidates[@]}"}; do
    if is_on_path "$candidate"; then
      printf '%s\n' "$candidate"
      return
    fi
  done

  # Nothing the shell already searches, so fall back to the head of the list and
  # let the reporting below tell the user to add it.
  printf '%s\n' "${candidates[0]:-}"
}

dir=$(resolve_dir)
if [ -z "$dir" ]; then
  echo "Error: nowhere to $mode to -- no INSTALL_DIR, GOBIN, GOPATH or HOME." >&2
  echo "Name the directory yourself: make $mode INSTALL_DIR=/usr/local/bin" >&2
  exit 1
fi

if [ "$mode" = install ]; then
  if [ ! -d "$dir" ] && ! mkdir -p "$dir" 2>/dev/null; then
    echo "Error: could not create $dir." >&2
    echo "Check the path, or name another: make install INSTALL_DIR=/usr/local/bin" >&2
    exit 1
  fi
elif [ ! -d "$dir" ]; then
  echo "Nothing to remove -- $dir does not exist."
  exit 0
fi

dir=$(canonicalize "$dir")
target="$dir/$name"

# A symlink here belongs to a package manager: Homebrew links its binaries rather
# than copying them. Installing over one would strand brew's copy and hand you a
# dev build under the name of your release, and removing one would take that
# release off PATH while brew still believed it owned the file. Nothing records
# what this script installed, so this is a proxy for "something else owns it"
# rather than a proof of it.
if [ -L "$target" ]; then
  echo "Error: $target is a symlink, so a package manager put it there." >&2
  echo "Leaving it alone. Remove it with that package manager (for Homebrew:" >&2
  echo "brew uninstall astro), or pass INSTALL_DIR= or NAME=astro-dev to work elsewhere." >&2
  exit 1
fi

if [ "$mode" = uninstall ]; then
  if [ -e "$target" ]; then
    rm -f "$target"
    echo "Removed $target"
  else
    echo "Nothing to remove at $target"
  fi
  exit 0
fi

output=${OUTPUT:?the Makefile passes this}
version=${VERSION:?the Makefile passes this}

if [ ! -w "$dir" ]; then
  echo "Error: $dir is not writable." >&2
  echo "Point INSTALL_DIR somewhere you own, or place it yourself:" >&2
  echo "  sudo install -m 0755 $output $target" >&2
  echo "Do not re-run this under sudo -- it would rebuild as root and leave you with a" >&2
  echo "root-owned Go build cache." >&2
  exit 1
fi

# Write beside the target and rename over it: a copy straight onto a binary that
# is currently running fails with ETXTBSY, while a rename replaces the directory
# entry and leaves the running process alone.
tmp=$(mktemp "$dir/.$name.XXXXXX")
trap 'rm -f "$tmp"' EXIT INT TERM
install -m 0755 "$output" "$tmp"
mv -f "$tmp" "$target"

echo "Installed $version to $target"

# Not-on-PATH and shadowed both look exactly like success until the version
# string surprises you, and they need different fixes, so say which one this is.
resolved=$(command -v "$name" 2>/dev/null || true)
if [ -n "$resolved" ] && [ "$(canonicalize "${resolved%/*}")" = "$dir" ]; then
  echo "Running $name now runs this build. Check with: $name version"
elif ! is_on_path "$dir"; then
  echo
  echo "Warning: $dir is not on your PATH, so this is not the $name you get by typing it."
  if [ -n "$resolved" ]; then
    echo "That is still $resolved."
  fi
  echo "Run $target directly, or add the directory to your shell profile:"
  echo "  export PATH=\"$dir:\$PATH\""
else
  echo
  echo "Warning: $name still resolves to $resolved, which comes earlier on your PATH."
  echo "Run $target directly, or move $dir ahead of ${resolved%/*} in PATH."
fi
