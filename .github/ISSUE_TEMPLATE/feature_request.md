---
name: Feature request
about: Propose a new rule, runtime, or capability
labels: enhancement
---

## The problem

<!-- What went wrong in your cluster that IFA did not tell you about, or told
     you about unhelpfully? Concrete beats abstract. -->

## What you would want it to do

## For a new rule

- What signals would it combine? <!-- A single-signal threshold usually belongs
  in Prometheus alerting rather than here. -->
- What would the suggested action be?
- What distinguishes it from an existing rule in docs/RECOMMENDATIONS.md?

## For a new runtime

- Does it expose Prometheus metrics? Link to their definitions if you can.
- Would you be able to run `ifa check` against it once implemented?
