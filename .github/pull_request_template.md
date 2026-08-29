## What and why

<!-- The diff says what changed. Say why. -->

## How it was verified

```bash
make verify
```

<!-- Plus anything specific: `make demo` output, a new test, `ifa check` against
     a real server. -->

## Checklist

- [ ] `make verify` passes (gofmt, vet, race tests, chart lint)
- [ ] New behaviour has a test that fails without the change
- [ ] No new dependency, or the PR explains why one is needed
- [ ] Stays read-only: no Kubernetes writes, no remediation
- [ ] No invented data: a signal the runtime does not expose stays unmeasured
- [ ] Docs updated in this PR if behaviour changed — a new rule needs an entry
      in `docs/RECOMMENDATIONS.md`
