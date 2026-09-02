# Roadmap

Status language is deliberate. "Implemented" means the code exists and is unit
tested. "Validated" means it has been run against the real thing — and almost
nothing here has earned that word yet, which is the single most important fact
on this page.

| Term | Means |
|---|---|
| **Implemented** | Code exists, has tests, and is exercised by CI |
| **Locally validated** | Additionally exercised end to end against a simulated system |
| **Integration validated** | Additionally run against a real server or cluster |
| **Experimental** | Exists, but the design may change |
| **Planned** | Not started |

## Where things stand

| Area | Status |
|---|---|
| Prometheus exposition parsing — labels, histograms, summaries, edge cases | Locally validated |
| Rule engine — 19 rules, suppression, sustained conditions | Locally validated |
| vLLM adapter | Implemented |
| Triton adapter | Implemented |
| DCGM adapter | Implemented |
| HTTP API, `/api/v1` and legacy paths | Implemented |
| `ifa` CLI including `ifa check` | Implemented |
| Self-metrics | Implemented |
| Kubernetes discovery via informers | Implemented |
| Helm chart, RBAC, NetworkPolicy, security contexts | Implemented |
| TimescaleDB history | Implemented |
| Demo and its end-to-end test | Locally validated |

"Locally validated" for the parser, rules and demo means `make demo` and
`TestDemoScenariosProduceTheirIntendedDiagnosis` run the whole pipeline over a
real socket against vLLM-shaped exposition, and each simulated failure mode
produces its intended diagnosis.

Nothing is integration validated. That is the honest state of the project.

## Next, in order

### 1. Validate against a live vLLM server

The highest-value thing that could happen to this project, and it is not code.
Every metric name, label and bucket boundary comes from vLLM's own source, and
the fixtures are built from those definitions — but a fixture built from a spec
and a real payload are not the same artifact, and the first version of this
adapter is proof: it passed its tests and could not read a real server.

What would close it: `ifa check` output from a real vLLM deployment, plus the
version. Ten seconds of someone's time. If you run vLLM, [open an
issue](https://github.com/pm32900/inference-fabric-autopilot/issues) with the
output.

### 2. A kind-based integration test in CI

Install the chart into a kind cluster, run a small CPU-mode vLLM, assert that
IFA discovers it, scrapes it, and produces findings. This is what would move
Kubernetes discovery and the chart from "implemented" to "integration
validated", and it would catch the class of bug — an RBAC rule that is one verb
short, a Service port that does not match — that unit tests structurally cannot.

### 3. Per-pod GPU attribution

A DCGM Exporter endpoint reports the GPUs on a *node*, not the GPUs belonging to
one pod. On a node running several inference workloads, the utilisation IFA
attributes to a target may belong to something else. DCGM's Kubernetes-aware
labels carry pod and namespace; using them needs a mapping from workload to pods
that the informer cache already has.

Until this lands, GPU findings are trustworthy on dedicated GPU nodes and
approximate on shared ones. The docs say so; the API does not yet.

### 4. Per-workload thresholds

Thresholds are global. A batch scoring pipeline and an interactive chat endpoint
have genuinely different definitions of "slow", and sharing one set means either
the batch workload is permanently on fire or the chat endpoint's real problems
are under the line. A per-target override block, falling back to the global set,
is the obvious shape.

### 5. A finding lifecycle

Findings are stateless: each request re-evaluates from scratch. IDs are stable
while a condition holds, which is enough to deduplicate, but there is no
first-seen timestamp, no flap suppression, and no way to acknowledge one. Any of
those would need durable state and should not be built before someone actually
wants it.

## Considered and not planned

**Automatic remediation.** IFA holds no write permission and will not gain one.
See [ADR 0001](adr/0001-read-only.md).

**A built-in dashboard.** Grafana exists and is better at this. A Grafana
dashboard JSON that reads IFA's API would be a welcome contribution; a
hand-rolled UI in this repository would not be maintained.

**Machine-learned anomaly detection.** [ADR 0004](adr/0004-deterministic-rules.md).
There is nothing credible to train on, and an unexplainable finding is not
actionable at 3am.

**More runtimes for their own sake.** An Ollama adapter existed here and was
removed: its only route to throughput numbers was to treat per-request response
statistics as cumulative counters, which produces numbers that look plausible
and mean nothing. One adapter that is right beats three that are shallow.
SGLang and TGI both expose Prometheus metrics and would be genuine additions —
from someone who runs them and can check the output.

## Toward v1.0

Not close. It would need, at minimum: integration validation against real vLLM
and a real cluster; authentication on the API; per-workload thresholds; signed
images and an SBOM; and a stable API used by somebody other than the author.
