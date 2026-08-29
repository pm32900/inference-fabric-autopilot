# 0004 — Deterministic rules, not a learned model

**Status:** accepted

## Context

Anomaly detection over inference telemetry is an obvious pitch, and this is a
project about AI infrastructure, so a model would fit the theme.

## Decision

Every rule is an explicit threshold or comparison written in Go, with the
thresholds in configuration.

## Why

The output has to be actionable by someone at 3am, which means it has to be
arguable. "The queue has been above 8 while GPU utilisation stayed above 85% for
the last minute, so this is a capacity shortage" can be checked and disagreed
with. "The anomaly score is 0.87" cannot.

The signals here are also not subtle. Preemption is either happening or it is
not. A queue is either draining or building. The hard part of inference
diagnostics is knowing *which pair of signals to look at together*, and that is
domain knowledge, not something to be learned from one cluster's telemetry.

There is also nothing to train on. A model would need labelled incidents from
fleets this project does not have, and shipping one trained on synthetic data
would be worse than shipping nothing.

## Consequences

- Rules are testable at their boundaries, and they are.
- Findings carry their evidence: the observed value, the threshold, and the
  comparison.
- Thresholds must be tuned per environment; the defaults assume interactive
  serving and say so.
- Genuinely novel failure modes are missed. That is the trade: this catches the
  known ones and explains them, rather than flagging the unknown ones and not.
