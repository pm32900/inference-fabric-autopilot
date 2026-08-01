# vLLM Validation Guide

**Phase 3 — Real Runtime Validation**  
**Status:** In progress — parser and rules implemented, live cluster validation pending.

This document explains how P95 Labs validates against a real vLLM inference server,
which Prometheus metrics are expected, what recommendations fire under which conditions,
and how to run the validation end-to-end.

---

## What "vLLM validation" means

P95 Labs is a read-only diagnostics layer. Validation means:

1. P95 Labs can scrape a real vLLM `/metrics` endpoint without error.
2. The vLLM-specific fields in `telemetry.Snapshot` are populated correctly.
3. The recommender produces accurate, actionable recommendations based on real signal.
4. No false positives fire on a healthy, lightly loaded vLLM instance.

Validation does **not** require P95 Labs to modify vLLM or its configuration.

---

## vLLM metrics P95 Labs reads

All metrics are read from the standard Prometheus `/metrics` endpoint that vLLM
exposes on its HTTP server (default port `8000`).

| Metric name | Type | Unit | Mapped to |
|---|---|---|---|
| `vllm:num_requests_running` | gauge | count | `Snapshot.NumRequestsRunning` |
| `vllm:num_requests_waiting` | gauge | count | `Snapshot.NumRequestsWaiting`, `Snapshot.QueueDepth` |
| `vllm:gpu_cache_usage_perc` | gauge | fraction [0,1] | `Snapshot.KVCacheUsagePct` (×100) |
| `vllm:e2e_request_latency_seconds{quantile="0.5"}` | summary | seconds | `Snapshot.P50LatencyMs` (×1000) |
| `vllm:e2e_request_latency_seconds{quantile="0.95"}` | summary | seconds | `Snapshot.P95LatencyMs` (×1000) |
| `vllm:e2e_request_latency_seconds{quantile="0.99"}` | summary | seconds | `Snapshot.P99LatencyMs` (×1000) |
| `vllm:time_to_first_token_seconds{quantile="0.95"}` | summary | seconds | `Snapshot.TTFTP95Ms` (×1000) |
| `vllm:generation_tokens_total` | counter | tokens | `Snapshot.TokensPerSecond` (raw counter) |
| `vllm:prompt_tokens_total` | counter | tokens | `VLLMSnapshot.PromptTokensTotal` |
| `vllm:request_success_total` | counter | requests | `Snapshot.RequestRatePerSec` (raw counter) |
| `vllm:request_failure_total` | counter | requests | `Snapshot.ErrorRatePct` (raw counter) |

### Metric name note

vLLM uses a colon separator (`vllm:metric_name`) which is non-standard Prometheus
convention but intentional — it avoids collisions with other exporters on the same
scrape endpoint. All metric name constants are defined in
`internal/runtime/vllm/metrics.go` — update there if your vLLM version differs.

### Version assumptions

Metric names were validated against **vLLM v0.4.x / v0.5.x**. Earlier versions
(pre-0.4) used different names. If metrics are missing from a scrape, P95 Labs
treats them as zero and does not fire rules that depend on them.

---

## Recommendation rules for vLLM

Three rules fire exclusively for `runtime: vllm` workloads. Thresholds are
configurable in `config.yaml` under `recommender.thresholds`.

### Rule 9 — Queue pressure

| Field | Value |
|---|---|
| Trigger | `NumRequestsWaiting > high_queue_depth` (default: 10) |
| Severity | warning |
| Meaning | Requests are piling up faster than vLLM can schedule them |
| Action | Increase `max_num_seqs`, add a replica, or enable chunked prefill |

### Rule 10 — KV cache near capacity

| Field | Value |
|---|---|
| Trigger | `KVCacheUsagePct > high_kv_cache_usage_pct` (default: 80%) |
| Severity | critical |
| Meaning | KV cache blocks are nearly exhausted; vLLM will evict and recompute |
| Action | Reduce `max_model_len` or `max_num_seqs`, enable prefix caching, or use a larger GPU |

### Rule 11 — TTFT degradation

| Field | Value |
|---|---|
| Trigger | `TTFTP95Ms > high_ttft_p95_ms` (default: 2000ms) |
| Severity | warning |
| Meaning | Users wait too long for the first token — long prefill or queue depth |
| Action | Enable chunked prefill, reduce concurrent long-context requests, or scale out |

Generic rules (1–8) also apply to vLLM workloads where signals are available
(e.g. Rule 1 fires when GPU is idle with a full queue, Rule 2 fires on high p95 latency).

---

## How to run validation against a real vLLM instance

### Prerequisites

- A running vLLM server accessible from your machine or cluster
- `kubectl port-forward` or direct network access to vLLM's HTTP port (default `8000`)
- P95 Labs control plane running in `prometheus` collector mode

### Step 1 — Verify vLLM exposes metrics

```bash
curl http://<vllm-host>:8000/metrics | grep "vllm:"
```

You should see lines like:

```
vllm:num_requests_running 0
vllm:num_requests_waiting 0
vllm:gpu_cache_usage_perc 0.12
```

If you see no `vllm:` prefixed lines, your vLLM version may use different metric
names. Check `internal/runtime/vllm/metrics.go` and update the constants.

### Step 2 — Configure P95 Labs to scrape vLLM

Edit `config.yaml`:

```yaml
collector:
  mode: prometheus
  interval_seconds: 15
  prometheus_targets:
    - workload_name: vllm-llama3
      namespace: inference
      runtime: vllm
      model_name: llama-3-8b
      metrics_url: http://<vllm-host>:8000/metrics
```

### Step 3 — Start P95 Labs

```bash
go run ./cmd/control-plane/
```

### Step 4 — Check telemetry is populated

```bash
curl http://localhost:8080/telemetry | jq '.[] | select(.runtime == "vllm")'
```

Expected fields populated:

```json
{
  "workload_name": "vllm-llama3",
  "runtime": "vllm",
  "num_requests_running": 3,
  "num_requests_waiting": 0,
  "kv_cache_usage_pct": 14.2,
  "ttft_p95_ms": 320.0,
  "p95_latency_ms": 1100.0
}
```

### Step 5 — Check recommendations

```bash
curl http://localhost:8080/recommendations | jq .
```

On a healthy, lightly loaded vLLM instance: **no recommendations** (empty array).

### Step 6 — Trigger rules intentionally (load test)

Use the vLLM OpenAI-compatible API to generate load:

```bash
# Send 20 concurrent requests to fill the queue
for i in $(seq 1 20); do
  curl -s http://<vllm-host>:8000/v1/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"llama-3-8b","prompt":"Explain quantum computing in detail","max_tokens":512}' &
done
wait
```

Then check recommendations — you should see Rule 9 (queue pressure) and possibly
Rule 11 (TTFT degradation) fire.

---

## Validating with the parser in isolation (no live cluster needed)

The parser is fully testable without a live vLLM instance.

```bash
go test ./internal/runtime/vllm/... -v
```

To test against a real `/metrics` payload, save the output and pass it to `Parse()`:

```bash
# Save real metrics to a fixture file
curl http://<vllm-host>:8000/metrics > testdata/vllm-real.txt

# Then add a test in parser_test.go that reads this file:
# body, _ := os.ReadFile("testdata/vllm-real.txt")
# snap := Parse(string(body))
```

---

## Current limitations

| Limitation | Detail |
|---|---|
| Counter-based rate | `TokensPerSecond` and `RequestRatePerSec` store raw counter values, not computed rates. Rate computation (delta between scrapes) is not yet implemented. |
| No GPU utilization signal | vLLM does not emit a direct GPU compute utilization metric. `GPUUtilizationPct` is approximated from `kv_cache_usage_perc`, which is not the same thing. True GPU utilization requires DCGM or nvidia-smi integration (Phase 4). |
| Single quantile TTFT | Only the p95 quantile is used for Rule 11. P50 and P99 are parsed but not yet used in rules. |
| No histogram support | vLLM may emit some metrics as histograms (with `_bucket` / `_count` / `_sum` suffixes) in newer versions. The current parser reads summary quantiles only. |
| Metric name drift | vLLM metric names changed between v0.3 and v0.4. If your deployment uses an older version, update constants in `internal/runtime/vllm/metrics.go`. |

---

## Next steps

- [ ] Implement rate computation (delta between scrapes) for counter metrics
- [ ] Add `testdata/` fixtures from real vLLM deployments
- [ ] Add `ifa report` CLI command for a single-command workload diagnostic summary
- [ ] Validate against vLLM v0.6.x metric name changes
- [ ] Add Triton Inference Server adapter following the same pattern
