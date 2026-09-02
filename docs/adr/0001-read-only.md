# 0001 — Read-only, with no remediation path

**Status:** accepted

## Context

The obvious next feature for anything that diagnoses a workload is to fix it:
patch `max_num_seqs`, scale the Deployment, restart a crash-looping pod. Several
inference platforms do exactly that.

## Decision

IFA holds `get`, `list` and `watch` and nothing else. There is no write path, no
feature flag that adds one, and every HTTP handler is a GET enforced in one
place.

## Why

The cost of being wrong is asymmetric. A false positive from a read-only tool
wastes somebody's afternoon; a false positive from a tool that scales a
deployment costs GPU money, or takes capacity away during an incident. This is a
rule engine reasoning about noisy percentile estimates from other people's
metrics — a system that will sometimes be wrong, and should be built as one.

The narrower reason is adoption. "It cannot change anything" is a claim a
platform team can verify in about a minute by reading the ClusterRole, and it is
the difference between a tool that gets installed in a production namespace and
one that does not.

## Consequences

- The permission set is auditable in one file, and the claim in the README is
  checkable rather than something you have to trust.
- Every finding has to state its suggested action well enough for a human to act
  on it, which is a higher bar for the text than "threshold exceeded".
- Anyone who wants automation builds it on top of the API. The
  `/api/v1/recommendations` payload carries stable rule codes and structured
  evidence specifically so that is possible.
