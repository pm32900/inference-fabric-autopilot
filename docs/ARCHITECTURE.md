# Architecture — Inference Fabric Autopilot

**Version:** Phase 2.5  
**Last updated:** 2026-06

---

## Overview

IFA is a passive observer. It reads metric and cluster state data, stores it
locally, and produces recommendations. It never writes to Kubernetes and never
calls external services at runtime.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                       │
│                                                                 │
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                  inference namespace                     │   │
│  │                                                          │   │
│  │   ┌─────────────────────┐    ┌────────────────────────┐  │   │
│  │   │  ifa-control-plane  │    │  Inference Workloads   │  │   │
│  │   │  (Deployment, x1)   │    │  vLLM / Triton / Ollama│  │   │
│  │   │                     │    │  (user-managed pods)   │  │   │
│  │   │  - HTTP API         │    └────────────┬───────────┘  │   │
│  │   │  - Recommender      │                 │ /metrics     │   │
│  │   │  - Collector        │◄────Prometheus──┘              │   │
│  │   │  - Telemetry store  │    scrape (pull, in-cluster)   │   │
│  │   └──────────┬──────────┘                                │   │
│  │              │ read-only                                  │   │
│  │              ▼                                            │   │
│  │   ┌─────────────────────┐                                │   │
│  │   │  Kubernetes API     │                                │   │
│  │   │  Server (in-cluster)│                                │   │
│  │   │  get/list/watch     │                                │   │
│  │   │  pods, nodes,       │                                │   │
│  │   │  deployments        │                                │   │
│  │   └─────────────────────┘                                │   │
│  │                                                          │   │
│  │   ┌─────────────────────┐                                │   │
│  │   │  ifa-node-agent     │                                │   │
│  │   │  (DaemonSet)        │                                │   │
│  │   │  one pod per node   │                                │   │
│  │   │  stub — reports     │                                │   │
│  │   │  node identity      │                                │   │
│  │   └─────────────────────┘                                │   │
│  │                                                          │   │
│  │   ┌─────────────────────┐  (optional)                    │   │
│  │   │  TimescaleDB        │◄──── control-plane writes      │   │
│  │   │  (in-cluster)       │      telemetry snapshots only  │   │
│  │   └─────────────────────┘                                │   │
│  └──────────────────────────────────────────────────────────┘   │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘

          │ kubectl port-forward or in-cluster service
          ▼
   ┌─────────────┐
   │   ifa CLI   │  (runs outside cluster, operator laptop)
   └─────────────┘
```

---

## Components

### ifa-control-plane

- **Language:** Go 1.22
- **Kind:** Kubernetes Deployment, 1 replica
- **Namespace:** `inference` (configurable)
- **Exposed ports:** 8080 (HTTP, internal only — no Ingress by default)

**Responsibilities:**
- Serves the HTTP API (`/healthz`, `/telemetry`, `/recommendations`)
- Runs the metric collector on a configurable interval
- Maintains an in-memory telemetry store (rolling window)
- Optionally persists snapshots to TimescaleDB
- Watches Kubernetes pods and nodes via client-go informers (read-only)
- Runs the rule-based recommender engine on each collection cycle

**What it does NOT do:**
- Does not patch, create, or delete any Kubernetes resource
- Does not send data outside the cluster
- Does not read prompt or response content from inference pods

---

### ifa-node-agent

- **Language:** Go 1.22
- **Kind:** Kubernetes DaemonSet (one pod per schedulable node)
- **Current state:** Stub — reports node identity on startup
- **Planned:** Future local metric collection (GPU device metrics, node-level
  resource pressure) without requiring privileged access

---

### ifa CLI (`ifa`)

- **Language:** Go 1.22, compiled binary
- **Runs:** On the operator's workstation, outside the cluster
- **Transport:** HTTP over `kubectl port-forward` or an in-cluster `ClusterIP` service

Commands:
```
ifa telemetry         Print current telemetry snapshot for all workloads
ifa recommendations   Print active recommendations
ifa workloads         List watched workloads and their runtime metadata
```

---

### Telemetry Store

Two backends, selectable at runtime:

| Backend | Config | Use case |
|---|---|---|
| In-memory | default | Development, short-lived deployments, air-gapped minimal installs |
| TimescaleDB | `database.enabled: true` | Persistent history, trend queries |

The in-memory store holds a fixed rolling window of snapshots per workload.
No data leaves the store. No external time-series SaaS is used.

---

### Recommender Engine

8 rule-based rules evaluated per workload per collection cycle:

| Rule | Signal | Trigger |
|---|---|---|
| GPU underutilisation | `gpu_utilisation_pct` | < `low_gpu_util_pct` threshold |
| GPU memory pressure | `gpu_memory_used_pct` | > `high_gpu_mem_pct` threshold |
| High P95 latency | `p95_latency_ms` | > `high_p95_latency_ms` threshold |
| High queue depth | `queue_depth` | > `high_queue_depth` threshold |
| High error rate | `error_rate_pct` | > `high_error_rate_pct` threshold |
| RPS-to-replica ratio | `requests_per_second` / replicas | > `min_replicas_for_rps` threshold |
| High KV-cache usage | `kv_cache_usage_pct` | > `high_kv_cache_usage_pct` threshold |
| High TTFT P95 | `ttft_p95_ms` | > `high_ttft_p95_ms` threshold |

All thresholds are configurable in `config.yaml`.

---

## Data Flow

```
Prometheus /metrics endpoint (in-cluster)
        │
        │  HTTP GET (pull, interval-based)
        ▼
  collector.Collect()
        │
        │  MetricSnapshot struct
        ▼
  telemetry.Store.Write()
        │
        ├──► in-memory ring buffer (always)
        └──► TimescaleDB INSERT (if enabled)
                │
                ▼
  recommender.Evaluate(snapshot)
        │
        │  []Recommendation
        ▼
  HTTP API — /telemetry, /recommendations
        │
        │  JSON response
        ▼
  ifa CLI or any HTTP client
```

No data moves outside this chain. The Kubernetes API watch runs in a separate
goroutine and feeds workload metadata (pod counts, labels) into the collector
context only.

---

## Deployment Modes

### Standard

```
collector.mode: prometheus   (or simulated for dev)
kubernetes.enabled: true
database.enabled: false      (or true if TimescaleDB is available)
```

Images pulled from wherever `controlPlane.image.repository` points.
No NetworkPolicy applied by default.

### Air-gapped

```
deploymentMode: airgapped
egress.enabled: false
collector.mode: prometheus
prometheus.url: http://prometheus.inference.svc.cluster.local:9090
```

- All images pre-loaded onto nodes or pushed to an internal registry
- `imagePullPolicy: Never` (or `IfNotPresent` pointing at internal mirror)
- NetworkPolicy: default-deny-egress on the `inference` namespace
- Explicit allow only for: Kubernetes API server, local Prometheus, local TimescaleDB
- No DNS lookups to external domains at runtime

See [AIRGAPPED_DEPLOYMENT.md](./AIRGAPPED_DEPLOYMENT.md) for the full procedure.

---

## External Dependencies at Runtime

| Dependency | Type | Required | Notes |
|---|---|---|---|
| Kubernetes API server | In-cluster | Yes (when `kubernetes.enabled: true`) | Read-only. Always in-cluster. |
| Prometheus | In-cluster | Yes (when `collector.mode: prometheus`) | Pull only. Must be reachable inside cluster. |
| TimescaleDB | In-cluster | No | Only if `database.enabled: true`. In-cluster only. |

**There are no external SaaS, cloud, or internet dependencies at runtime.**

---

## Technology Stack

| Layer | Technology |
|---|---|
| Language | Go 1.22 |
| HTTP framework | `net/http` (stdlib) |
| Kubernetes client | `client-go` v0.29 |
| Config | `gopkg.in/yaml.v3` |
| Database (optional) | TimescaleDB (PostgreSQL extension) |
| Packaging | Docker, Helm 3 |
| Local dev cluster | kind |
| Metrics source | Prometheus (pull model) |
