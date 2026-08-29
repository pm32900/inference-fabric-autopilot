#!/usr/bin/env bash
#
# prepare.sh — put this run's change-set on a fresh branch off main.
#
# Deliberately does not push. Everything that leaves this runner is pushed by
# the step that follows, so there is exactly one identity writing to the remote
# and one place to look when a push is rejected.
#
# Attribution: a replayed commit keeps the author, author date and message it
# was written with. `git cherry-pick` preserves all three, and nothing here
# amends them. The git identity configured below is used only for commits this
# run creates itself — the fix-ups made when a validation fails — and for the
# committer line, which records who replayed the commit rather than who wrote
# it.
#
# `-x` records the commit this was replayed from. That trailer is the only
# honest way to explain a branch whose history exists twice in the same
# repository, and it costs nothing.
#
set -euo pipefail

SHA="${CS_SHA:?CS_SHA is required}"
BRANCH="${CS_BRANCH:?CS_BRANCH is required}"
OUT="${GITHUB_OUTPUT:-/dev/stdout}"

# Identity for commits this run creates itself. The work in those commits is
# the assistant's, so it is attributed to the assistant; overriding this to a
# human who did not write them would be a false record.
FIXUP_NAME="${FIXUP_AUTHOR_NAME:-Claude}"
FIXUP_EMAIL="${FIXUP_AUTHOR_EMAIL:-noreply@anthropic.com}"

emit() { printf '%s=%s\n' "$1" "$2" >>"$OUT"; }

# git refuses to commit at all without an identity, so this is required even
# for a cherry-pick that applies cleanly.
git config user.name "$FIXUP_NAME"
git config user.email "$FIXUP_EMAIL"

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

# No amend. The commit is left exactly as cherry-pick produced it: original
# author, original author date, original message, plus the -x provenance line.
echo "prepared $(git rev-parse --short HEAD)"
echo "  author: $(git log -1 --format='%an <%ae>') on $(git log -1 --format=%aI)"
emit head "$(git rev-parse HEAD)"
