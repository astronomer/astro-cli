#!/usr/bin/env bash
# Keeps a PR open that merges main into v2. Run by .github/workflows/sync-v2.yaml;
# see that file for why the PR's head is a branch of its own and never main.
#
# Needs: a clone whose `origin` can be pushed to, `gh` authenticated as a token
# that can push SYNC_BRANCH and write PRs, and REPO (owner/name). Optional:
# SYNC_BRANCH, BASE, SOURCE override the branch names.
#
# The branch only ever moves forward, so every push is a plain one:
#   - no branch yet, or one v2 already merged: start it from v2 and merge main in;
#   - a branch with commits v2 doesn't have (the open PR, maybe with fixes someone
#     pushed): merge main into it, keeping what is there.
# When the merge can't be pushed as is, the branch is left for a person:
#   - it conflicts, or
#   - it changes .github/workflows: GitHub refuses a push that does that from an
#     App token without the `workflows` permission, which this token doesn't have.
# A new branch then gets one empty commit on top of v2, because GitHub opens no PR
# whose head has nothing v2 lacks; the PR body says what to merge by hand. An
# existing branch is not touched, and the PR gets a comment instead.
set -euo pipefail

: "${REPO:?REPO must be owner/name}"
SYNC_BRANCH=${SYNC_BRANCH:-sync/main-into-v2}
BASE=${BASE:-v2}
SOURCE=${SOURCE:-main}
OWNER=${REPO%%/*}
TMP=${RUNNER_TEMP:-$(mktemp -d)}

git fetch --quiet --no-tags origin \
  "+refs/heads/${BASE}:refs/remotes/origin/${BASE}" \
  "+refs/heads/${SOURCE}:refs/remotes/origin/${SOURCE}"
# The sync branch may not exist; a missing ref is not an error here.
git fetch --quiet --no-tags origin "+refs/heads/${SYNC_BRANCH}:refs/remotes/origin/${SYNC_BRANCH}" 2>/dev/null || true

base_ref=origin/${BASE}
source_ref=origin/${SOURCE}
sync_ref=origin/${SYNC_BRANCH}
source_sha=$(git rev-parse "$source_ref")

behind=$(git rev-list --count "${base_ref}..${source_ref}")
if [ "$behind" = 0 ]; then
  echo "${BASE} already has every commit on ${SOURCE}."
  exit 0
fi

# Match on the head branch AND this repository: --head alone also matches a
# fork's branch of the same name.
pr=$(gh pr list --repo "$REPO" --base "$BASE" --head "$SYNC_BRANCH" --state open \
  --json number,isCrossRepository,headRepositoryOwner \
  --jq "map(select(.isCrossRepository == false and .headRepositoryOwner.login == \"${OWNER}\")) | .[0].number // empty")

# Why the merge was left for a person, and the files involved. Empty when it pushed.
blocked=""
blocked_files=""

# merge_source tries `git merge SOURCE` onto HEAD. It returns 0 with the merge
# committed and safe to push, or 1 with HEAD unchanged and $blocked set.
merge_source() {
  if ! git merge --no-ff --no-edit -m "Merge ${SOURCE} into ${BASE}" "$source_ref" >"$TMP/merge.log" 2>&1; then
    blocked="conflicts"
    blocked_files=$(git diff --name-only --diff-filter=U)
    git merge --abort
    return 1
  fi
  if ! git diff --quiet HEAD^1 HEAD -- .github/workflows; then
    blocked="workflows"
    blocked_files=$(git diff --name-only HEAD^1 HEAD -- .github/workflows)
    git reset --quiet --hard HEAD^1
    return 1
  fi
  return 0
}

branch_exists=false
if git rev-parse --verify --quiet "$sync_ref" >/dev/null; then
  branch_exists=true
fi

push=false
if $branch_exists && git merge-base --is-ancestor "$source_ref" "$sync_ref"; then
  echo "${SYNC_BRANCH} already has ${SOURCE} at ${source_sha}."
elif $branch_exists && ! git merge-base --is-ancestor "$sync_ref" "$base_ref"; then
  # The branch carries work v2 doesn't have yet: build on it, never replace it.
  git checkout --quiet -B "$SYNC_BRANCH" "$sync_ref"
  if merge_source; then
    push=true
  fi
else
  # No branch, or one v2 has merged. Starting from v2 is still a fast-forward of
  # a merged branch, since v2 contains it.
  git checkout --quiet -B "$SYNC_BRANCH" "$base_ref"
  if merge_source; then
    push=true
  else
    git commit --quiet --allow-empty -m "Start merging ${SOURCE} into ${BASE}" \
      -m "The merge needs a person (${blocked}); see the PR."
    push=true
  fi
fi

if $push; then
  git push --quiet origin "HEAD:refs/heads/${SYNC_BRANCH}"
  echo "Pushed ${SYNC_BRANCH} at $(git rev-parse HEAD)."
fi

# The PR body is rewritten on every run, so it always describes main as it is now.
commits=$(git log --no-merges --format='- %h %s' "${base_ref}..${source_ref}")
listed=$(printf '%s\n' "$commits" | head -n 100)
count=$(printf '%s\n' "$commits" | grep -c . || true)
if [ "$count" -gt 100 ]; then
  listed+=$'\n'"- …and $((count - 100)) more: \`git log --no-merges origin/${BASE}..origin/${SOURCE}\`"
fi

{
  cat <<EOF
Merges \`${SOURCE}\` into \`${BASE}\`. Opened and kept current by \`sync-v2.yaml\`, which runs on every push to \`${SOURCE}\`, daily, and on demand.

**\`${BASE}\` is missing ${count} commit(s) from \`${SOURCE}\`** (as of ${source_sha}):

${listed}

EOF
  if [ -n "$blocked" ]; then
    if [ "$blocked" = conflicts ]; then
      echo "### Needs a manual merge: it conflicts"
    else
      echo "### Needs a manual merge: it changes workflows"
      echo
      echo "The bot's token has no \`workflows\` permission, and GitHub refuses a push from it that changes \`.github/workflows\`."
    fi
    cat <<EOF

The bot did not merge \`${SOURCE}\` into \`${SYNC_BRANCH}\`; that is left to you. Files:

\`\`\`
${blocked_files}
\`\`\`

To finish it:

\`\`\`
git fetch origin
git checkout ${SYNC_BRANCH} && git pull
git merge origin/${SOURCE}
# resolve, then: git commit
git push
\`\`\`

EOF
  fi
  cat <<EOF
### Before merging
- **Review it as a port, not a rubber stamp.** \`${BASE}\`'s layout differs from \`${SOURCE}\`'s and its lint is stricter, so \`${SOURCE}\`'s code can need changes to land here. Push them to \`${SYNC_BRANCH}\`; the bot merges new \`${SOURCE}\` commits on top and never rewrites the branch.
- **Merge with "Create a merge commit". Never squash or rebase** (see #2270): either one drops \`${SOURCE}\` as a parent, and the next sync brings the same commits again.
- Nothing merges this automatically.

<sub>The bot pushes with a GitHub App token, so \`${BASE}\`'s CI runs on this PR's own merge commit.</sub>
EOF
} >"$TMP/body.md"

if [ -z "$pr" ]; then
  url=$(gh pr create --repo "$REPO" --base "$BASE" --head "$SYNC_BRANCH" \
    --title "Merge ${SOURCE} into ${BASE}" --body-file "$TMP/body.md")
  echo "Opened ${url}."
  exit 0
fi

gh pr edit "$pr" --repo "$REPO" --body-file "$TMP/body.md" >/dev/null
echo "Updated #${pr}."

# An existing branch the bot couldn't move: say so once per tip of main.
if [ -n "$blocked" ]; then
  marker="<!-- sync-v2: ${source_sha} -->"
  if ! gh api "repos/${REPO}/issues/${pr}/comments" --paginate --jq '.[].body' | grep -qF "$marker"; then
    {
      echo "$marker"
      echo "\`${SOURCE}\` moved to ${source_sha} and merging it into \`${SYNC_BRANCH}\` needs a person (${blocked}): the bot left the branch as it was. The PR description lists the files and the commands."
    } >"$TMP/comment.md"
    gh pr comment "$pr" --repo "$REPO" --body-file "$TMP/comment.md" >/dev/null
    echo "Commented on #${pr}: ${blocked}."
  fi
fi
