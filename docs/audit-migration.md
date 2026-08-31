# Audit migration

A large audit of this repository produced six change-sets that together rewrite
the telemetry model, the runtime adapters, the recommendation engine, the Helm
chart, CI and the documentation. They were developed on one branch and are being
landed on `main` one at a time, on a schedule, each behind the same review gate a
hand-written pull request goes through.

This document exists so that someone finding an `audit-migration/*` pull request
in six months knows what it was.

## Why staged rather than one merge

The six change-sets are a dependency chain, not six independent improvements.
Change-set 03 cannot compile without 02, and 04 through 06 all assume 03. Landing
them one at a time means each one gets reviewed on its own terms and `git bisect`
stays meaningful afterwards, which a single 15,000-line merge would destroy.

It also means that if one of them turns out to be wrong, the blast radius is that
change-set rather than the whole audit.

## How it works

`audit/staging` holds the six commits as they were developed. It is never
modified and never merged.

`.github/audit-migration/queue.json` lists them in dependency order, with the
problem each one addresses and the validations that apply to it.

`.github/workflows/audit-migration.yml` runs daily. Each run:

1. Reads the queue and works out the next change-set by looking at the state of
   existing pull requests. There is no progress file to get out of sync.
2. Branches from the current `main` and cherry-picks that change-set, setting
   the author and author date per the Attribution section below and leaving the
   message otherwise as written, plus the `(cherry picked from commit …)` line
   git adds.
3. Runs the validations for that change-set — always vet, build and the race
   detector; additionally Helm rendering, shellcheck, the benchmarks or the demo
   where the change-set touches them.
4. Fixes anything that fails, on that branch, as a separate commit. It does not
   delete failing tests or loosen assertions, and after three failed attempts it
   opens the pull request as a draft and stops.
5. Opens a pull request describing the problem, the root cause, the
   implementation and the trade-offs, and enables auto-merge with rebase.

`main` requires the `verify` status check, so nothing merges without it passing.
The workflow never pushes to `main` and never merges with `--admin`.

## Stopping it

Disable **Audit migration** in the Actions tab. Nothing else depends on it.

A run that finds an open pull request does nothing. A run that finds a pull
request that was *closed without merging* stops permanently and reports why —
closing one is how you tell it a human has taken over.

## When it finishes

The last run opens an issue listing what landed and recommending the next
roadmap item. It does not start that work. At that point:

- delete `.github/workflows/audit-migration.yml` and `scripts/audit-migration/`
- switch branch protection from `verify` to the checks in `ci.yml`, which
  change-set 05 introduces and which cover the chart, the container image, the
  shell scripts and the demo
- delete `.github/workflows/verify.yml`
- delete the `audit/staging` branch and the `GH_PAT` secret

## Attribution

The change-sets on `audit/staging` were drafted by an AI assistant working from a
detailed audit brief written by the repository owner, who reviews every pull
request and merges it. Commits land authored by the owner, with the author date
set to the day they were reviewed and merged, and no `Co-Authored-By` trailer.
This is set in the workflow's `env:` block.

The provenance is not hidden by that choice, and is not intended to be. Every
replayed commit keeps the `(cherry picked from commit …)` line git writes, and
the commit it names is on `audit/staging` in this repository with its original
authorship intact. `git log audit/staging` shows the drafts as they were made.

Attribution on `main` is not uniform, and rewriting merged history to make it so
would be worse than the inconsistency:

- the two bootstrap commits and change-set 02 carry a `Co-Authored-By: Claude`
  trailer;
- change-set 03 landed authored by `Claude <noreply@anthropic.com>`, keeping its
  original author date;
- change-sets 04 onward follow the policy above.
