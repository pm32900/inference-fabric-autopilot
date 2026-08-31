---
name: Runtime validation report
about: You ran `ifa check` against a real server — the most useful contribution right now
labels: validation
---

Every adapter here is tested against fixtures built from each runtime's own
metric definitions, not against live servers. This issue closes that gap.

## Runtime

- Runtime and version: <!-- e.g. vLLM 0.9.1, engine V1 -->
- Deployment shape: <!-- single model? several? tensor parallelism? -->

## `ifa check` output

```
ifa check http://your-target:8000/metrics -runtime vllm -model your-model
```

<!-- paste the whole thing, including any MISSING lines -->

## Anything that looked wrong

<!-- Values that disagree with what you see in Prometheus, metrics reported as
     missing that you know are present, or findings that do not match reality. -->
