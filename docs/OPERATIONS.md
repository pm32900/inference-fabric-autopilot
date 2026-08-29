# Operations

## Health

| Endpoint | Meaning |
|---|---|
| `/api/v1/healthz` | The process is running. Echoes the loaded configuration. |
| `/api/v1/readyz` | The first collection cycle has completed. |
| `/metrics` | IFA's own operational metrics. |

Readiness deliberately does not depend on scrapes succeeding: an instance that
cannot reach its targets is exactly the one you need to be able to query.

## What to alert on

```promql
# A target stopped being scraped entirely. This is the important one:
# a target that is never attempted increments no error counter, so alerting
# on ifa_scrape_errors_total alone misses it.
time() - ifa_last_successful_scrape_timestamp_seconds > 300

# Scrapes are failing.
rate(ifa_scrape_errors_total[5m]) > 0

# An integration is only half working: some rules cannot run for this workload.
ifa_target_missing_metrics > 0

# History is incomplete (diagnostics are unaffected).
rate(ifa_database_dropped_total[5m]) > 0
```

Do not page on `ifa_active_recommendations`. It is a count of findings across the
fleet and moves for benign reasons; the findings themselves carry severity.

## Nothing is being collected

`/api/v1/telemetry` returns `count: 0` after a scrape interval.

1. **Are there any targets?** `curl -s localhost:8080/api/v1/healthz | jq
   .config.target_count`. Zero means the ConfigMap has no `collector.targets`,
   or the pod is running an older ConfigMap — the startup log prints the loaded
   configuration.
2. **Are scrapes failing?** `curl -s localhost:8080/metrics | grep
   ifa_scrape_errors_total`. Non-zero means the endpoint is unreachable: check
   the Service name and port, and whether a NetworkPolicy is blocking egress.
   The logs name the URL and the error.
3. **Are scrapes succeeding but returning nothing useful?**
   `ifa_target_missing_metrics` will be non-zero, and `IFA-OBS-001` will be
   firing. Go to the next section.

## Scrapes succeed but every value is null

Almost always one of three things, in order of likelihood.

**The `model_name` filter does not match.** vLLM labels every series with
`model_name`; if the configured value differs by so much as a revision suffix,
nothing matches and every metric reads as absent. Run:

```bash
ifa check http://the-target:8000/metrics -runtime vllm -model whatever-you-configured
```

It lists the models actually present in the payload.

**Metrics are disabled.** vLLM started with `--disable-log-stats` responds `200`
with nothing useful.

**The wrong runtime is configured.** A Triton endpoint read by the vLLM adapter
parses cleanly and matches nothing.

## Findings look wrong

Every finding carries its evidence — the observed value, the threshold, the
comparison, and the runtime metric the value came from. Check that first:

```bash
ifa recommendations -code IFA-CAP-001
curl -s localhost:8080/api/v1/recommendations | jq '.items[] | select(.code=="IFA-CAP-001") | .evidence'
```

Then query the source metric directly and compare. If the numbers agree and the
conclusion still looks wrong, the threshold is wrong for your workload — see the
tuning section of [RECOMMENDATIONS.md](RECOMMENDATIONS.md). If the numbers
disagree, that is a bug worth an issue.

## No findings at all, and that seems too good

Check for `IFA-OBS-001` and `IFA-OBS-002` first. Partial telemetry and stale
telemetry both produce quiet output, and those two rules exist so that quiet
does not read as healthy.

Then check that enough history has accumulated: sustained rules need
`sustain_for` (default 45s) of continuous samples and will not fire for the
first minute after a restart.

## Changing thresholds

Edit the values file and `helm upgrade`. The Deployment carries a checksum
annotation over the ConfigMap, so the pods restart. Without that annotation the
ConfigMap would update and the running process would keep the old values, which
is a genuinely confusing failure.

Confirm afterwards:

```bash
curl -s localhost:8080/api/v1/rules | jq .thresholds
```

## Restarts and rollouts

The Deployment uses `Recreate`. A restart clears the in-memory window, so:

- Sustained rules stay quiet for `sustain_for`.
- Rate metrics report unmeasured until the second scrape.
- History in the database is unaffected.

This is also what a counter reset in a *watched* workload looks like from IFA's
side: after a vLLM pod restarts, its rate metrics report unmeasured for one
interval rather than reporting zero, so the low-throughput rule does not fire at
exactly the moment you are already dealing with a restart.

## Memory

Bounded by `retention_samples × workloads`, roughly 600 bytes per snapshot. Two
hundred workloads at the default 120 samples is on the order of 15 MB. If
`ifa_store_snapshots` is far above `targets × retention_samples`, workloads are
being created faster than they are pruned — usually a target whose `name`
changes between scrapes.

## Logs

Structured, JSON by default (`IFA_LOG_FORMAT=text` for local work). API requests
log at debug; slow requests and 5xx log at info and error, so raising the level
to debug on a busy instance is noisy.

Notable messages:

| Message | Meaning |
|---|---|
| `scrape failed` | Target unreachable or returned non-200. Includes the URL and error. |
| `exposition lines could not be parsed` | The runtime's metrics format may have changed. |
| `DCGM scrape failed` | GPU metrics unavailable for that scrape; inference telemetry is unaffected. |
| `telemetry dropped: database write queue is full` | History incomplete; diagnostics unaffected. |
| `kubernetes discovery unavailable` | Scaling rules will not run. Not fatal. |
| `pruned workloads with no recent telemetry` | Routine; a workload stopped reporting and aged out. |
