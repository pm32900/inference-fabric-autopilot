# Rule catalogue

Nineteen rules, each with a permanent code. Codes never change meaning; if a
rule's logic changes materially it gets a new code and the old one is retired.

The running instance serves this catalogue too, with the thresholds actually in
force:

```bash
curl -s localhost:8080/api/v1/rules | jq '.items[] | {code, severity, title}'
curl -s localhost:8080/api/v1/rules | jq .thresholds
```

## How rules are written

Three properties apply to all of them.

**Rules refuse to fire on absent data.** Every measurement is optional. A
comparison against something the runtime did not report is false, never true and
never "0 < threshold". A vLLM deployment with no DCGM endpoint produces no GPU
findings at all rather than findings about a GPU sitting at 0%.

**Most rules require the condition to persist.** `sustain_for` (default 45s) is
how long. A queue that is deep for one scheduler step is normal; one that is
deep for a minute is not. Rules that use the sustain window report the span they
observed in the `window_seconds` field.

**Specific diagnoses supersede generic symptoms.** When `IFA-LAT-001` explains a
slow TTFT as queueing, the generic `IFA-LAT-003` ("latency is high") is
suppressed for that workload. You get one finding that says what to do, not five
that each say a different number is large.

## Suppression graph

```
IFA-SCL-001  ──suppresses──▶  IFA-CAP-001
IFA-CAP-004  ──suppresses──▶  IFA-CAP-001, IFA-CAP-002
IFA-KV-001   ──suppresses──▶  IFA-KV-002
IFA-CAP-001  ──suppresses──▶  IFA-LAT-003
IFA-CAP-002  ──suppresses──▶  IFA-LAT-003
IFA-LAT-001  ──suppresses──▶  IFA-LAT-003
IFA-LAT-002  ──suppresses──▶  IFA-LAT-003
```

---

## Capacity and scheduling

### `IFA-CAP-001` — Requests queueing while the GPU is saturated
**warning** · all runtimes · sustained · suppresses `IFA-LAT-003`

Fires when `requests_waiting > queue_waiting_requests` **and**
`gpu_utilization_percent > gpu_util_high_pct`, both held for the sustain window.

The accelerator is doing useful work and still cannot keep up. Raising the
runtime's concurrency limit here lengthens the queue rather than draining it —
more concurrent sequences compete for the same compute.

*Inputs:* `vllm:num_requests_waiting` (or `nv_inference_pending_request_count`),
`DCGM_FI_DEV_GPU_UTIL`. Requires a DCGM endpoint.

### `IFA-CAP-002` — Requests queueing while the GPU is idle
**warning** · all runtimes · sustained · suppresses `IFA-LAT-003`

Fires when `requests_waiting > queue_waiting_requests` **and**
`gpu_utilization_percent < gpu_util_low_pct`, both held for the sustain window.

The exact mirror of `IFA-CAP-001`, and the reason a single-signal queue alert is
not actionable: identical queue depth, opposite fix. Work is arriving and not
reaching the accelerator, which is an admission or batching limit — `max_num_seqs`,
a batch-size cap, or a `max_model_len` set far above real prompt lengths, which
reserves KV blocks per sequence and caps how many can run concurrently.

The gap between `gpu_util_low_pct` (35) and `gpu_util_high_pct` (85) is
deliberate. A workload at 60% is neither saturated nor obviously misconfigured,
and firing on it produces advice nobody can act on.

### `IFA-CAP-003` — Queue is growing, not just deep
**critical** · all runtimes · trend-based

Fires when the queue is above threshold **and** the mean of the last third of the
trend window exceeds the mean of the first third by more than one request.

A deep but flat queue is a steady state: latency is worse than you would like,
but it is not getting worse. A growing queue does not reach equilibrium on its
own. Separating the two is the difference between "tune this next sprint" and
"act now".

The trend window is `sustain_for × 3` and needs at least six samples, so a
scrape interval much larger than `sustain_for / 6` keeps this rule dormant.

### `IFA-CAP-004` — Waiting requests are deferred, not capacity-bound
**warning** · vLLM only · suppresses `IFA-CAP-001`, `IFA-CAP-002`

Fires when `vllm:num_requests_waiting_by_reason{reason="deferred"}` exceeds the
`capacity` count and the total queue is above threshold.

vLLM defers a request when something other than scheduler space blocks it: a
LoRA-adapter budget, an in-flight KV transfer, a blocked request state. These do
not move when you add replicas. Without this rule the workload looks exactly like
a capacity shortage.

*Requires* vLLM V1 or newer; the metric does not exist on older builds, and the
rule stays dormant rather than guessing.

---

## KV cache

### `IFA-KV-001` — KV cache exhausted: requests are being preempted
**critical** · all runtimes · suppresses `IFA-KV-002`

Fires when `preemptions_per_sec > preemptions_per_sec` (default 0.01).

This is the rule that justifies the whole approach. KV-cache *utilisation* is a
proxy: a workload can sit at 97% indefinitely with no harm at all, because using
the cache you paid for is the point. Preemption is the symptom — the engine is
evicting in-flight requests to reclaim blocks, and everything it evicts is
recomputed from scratch. It costs GPU time and adds latency simultaneously.

The default threshold is just above zero because a steady-state preemption rate
is not normal.

*Inputs:* `vllm:num_preemptions_total`, converted to a rate across scrapes.

### `IFA-KV-002` — KV cache near capacity
**warning** · all runtimes · sustained

Fires when `kv_cache_usage_percent > kv_cache_high_pct` (default 90) for the
sustain window, and `IFA-KV-001` did not fire.

Reported as headroom rather than harm: no requests are being evicted yet, but a
burst of long prompts would start evicting. If traffic is steady this needs no
action, and the finding says so.

---

## Latency

### `IFA-LAT-001` — Time to first token dominated by queueing
**warning** · all runtimes · suppresses `IFA-LAT-003`

Fires when `ttft_p95_ms > ttft_p95_ms` and queue time accounts for at least
`queue_share_of_ttft_pct` (default 50%) of it.

A capacity problem wearing a latency costume. The user-visible delay is time
spent waiting to be admitted, not time spent processing the prompt, so prefill
tuning would barely move it.

The two percentiles come from separate histograms with separate bucket
boundaries, so the split is an estimate; interpolation error can push the ratio
above 1 and the rule clamps it rather than reporting a share above 100%.

*Requires* `vllm:request_queue_time_seconds` (V1) or Triton's
`nv_inference_queue_summary_us`. Without it, neither this rule nor `IFA-LAT-002`
can attribute the latency, and both stay dormant.

### `IFA-LAT-002` — Time to first token dominated by prefill
**warning** · all runtimes · suppresses `IFA-LAT-003`

Fires when `ttft_p95_ms` is above target and queue time is *below*
`queue_share_of_ttft_pct` of it.

Requests are admitted promptly and then spend the time in prefill. The fix is
chunked prefill and prefix caching, or shorter prompts. Adding replicas does not
reduce the cost of processing a long prompt.

### `IFA-LAT-003` — End-to-end p95 latency above target
**warning** · all runtimes · sustained

Fires when `p95_latency_ms > e2e_p95_ms` for the sustain window and no more
specific rule explained it.

Deliberately a symptom, not a diagnosis. When it survives suppression it means
none of the queueing, cache-pressure or throughput rules matched — usually a
change in generation length rather than a resource problem.

### `IFA-LAT-004` — Tail latency far above p95
**info** · all runtimes

Fires when `p99 / p95 > tail_ratio_p99_p95` (default 3).

A gap that wide is a mixture, not a distribution: a minority of requests is doing
substantially more work, or is stuck behind requests that are. Usually long-context
requests sharing a batch with short ones, which routing fixes without any change
in capacity.

---

## Efficiency

### `IFA-EFF-001` — Prefix cache is not paying for itself
**info** · vLLM only

Fires when `prefix_cache_hit_rate_percent < prefix_cache_hit_low_pct` (default
10%) while prompt-token throughput is above 50 tokens/second.

The throughput floor matters: a hit rate computed from tiny counter deltas is
noise, and a near-idle workload would otherwise produce a permanent finding.

The usual cause is not a disabled cache but routing — requests that share a
system prompt or conversation history are being spread across replicas, so no
replica ever sees the prefix twice. Prefix-aware or session-affinity routing at
the load balancer fixes it.

Computed from the *rate* of `vllm:prefix_cache_hits_total` over
`vllm:prefix_cache_queries_total` between scrapes, not the lifetime ratio, so a
workload that was bad an hour ago stops reporting once it recovers.

### `IFA-EFF-002` — GPU busy but token throughput low
**info** · all runtimes

Fires when `gpu_utilization_percent > gpu_util_high_pct` and
`tokens_per_second < tokens_per_second_low`.

GPU utilisation counts any kernel activity, so a prefill-heavy workload looks
fully busy while producing very little user-visible output. Comparing prompt-token
throughput against generation-token throughput tells you which it is.

### `IFA-EFF-003` — GPU memory near capacity
**warning** · all runtimes · sustained

Fires when `gpu_memory_used_percent > gpu_memory_high_pct` for the sustain window.

For a workload with a preallocated KV cache this is often by design, and the
finding says so: check whether the number moves at all before treating it as a
leak. A static figure is the runtime's own reservation; a climbing one usually
means another process on the device or a growing set of loaded adapters.

---

## Errors

### `IFA-ERR-001` — Elevated error rate
**critical** · all runtimes · sustained

Fires when `error_rate_percent > error_rate_pct`.

**Never fires for vLLM.** vLLM exposes no failure counter — only
`vllm:request_success_total` broken down by finished reason — so there is no
error rate to derive, and IFA does not invent one. Triton has
`nv_inference_request_failure` and does support this rule.

### `IFA-ERR-002` — Clients are abandoning requests
**warning** · all runtimes · sustained

Fires when `abort_rate_percent > abort_rate_pct` (default 5%).

The runtime records an abort when the caller goes away, so this is a
client-visible symptom even though no request failed: users or upstream timeouts
are giving up before the response arrives. Read it alongside TTFT — if TTFT is
also high, the aborts are the downstream effect and the latency is the thing to
fix.

Sustained because rates derived from integer counters over one scrape interval
are lumpy: a single abort in a quiet interval reads as a large percentage.

---

## Scaling

Both scaling rules need Kubernetes discovery enabled. Without it they stay
dormant rather than treating an unknown deployment as one with zero replicas.

### `IFA-SCL-001` — Saturated at the replica ceiling
**critical** · all runtimes · sustained · suppresses `IFA-CAP-001`

Fires when the queue is above threshold, `replicas >= max_replicas`, and the
ceiling comes from a HorizontalPodAutoscaler.

Horizontal scaling has nothing left to give, so "add replicas" is not available
advice — which is why it supersedes `IFA-CAP-001`. Either raise the HPA maximum,
if there is cluster capacity, or reduce per-request cost. When GPU nodes are the
constraint this is a cluster-capacity decision, not a workload one.

### `IFA-SCL-002` — Replicas unavailable while the queue builds
**critical** · all runtimes

Fires when `ready_replicas` is at least one below `replicas` while the queue is
above threshold.

Capacity has been asked for and is not arriving. For GPU workloads the usual
causes are pods pending on unavailable accelerators, a long model download, or a
readiness probe that fails during warm-up.

---

## Observability of the observed workload

### `IFA-OBS-001` — Telemetry incomplete for this workload
**info** · all runtimes

Fires when the scrape succeeded but expected metrics were absent from the
payload.

Reported so that partial coverage does not read as a clean bill of health. A
workload with half its metrics missing produces few findings, and without this
rule that silence looks identical to health. The finding lists exactly which
metrics were missing.

Optional metrics — those that legitimately do not exist on some versions or
without a feature enabled — are excluded, because noise here trains people to
ignore the field.

### `IFA-OBS-002` — Telemetry is stale
**warning** · all runtimes

Fires when the newest successful scrape is older than `stale_after` (default 2m).

Every other finding for that workload is being evaluated against telemetry that
may no longer describe it. A failed scrape writes no snapshot at all — writing a
zeroed one would let rules diagnose a workload the collector cannot see, and
would stop this rule from ever firing.

---

## Tuning

Thresholds are global, not per-workload — a real limitation when a batch pipeline
and a chat endpoint share a cluster. The defaults assume interactive serving.

Starting points for a batch workload:

```yaml
recommender:
  thresholds:
    ttft_p95_ms: 30000        # nobody is watching a spinner
    e2e_p95_ms: 600000
    queue_waiting_requests: 200   # a deep queue is the design
    sustain_for: 5m               # slower reaction, far less noise
```

Changing a threshold requires a restart; the chart's config checksum annotation
rolls the pods when the ConfigMap changes.
