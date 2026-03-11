#!/usr/bin/env bash
#
# Clones the astronomer/agents repo, generates skills/index.json,
# and packs all skills into a tarball for go:embed.
#
# Usage:
#   ./script/embed-skills.sh
#
# Set AGENTS_REPO_PATH to use a local checkout instead of cloning:
#   AGENTS_REPO_PATH=../agents ./script/embed-skills.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
EMBED_DIR="$ROOT_DIR/cmd/agent/embedded"
AGENTS_REPO="astronomer/agents"
AGENTS_REF="${AGENTS_REF:-main}"

mkdir -p "$EMBED_DIR"

# Get the agents repo
if [ -n "${AGENTS_REPO_PATH:-}" ]; then
  echo "Using local agents repo: $AGENTS_REPO_PATH"
  AGENTS_DIR="$AGENTS_REPO_PATH"
else
  AGENTS_DIR=$(mktemp -d)
  trap 'rm -rf "$AGENTS_DIR"' EXIT
  echo "Cloning $AGENTS_REPO@$AGENTS_REF..."
  gh repo clone "$AGENTS_REPO" "$AGENTS_DIR" -- --depth 1 --branch "$AGENTS_REF" --quiet 2>&1
fi

SKILLS_DIR="$AGENTS_DIR/skills"

if [ ! -d "$SKILLS_DIR" ]; then
  echo "Error: $SKILLS_DIR does not exist"
  exit 1
fi

# Generate index.json
echo "Generating skills index..."
python3 -c "
import os, json, re

skills_dir = '$SKILLS_DIR'
skills = []

for name in sorted(os.listdir(skills_dir)):
    skill_dir = os.path.join(skills_dir, name)
    skill_md = os.path.join(skill_dir, 'SKILL.md')

    if not os.path.isdir(skill_dir) or not os.path.isfile(skill_md):
        continue

    description = name + ' skill'
    with open(skill_md) as f:
        content = f.read()
    fm_match = re.match(r'^---\n(.*?)\n---', content, re.DOTALL)
    if fm_match:
        for line in fm_match.group(1).splitlines():
            if line.startswith('description:'):
                desc = line[len('description:'):].strip()
                desc = desc.strip('\"').strip(\"'\")
                if desc:
                    description = desc
                break

    files = []
    for root, dirs, filenames in os.walk(skill_dir):
        dirs[:] = [d for d in dirs if not d.startswith('.')]
        for fname in sorted(filenames):
            if fname.startswith('.'):
                continue
            rel = os.path.relpath(os.path.join(root, fname), skill_dir)
            files.append(rel)

    skills.append({'name': name, 'description': description, 'files': files})

with open(os.path.join(skills_dir, 'index.json'), 'w') as f:
    json.dump({'skills': skills}, f, indent=2)

print(f'Indexed {len(skills)} skills')
"

# Create tarball of the skills directory
echo "Packing skills..."
tar -czf "$EMBED_DIR/skills.tar.gz" -C "$SKILLS_DIR" .

SKILL_COUNT=$(python3 -c "import json; print(len(json.load(open('$SKILLS_DIR/index.json'))['skills']))")
TARBALL_SIZE=$(wc -c < "$EMBED_DIR/skills.tar.gz" | tr -d ' ')
echo ""
echo "Embedded $SKILL_COUNT skills"
echo "  Tarball: $(( TARBALL_SIZE / 1024 ))KB"
echo "  Output:  $EMBED_DIR/skills.tar.gz"
