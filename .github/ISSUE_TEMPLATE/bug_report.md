---
name: Bug report
about: Something behaves differently from what the docs describe
labels: bug
---

## What happened

## What you expected

## Version and environment

- IFA version: <!-- `ifa version`, or /api/v1/healthz -->
- Runtime and version: <!-- e.g. vLLM 0.9.1, Triton 24.08 -->
- Kubernetes version, if relevant:
- Installed via: <!-- Helm / helm template / binary / make demo -->

## Reproduction

<!-- If a target is involved, this is usually the single most useful thing: -->
```
ifa check http://your-target:8000/metrics -runtime vllm -model your-model
```

## Relevant output

<!-- Logs, or the finding you disagree with. Findings carry their evidence:
     `ifa recommendations -code IFA-XXX-NNN` includes the numbers and thresholds. -->
