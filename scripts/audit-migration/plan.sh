#!/usr/bin/env bash
#
# plan.sh — decide what, if anything, this run should do.
#
# Progress is derived from the state of pull requests rather than from a file
# committed to the repository. A state file would need updating from inside the
# same workflow that reads it, which turns every run into a write to main and
# makes a half-finished run leave the queue in a lie. Pull requests already are
# the record of what landed, so they are the source of truth.
#
# Writes GitHub Actions outputs and exits 0 in every non-error case; the caller
# branches on `status`:
#
#   proceed  — outputs id/sha/slug/title/branch describe the change-set to land
#   waiting  — a pull request from an earlier run is still open
#   blocked  — a pull request was closed without merging; a human decided
#              something, so stop rather than reopen it tomorrow
#   done     — every change-set has merged
#
set -euo pipefail

QUEUE="${QUEUE_FILE:-.github/audit-migration/queue.json}"
REPO="${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
OUT="${GITHUB_OUTPUT:-/dev/stdout}"

# Newlines are flattened rather than delimiter-encoded: every value here is a
# single line by construction, and a stray one would otherwise corrupt every
# output after it in a way that only shows up as a confusing later failure.
emit() { printf '%s=%s\n' "$1" "$(printf '%s' "$2" | tr '\n' ' ')" >>"$OUT"; }

note() { printf '%s\n' "$*" >&2; }

if [[ ! -f "$QUEUE" ]]; then
  note "queue file not found: $QUEUE"
  exit 1
fi

branch_for() { printf 'audit-migration/%s-%s' "$1" "$2"; }

# One API call, reused for every lookup below. `gh pr list --state all` returns
# the head branch and merge state we need to classify each change-set.
prs="$(gh pr list --repo "$REPO" --state all --limit 200 \
        --json number,headRefName,state,url 2>/dev/null || echo '[]')"

pr_state() { # $1 = branch name -> OPEN | MERGED | CLOSED | NONE
  jq -r --arg b "$1" \
    'map(select(.headRefName == $b)) | if length == 0 then "NONE" else (sort_by(.number) | last | .state) end' \
    <<<"$prs"
}

pr_url() {
  jq -r --arg b "$1" \
    'map(select(.headRefName == $b)) | if length == 0 then "" else (sort_by(.number) | last | .url) end' \
    <<<"$prs"
}

total="$(jq '.change_sets | length' "$QUEUE")"
note "queue has ${total} change-set(s)"

for row in $(jq -r '.change_sets[] | @base64' "$QUEUE"); do
  entry() { base64 -d <<<"$row" | jq -r ".$1"; }

  id="$(entry id)"
  slug="$(entry slug)"
  branch="$(branch_for "$id" "$slug")"
  state="$(pr_state "$branch")"

  case "$state" in
    MERGED)
      note "[${id}] merged — skipping"
      continue
      ;;
    OPEN)
      note "[${id}] a pull request is still open: $(pr_url "$branch")"
      emit status waiting
      emit id "$id"
      emit url "$(pr_url "$branch")"
      exit 0
      ;;
    CLOSED)
      note "[${id}] a pull request was closed without merging: $(pr_url "$branch")"
      note "someone made a decision about this change-set; not reopening it automatically"
      emit status blocked
      emit id "$id"
      emit url "$(pr_url "$branch")"
      exit 0
      ;;
    NONE)
      note "[${id}] not started — this run will land it"
      emit status proceed
      emit id "$id"
      emit sha "$(entry sha)"
      emit slug "$slug"
      emit branch "$branch"
      emit title "$(entry title)"
      emit problem "$(entry problem)"
      emit validate "$(base64 -d <<<"$row" | jq -r '.validate | join(" ")')"
      emit atomic "$(base64 -d <<<"$row" | jq -r '.atomic // false')"
      exit 0
      ;;
  esac
done

note "every change-set has merged; the migration is complete"
emit status "done"
emit total "$total"
