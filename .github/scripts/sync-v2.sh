#!/usr/bin/env bash
# Keeps a PR open that merges main into v2. Run by .github/workflows/sync-v2.yaml;
# see that file for why the PR's head is a branch of its own and never main.
#
# Needs: a clone whose `origin` can be pushed to, git's user.email set to the
# bot's (commits by anyone else on the sync branch count as a person's work),
# `gh` authenticated as a token that can push SYNC_BRANCH and write PRs, and
# REPO (owner/name). Optional: SYNC_BRANCH, BASE, SOURCE override branch names.
#
# Cases, in order:
#   - v2 has main: close an open sync PR (deleting its branch when it holds only
#     the bot's commits), or just comment on it if a person pushed to it. Done.
#   - the last sync PR was closed without merging and main hasn't moved since:
#     respect the close and do nothing. If main has moved, start over: delete
#     the old branch and open a new PR. A closed PR's branch is never built on
#     (its commits stay reachable from the closed PR on GitHub).
#   - an open PR, or a branch with a person's commits and no PR yet: merge main
#     into the branch, keeping what is there.
#   - otherwise: build the branch from v2 and merge main in. A branch v2 already
#     merged is an ancestor of v2, so that push is still a fast-forward.
# Every push to the branch is a plain one; the only rewrite is the delete above.
#
# When the merge can't be pushed as is, it is left for a person:
#   - it conflicts, or
#   - it changes .github/workflows (see merge_source).
# A new branch then gets one empty commit on top of v2, because GitHub opens no PR
# whose head has nothing v2 lacks; the PR body says what to merge by hand. An
# existing branch is not touched, and the PR gets one comment per main SHA.
set -euo pipefail

: "${REPO:?REPO must be owner/name}"
SYNC_BRANCH=${SYNC_BRANCH:-sync/main-into-v2}
BASE=${BASE:-v2}
SOURCE=${SOURCE:-main}
OWNER=${REPO%%/*}
TMP=${RUNNER_TEMP:-$(mktemp -d)}
BOT_EMAIL=$(git config user.email)

git fetch --quiet --no-tags origin \
  "+refs/heads/${BASE}:refs/remotes/origin/${BASE}" \
  "+refs/heads/${SOURCE}:refs/remotes/origin/${SOURCE}"
# The sync branch may not exist. ls-remote tells "missing" from a failed fetch.
if [ -n "$(git ls-remote --heads origin "refs/heads/${SYNC_BRANCH}")" ]; then
  git fetch --quiet --no-tags origin "+refs/heads/${SYNC_BRANCH}:refs/remotes/origin/${SYNC_BRANCH}"
  branch_exists=true
else
  branch_exists=false
fi

base_ref=origin/${BASE}
source_ref=origin/${SOURCE}
sync_ref=origin/${SYNC_BRANCH}
source_sha=$(git rev-parse "$source_ref")
main_marker="<!-- sync-v2 main: ${source_sha} -->"
blocked_marker="<!-- sync-v2 blocked: ${source_sha} -->"

# The latest sync PR, open or not. Match on the head branch AND this repository:
# --head alone also matches a fork's branch of the same name. gh lists newest first.
prs=$(gh pr list --repo "$REPO" --base "$BASE" --head "$SYNC_BRANCH" --state all --limit 20 \
  --json number,state,isCrossRepository,headRepositoryOwner,body \
  --jq "map(select(.isCrossRepository == false and .headRepositoryOwner.login == \"${OWNER}\"))")
pr=$(jq -r 'map(select(.state == "OPEN")) | .[0].number // empty' <<<"$prs")
pr_body=$(jq -r 'map(select(.state == "OPEN")) | .[0].body // empty' <<<"$prs")
last_state=$(jq -r '.[0].state // empty' <<<"$prs")
last_number=$(jq -r '.[0].number // empty' <<<"$prs")
last_body=$(jq -r '.[0].body // empty' <<<"$prs")

# only_bot_commits: true when every commit on the branch that v2 lacks is the bot's.
only_bot_commits() {
  [ -z "$(git log --format=%ae "${base_ref}..${sync_ref}" | grep -vxF "$BOT_EMAIL" || true)" ]
}

# comment_once MARKER TEXT: comments on the open PR unless a comment, or the
# body as it was before this run, already carries MARKER. A failed fetch of the
# comments fails the run rather than reading as "no marker".
comment_once() {
  local comments
  comments=$(gh api "repos/${REPO}/issues/${pr}/comments" --paginate --jq '.[].body')
  if grep -qF "$1" <<<"${comments}${pr_body}"; then
    return 0
  fi
  printf '%s\n%s\n' "$1" "$2" >"$TMP/comment.md"
  gh pr comment "$pr" --repo "$REPO" --body-file "$TMP/comment.md" >/dev/null
  echo "Commented on #${pr}."
}

if git merge-base --is-ancestor "$source_ref" "$base_ref"; then
  echo "${BASE} already has every commit on ${SOURCE}."
  if [ -n "$pr" ]; then
    if ! $branch_exists || only_bot_commits; then
      gh pr close "$pr" --repo "$REPO" --comment "\`${BASE}\` has \`${SOURCE}\` (${source_sha}); closing." >/dev/null
      echo "Closed #${pr}."
      if $branch_exists; then
        git push --quiet origin --delete "$SYNC_BRANCH"
        echo "Deleted ${SYNC_BRANCH}."
      fi
    else
      comment_once "<!-- sync-v2 has-main: ${source_sha} -->" \
        "\`${BASE}\` already has \`${SOURCE}\` (${source_sha}), but this branch has commits from someone other than the bot, so it stays open. Close it if they aren't needed."
    fi
  fi
  exit 0
fi

superseded=""
if [ -z "$pr" ] && [ "$last_state" = CLOSED ]; then
  if grep -qF "$main_marker" <<<"$last_body"; then
    echo "#${last_number} was closed without merging and ${SOURCE} hasn't moved since; leaving it closed."
    exit 0
  fi
  superseded=$last_number
fi

# Delete the bot's branch when there's no open PR to build on and it belongs to
# a closed or merged PR, or holds only the bot's own commits (say a run that
# pushed and then failed to open the PR). A branch v2 has merged needs no delete.
if $branch_exists && [ -z "$pr" ] && ! git merge-base --is-ancestor "$sync_ref" "$base_ref" &&
  { [ -n "$last_state" ] || only_bot_commits; }; then
  git push --quiet origin --delete "$SYNC_BRANCH"
  echo "Deleted ${SYNC_BRANCH}, left from #${last_number:-no PR}, to start over."
  branch_exists=false
fi

# Why the merge was left for a person, and the files involved. Empty when it pushed.
blocked=""
blocked_files=""

# merge_source tries `git merge SOURCE` onto HEAD. It returns 0 with the merge
# committed and safe to push, or 1 with HEAD unchanged and $blocked set. A merge
# that fails for any reason other than conflicts fails the run.
merge_source() {
  if ! git merge --no-ff --no-edit -m "Merge ${SOURCE} into ${BASE}" "$source_ref" >"$TMP/merge.log" 2>&1; then
    blocked_files=$(git diff --name-only --diff-filter=U)
    if git rev-parse --verify --quiet MERGE_HEAD >/dev/null; then
      git merge --abort
    fi
    if [ -z "$blocked_files" ]; then
      echo "::error::git merge ${source_ref} failed without conflicts:" >&2
      cat "$TMP/merge.log" >&2
      exit 1
    fi
    blocked="conflicts"
    return 1
  fi
  # GitHub documents that it refuses a push from an App token without the
  # `workflows` permission when the push changes .github/workflows. Unverified
  # here; checking first gives a friendlier result than the refused push, which
  # push_branch still catches.
  if ! git diff --quiet HEAD^1 HEAD -- .github/workflows; then
    blocked="workflows"
    blocked_files=$(git diff --name-only HEAD^1 HEAD -- .github/workflows)
    git reset --quiet --hard HEAD^1
    return 1
  fi
  return 0
}

push_branch() {
  if ! git push --quiet origin "HEAD:refs/heads/${SYNC_BRANCH}" 2>"$TMP/push.log"; then
    cat "$TMP/push.log" >&2
    echo "::error::GitHub refused the push to ${SYNC_BRANCH}. If it mentions workflows, the App token lacks the \`workflows\` permission: merge ${SOURCE} into ${SYNC_BRANCH} by hand, or grant workflows: write to astro-cli-sync in astronomer/.github#105." >&2
    exit 1
  fi
  echo "Pushed ${SYNC_BRANCH} at $(git rev-parse HEAD)."
}

if $branch_exists && git merge-base --is-ancestor "$source_ref" "$sync_ref"; then
  echo "${SYNC_BRANCH} already has ${SOURCE} at ${source_sha}."
elif $branch_exists && ! git merge-base --is-ancestor "$sync_ref" "$base_ref"; then
  # The open PR's branch, or a person's work with no PR yet: build on it.
  git checkout --quiet -B "$SYNC_BRANCH" "$sync_ref"
  if merge_source; then
    push_branch
  fi
else
  git checkout --quiet -B "$SYNC_BRANCH" "$base_ref"
  if ! merge_source; then
    git commit --quiet --allow-empty -m "Start merging ${SOURCE} into ${BASE}" \
      -m "The merge needs a person (${blocked}); see the PR."
  fi
  push_branch
fi

# The PR body is rewritten on every run, so it always describes main as it is now.
count=$(git rev-list --count --no-merges "${base_ref}..${source_ref}")
listed=$(git log -n 100 --no-merges --format='- %h %s' "${base_ref}..${source_ref}")
if [ "$count" -gt 100 ]; then
  listed+=$'\n'"- …and $((count - 100)) more: \`git log --no-merges origin/${BASE}..origin/${SOURCE}\`"
fi

{
  echo "$main_marker"
  if [ -n "$blocked" ]; then
    echo "$blocked_marker"
  fi
  cat <<EOF
Merges \`${SOURCE}\` into \`${BASE}\`. Opened and kept current by \`sync-v2.yaml\`, which runs on every push to \`${SOURCE}\`, daily, and on demand.
EOF
  if [ -n "$superseded" ]; then
    echo
    echo "Supersedes #${superseded}, which was closed without merging before \`${SOURCE}\` moved."
  fi
  cat <<EOF

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
- **Review it as a port, not a rubber stamp.** \`${BASE}\`'s layout differs from \`${SOURCE}\`'s and its lint is stricter, so \`${SOURCE}\`'s code can need changes to land here. Push them to \`${SYNC_BRANCH}\`; the bot merges new \`${SOURCE}\` commits on top and never rewrites the branch while this PR is open.
- **Merge with "Create a merge commit". Never squash or rebase** (see #2270): either one drops \`${SOURCE}\` as a parent, and the next sync brings the same commits again.
- Nothing merges this automatically. If you close it unmerged, the bot leaves it closed until \`${SOURCE}\` moves, then opens a new one.

<sub>The bot pushes with a GitHub App token, so \`${BASE}\`'s CI runs on this PR's own merge commit.</sub>
EOF
} >"$TMP/body.md"

if [ -z "$pr" ]; then
  url=$(gh pr create --repo "$REPO" --base "$BASE" --head "$SYNC_BRANCH" \
    --title "Merge ${SOURCE} into ${BASE}" --body-file "$TMP/body.md")
  echo "Opened ${url}."
  exit 0
fi

# An existing branch the bot couldn't move: say so once per tip of main. Checked
# before the body is rewritten, since the old body may already carry the marker.
if [ -n "$blocked" ]; then
  comment_once "$blocked_marker" \
    "\`${SOURCE}\` moved to ${source_sha} and merging it into \`${SYNC_BRANCH}\` needs a person (${blocked}): the bot left the branch as it was. The PR description lists the files and the commands."
fi

gh pr edit "$pr" --repo "$REPO" --body-file "$TMP/body.md" >/dev/null
echo "Updated #${pr}."
