#!/usr/bin/env bash
#
# prepare.sh — put this run's change-set on a fresh branch off main.
#
# Deliberately does not push. Everything that leaves this runner is pushed by
# the step that follows, so there is exactly one identity writing to the remote
# and one place to look when a push is rejected.
#
# Attribution: the repository owner is recorded as the author of every commit
# this lands, and the author date is set to the run so the history reads in the
# order the changes were reviewed and merged rather than the order they were
# drafted. The message is not otherwise altered.
#
# `-x` records the commit this was replayed from. That trailer is the only
# honest way to explain a branch whose history exists twice in the same
# repository, and it costs nothing.
#
set -euo pipefail

SHA="${CS_SHA:?CS_SHA is required}"
BRANCH="${CS_BRANCH:?CS_BRANCH is required}"
OUT="${GITHUB_OUTPUT:-/dev/stdout}"

# Identity every commit on the branch lands under: the replayed change-set and
# any fix-up this run makes. Set in the workflow; required, because silently
# falling back to the replayed commit's author would make the attribution
# depend on which change-set happened to be next.
AUTHOR_NAME="${COMMIT_AUTHOR_NAME:?COMMIT_AUTHOR_NAME is required}"
AUTHOR_EMAIL="${COMMIT_AUTHOR_EMAIL:?COMMIT_AUTHOR_EMAIL is required}"

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

GIT_COMMITTER_DATE="$now" git commit --quiet --amend --no-edit \
  --date="$now" \
  --author="${AUTHOR_NAME} <${AUTHOR_EMAIL}>"

echo "prepared $(git rev-parse --short HEAD)"
echo "  author: $(git log -1 --format='%an <%ae>') on $(git log -1 --format=%aI)"
emit head "$(git rev-parse HEAD)"
