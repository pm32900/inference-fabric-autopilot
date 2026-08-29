#!/usr/bin/env bash
#
# prepare.sh — put this run's change-set on a fresh branch off main.
#
# Deliberately does not push. Everything that leaves this runner is pushed by
# the step that follows, so there is exactly one identity writing to the remote
# and one place to look when a push is rejected.
#
# Two things here are worth knowing about:
#
#   1. The author date is rewritten to now. `git cherry-pick` preserves the
#      original author date, so without this every commit would land dated the
#      day the work was written rather than the day it was reviewed and merged.
#      The author identity is not rewritten — see AUTHOR_NAME/AUTHOR_EMAIL.
#
#   2. `-x` records the commit this was replayed from. That trailer is the only
#      honest way to explain a branch whose history exists twice in the same
#      repository, and it costs nothing.
#
set -euo pipefail

SHA="${CS_SHA:?CS_SHA is required}"
BRANCH="${CS_BRANCH:?CS_BRANCH is required}"
OUT="${GITHUB_OUTPUT:-/dev/stdout}"

# The identity the landed commits are attributed to. Set these in the workflow;
# they default to the author of the commit being replayed, which is only right
# if that commit was already attributed correctly.
AUTHOR_NAME="${COMMIT_AUTHOR_NAME:-$(git log -1 --format=%an "$SHA")}"
AUTHOR_EMAIL="${COMMIT_AUTHOR_EMAIL:-$(git log -1 --format=%ae "$SHA")}"
COAUTHOR="${COMMIT_COAUTHOR:-}"

emit() { printf '%s=%s\n' "$1" "$2" >>"$OUT"; }

git config user.name "$AUTHOR_NAME"
git config user.email "$AUTHOR_EMAIL"

git fetch --quiet origin main
git checkout --quiet -B "$BRANCH" origin/main
echo "branched ${BRANCH} from origin/main at $(git rev-parse --short HEAD)"

conflict=false
if git cherry-pick -x "$SHA"; then
  echo "cherry-pick applied cleanly"
else
  echo "cherry-pick left conflicts in:"
  git diff --name-only --diff-filter=U | sed 's/^/  /'
  conflict=true
fi

emit conflict "$conflict"
emit base "$(git rev-parse origin/main)"

if [[ "$conflict" == true ]]; then
  # Leave the working tree conflicted on purpose. The next step resolves it
  # with the full diff in front of it, which is a better position than any
  # resolution this script could guess at.
  exit 0
fi

now="$(date -u +'%Y-%m-%dT%H:%M:%SZ')"
message="$(git log -1 --format=%B)"
if [[ -n "$COAUTHOR" ]]; then
  # Only append the trailer if it is not already there — the source commit may
  # carry one, and duplicates render badly on GitHub.
  if ! grep -qiF "$COAUTHOR" <<<"$message"; then
    message="${message}"$'\n'"Co-Authored-By: ${COAUTHOR}"
  fi
fi

GIT_COMMITTER_DATE="$now" git commit --quiet --amend \
  --date="$now" \
  --author="${AUTHOR_NAME} <${AUTHOR_EMAIL}>" \
  --message "$message"

echo "prepared $(git rev-parse --short HEAD) authored ${now} by ${AUTHOR_NAME} <${AUTHOR_EMAIL}>"
emit head "$(git rev-parse HEAD)"
