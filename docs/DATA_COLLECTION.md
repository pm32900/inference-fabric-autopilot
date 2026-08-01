# Data Collection — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06

---

## Purpose of This Document

This document provides an exact, field-level inventory of every data point
IFA collects, where it comes from, how long it is retained, and where it goes.
It is written for security reviewers, legal teams, and design partners evaluating
whether IFA is appropriate for their environment.

---

## Guiding Principle

IFA collects the minimum data required to produce infrastructure recommendations.
It is designed so that **no change to its code or configuration can expose
prompt bodies, response bodies, or user-identifying information**, because it
never connects to the inference API endpoints that carry that data.

---

## Data Sources

IFA has exactly two data sources:

| Source | Protocol | Direction | What it returns |
|---|---|---|---|
| Prometheus `/metrics` endpoint on inference pods | HTTP GET (pull) | IFA → pod | Aggregated numeric counters and gauges |
| Kubernetes API server | HTTPS (client-go watch) | IFA → API server | Resource metadata (names, labels, replica counts) |

There is no third data source. IFA does not open connections to inference API
endpoints, message queues, log aggregators, or any external service.

---

## Collected Fields — Telemetry Metrics

These fields are read from the Prometheus `/metrics` endpoint of each watched
inference workload. All values are numeric aggregates. None contain user data.

| Field | Type | Source metric | Description |
|---|---|---|---|
| `workload_name` | string | label | Name of the inference deployment |
| `namespace` | string | label | Kubernetes namespace |
| `runtime` | string | label | Runtime type: `vllm`, `triton`, `ollama` |
| `model_name` | string | label | Model identifier (e.g. `llama-3-8b`) |
| `gpu_utilisation_pct` | float64 | gauge | GPU compute utilisation, 0–100 |
| `gpu_memory_used_pct` | float64 | gauge | GPU memory used as % of total |
| `p95_latency_ms` | float64 | histogram summary | P95 end-to-end request latency in ms |
| `ttft_p95_ms` | float64 | histogram summary | P95 time-to-first-token in ms |
| `requests_per_second` | float64 | counter / rate | Throughput at scrape time |
| `queue_depth` | int | gauge | Pending requests in the inference queue |
| `error_rate_pct` | float64 | counter / rate | Fraction of requests returning errors, 0–100 |
| `kv_cache_usage_pct` | float64 | gauge | KV-cache utilisation, 0–100 |
| `timestamp` | time.Time | collector | When this snapshot was taken (local clock) |

**Total fields per snapshot: 13.**
None of these fields contain request content, user identifiers, or session data.

---

## Collected Fields — Kubernetes Metadata

These fields are read from the Kubernetes API server via read-only watch.

| Field | Resource | Purpose |
|---|---|---|
| `metadata.name` | Pod, Deployment, DaemonSet, Node | Workload identification |
| `metadata.namespace` | Pod, Deployment | Scope |
| `metadata.labels` | Pod, Deployment | Runtime and model label extraction |
| `status.phase` | Pod | Filter running vs. non-running pods |
| `spec.nodeName` | Pod | Node-to-pod mapping |
| `spec.replicas` | Deployment | Replica count for RPS-per-replica rule |
| `status.readyReplicas` | Deployment | Ready replica count |
| `status.conditions` | Node | Node health context |
| `status.capacity` | Node | Node resource capacity |

**IFA does not read:** `data` fields of ConfigMaps, `data` fields of Secrets,
pod logs, pod exec streams, or admission webhook payloads.

---

## Data Not Collected

The following table is explicit. These data types are **never** read by IFA
under any configuration.

| Data type | Reason not collected |
|---|---|
| Prompt text / input payload | IFA scrapes `/metrics` only, not the inference API |
| Response text / output payload | Same as above |
| HTTP request headers from inference calls | Not exposed via Prometheus metrics |
| Authentication tokens or API keys | Not in RBAC grant; not in `/metrics` |
| User identifiers (IP, user-agent, session ID) | Not in Prometheus metric format |
| Model weights or model files | IFA has no access to model storage volumes |
| Kubernetes Secrets (any field) | Not in RBAC grant |
| Pod logs (stdout/stderr) | Not in RBAC grant |
| Container filesystem contents | No volume mounts to workload pods |
| Inter-pod network traffic | No packet capture; no eBPF |
| GPU kernel traces | No privileged access; no eBPF |

---

## Privacy Configuration Fields

As of Phase 2.5, IFA introduces explicit privacy flags in `config.yaml`.
These document and enforce collection boundaries at the config layer.

```yaml
privacy:
  collect_prompt_bodies: false       # must remain false; no code path reads prompts
  collect_response_bodies: false     # must remain false; no code path reads responses
  collect_headers: false             # must remain false; not scraped
  collect_user_identifiers: false    # must remain false; not scraped
```

All four default to `false`. There is no code path that reads this data
regardless of config, but these fields exist so a security team can verify the
declared posture at a glance and include them in automated config audits.

---

## Data Retention

| Storage backend | Retention | Location | Configurable |
|---|---|---|---|
| In-memory store | Rolling window (last N snapshots per workload) | Process heap, control-plane pod | Not yet exposed as config |
| TimescaleDB | Indefinite until manually pruned | In-cluster PostgreSQL | Standard TimescaleDB retention policies |

Data is lost when the control-plane pod restarts (in-memory mode).
Data is not replicated outside the cluster under any configuration.

---

## Data Transit

| Leg | Transport | Encryption |
|---|---|---|
| Prometheus scrape (pod → collector) | HTTP | Plaintext (in-cluster) |
| Kubernetes API watch | HTTPS | TLS (standard in-cluster CA) |
| Control plane → TimescaleDB | TCP (PostgreSQL wire) | Plaintext by default; TLS configurable in DSN |
| CLI → control plane (port-forward) | HTTP over kubectl tunnel | kubectl tunnel is over TLS to API server |

No data transits outside the cluster boundary.

---

## Data Sharing

IFA does not share data with any third party. There are no:
- Telemetry callbacks to any vendor service
- Analytics pings
- Error reporting services (Sentry, Datadog, etc.)
- License validation calls

This is enforced structurally in air-gapped mode by the NetworkPolicy default-
deny-egress rule. In standard mode it is a code property: there are no HTTP
client calls to external hosts in the codebase.

---

## Audit Checklist for Security Reviewers

Use this checklist when evaluating IFA for a restricted environment:

- [ ] Review `internal/collector/` — confirm no HTTP calls to inference API endpoints
- [ ] Review `internal/k8s/` — confirm only `get`, `list`, `watch` verbs used
- [ ] Review `deploy/kubernetes/clusterrole.yaml` — confirm no write verbs
- [ ] Review `deploy/helm/autopilot/templates/rbac.yaml` — confirm same
- [ ] Review `config.yaml` privacy fields — confirm all `false`
- [ ] Review `deploy/helm/autopilot/templates/networkpolicy.yaml` — confirm default-deny-egress in air-gapped mode
- [ ] Run `go test ./...` — confirm 18/18 passing including config validation tests
- [ ] Verify image checksums from `checksums.sha256` before loading in air-gapped bundle
