# P95 Autopilot

**by P95 Labs**

> A read-only Kubernetes-native diagnostics and recommendation system for AI inference workloads.

(https://github.com/p95labs/inference-fabric-autopilot/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](./LICENSE)
![Status: Alpha](https://img.shields.io/badge/status-alpha-orange.svg)

---

> **Alpha status.** This project is under active development. APIs, config schema,
> and metric mappings may change without notice. Not production-hardened.
> Do not use it as a control plane in production without understanding its current limitations.

---

## What it does

P95 Autopilot watches AI inference workloads running on Kubernetes, collects
Prometheus telemetry, and produces actionable recommendations — without touching
anything it observes.

- Scrapes Prometheus `/metrics` endpoints from inference pods (vLLM supported today)
- Watches Kubernetes pod and workload state via `client-go`
- Evaluates 11 rule-based recommendations per scrape: GPU pressure, KV cache saturation, queue depth, p95/p99 latency, TTFT degradation, error rate, token throughput, autoscaling lag
- Exposes a JSON HTTP API (`/telemetry`, `/recommendations`, `/workloads`, `/healthz`)
- Ships a CLI (`ifa`) for terminal-based inspection
- Runs fully in-cluster — no external SaaS, no phone-home, no internet required
- Supports air-gapped / restricted-environment deployment via a packaged bundle

## What it does not do

- Does **not** create, update, patch, or delete any Kubernetes resource
- Does **not** exec into pods or run commands inside containers
- Does **not** collect prompt bodies, response payloads, or user-identifying data
- Does **not** call external APIs at runtime
- Does **not** make autonomous changes to workloads

---

## Architecture

```
┌─────────────────────────────────────────────────────┐
│                   Kubernetes Cluster                 │
│                                                     │
│  ┌──────────────────────────────────────────────┐   │
│  │           P95 Autopilot Control Plane        │   │
│  │                                              │   │
│  │  ┌─────────────┐   ┌──────────────────────┐ │   │
│  │  │  Collector  │   │     Recommender      │ │   │
│  │  │  (Prometheus│──▶│  11 rule-based rules │ │   │
│  │  │  + k8s pod  │   │  vLLM-aware          │ │   │
│  │  │  watcher)   │   └──────────────────────┘ │   │
│  │  └─────────────┘            │                │   │
│  │         │                   ▼                │   │
│  │         ▼           ┌──────────────┐         │   │
│  │  ┌─────────────┐    │  HTTP API    │         │   │
│  │  │  Telemetry  │    │  /telemetry  │         │   │
│  │  │  Store      │    │  /recommend. │         │   │
│  │  │  (in-memory │    │  /workloads  │         │   │
│  │  │  + optional │    │  /healthz    │         │   │
│  │  │  TimescaleDB│    └──────────────┘         │   │
│  │  └─────────────┘                             │   │
│  └──────────────────────────────────────────────┘   │
│                           ▲                         │
│                    read-only scrape                  │
│                           │                         │
│  ┌────────────┐   ┌────────────────┐                │
│  │ vLLM pod   │   │  Other inference│               │
│  │ /metrics   │   │  workloads     │                │
│  └────────────┘   └────────────────┘                │
└─────────────────────────────────────────────────────┘
```

All Kubernetes API access uses `get`, `list`, and `watch` verbs only.
See [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) for the full component map and data flow.

---

## Quickstart (local, no cluster needed)

```bash
git clone https://github.com/<your-username>/inference-fabric-autopilot.git
cd inference-fabric-autopilot

# Run with simulated telemetry — no Kubernetes or database required
go run ./cmd/control-plane/
```

```bash
# In another terminal
curl http://localhost:8080/healthz
curl http://localhost:8080/telemetry | jq .
curl http://localhost:8080/recommendations | jq .
```

See [`examples/sample-telemetry.json`](./examples/sample-telemetry.json) and
[`examples/sample-recommendations.json`](./examples/sample-recommendations.json)
for the shape of the API responses.

---

## CLI

```bash
# Build the CLI
go build -o ifa ./cmd/ifa/

# Usage
./ifa telemetry          # Show latest telemetry snapshots
./ifa recommendations    # Show current recommendations
./ifa workloads          # Show Kubernetes-discovered workloads

# Point at a non-default control plane
IFA_URL=http://my-cluster:8080 ./ifa recommendations
```

---

## API endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness check — returns `ok` |
| `GET` | `/telemetry` | Latest telemetry snapshot per workload |
| `GET` | `/recommendations` | Current recommendations across all workloads |
| `GET` | `/workloads` | Kubernetes-discovered inference workloads |

All responses are JSON. No authentication is required (alpha limitation — restrict via NetworkPolicy).

---

## Recommendation rules

| Rule | Trigger | Severity |
|---|---|---|
| 1 | GPU idle with queue pressure | warning |
| 2 | p95 latency exceeds threshold | warning |
| 3 | p99 latency exceeds threshold | warning |
| 4 | GPU memory near capacity | critical |
| 5 | High error rate | critical |
| 6 | Queue depth exceeds threshold | warning |
| 7 | Low token throughput at high GPU utilization | info |
| 8 | Autoscaling lag (replicas too low for RPS) | warning |
| 9 | vLLM queue pressure (num_requests_waiting) | warning |
| 10 | vLLM KV cache near capacity | critical |
| 11 | vLLM p95 TTFT degradation | warning |

Rules 9–11 fire only for `runtime: vllm` workloads.

---

## Running tests

```bash
go test ./...
go vet ./...
```

Expected output:
```
ok  github.com/.../internal/config
ok  github.com/.../internal/recommender
ok  github.com/.../internal/runtime/vllm
ok  github.com/.../internal/telemetry
```

---

## Deploy to Kubernetes

**Standard (Helm):**
```bash
kubectl create namespace inference

helm install autopilot deploy/helm/autopilot \
  --namespace inference \
  --values deploy/helm/autopilot/values.yaml \
  --wait

kubectl port-forward -n inference svc/autopilot 8080:8080 &
curl http://localhost:8080/healthz
```

**Air-gapped:**
```bash
# On internet-connected machine
./scripts/export-airgap-bundle.sh

# Transfer airgap-bundle.tar.gz to restricted machine, then:
./scripts/load-airgap-bundle.sh airgap-bundle.tar.gz --runtime docker
```

See [docs/AIRGAPPED_DEPLOYMENT.md](./docs/AIRGAPPED_DEPLOYMENT.md) for the full procedure.

---

## Configuration

`config.yaml` at the repo root controls all runtime behaviour:

```yaml
collector:
  mode: prometheus          # or: simulated
  interval_seconds: 15
  prometheus_targets:
    - workload_name: my-vllm
      namespace: inference
      runtime: vllm
      model_name: meta-llama/Meta-Llama-3-8B
      metrics_url: http://vllm-service:8000/metrics

recommender:
  thresholds:
    high_p95_latency_ms: 500.0
    high_queue_depth: 10
    high_gpu_mem_pct: 85.0

privacy:
  collect_prompt_bodies: false
  collect_response_bodies: false
  collect_user_identifiers: false
```

---

## Security posture

- ClusterRole uses `get`, `list`, `watch` only — no write verbs
- No prompt/response body collection by design
- NetworkPolicy template included for egress restriction
- HTTP API is unauthenticated (alpha limitation) — restrict with NetworkPolicy or ingress
- Default `config.yaml` DSN is a placeholder — change before any real deployment

See [SECURITY.md](./SECURITY.md) and [docs/SECURITY_MODEL.md](./docs/SECURITY_MODEL.md).

---

## Project status

| Phase | Description | Status |
|---|---|---|
| Phase 1 | Core foundation — API, collector, recommender, CLI | ✅ Complete |
| Phase 2 | Alpha hardening — config, dual telemetry backend, Helm | ✅ Complete |
| Phase 2.5 | Restricted-environment readiness — air-gap, RBAC, NetworkPolicy | ✅ Complete |
| Phase 3 | vLLM runtime validation — metrics adapter, parser, rules 9–11 | ✅ Complete |
| Phase 4 | GPU utilization via DCGM, counter-rate computation, histogram support | ✅ Complete |
| Phase 5 | Multi-runtime support (Triton, Ollama), operator UX improvements | 📋 Planned |

See [docs/ROADMAP.md](./docs/ROADMAP.md) for details.

---

## Documentation

| Document | Description |
|---|---|
| [docs/ARCHITECTURE.md](./docs/ARCHITECTURE.md) | Component map, data flow, dependencies |
| [docs/SECURITY_MODEL.md](./docs/SECURITY_MODEL.md) | Threat model, trust boundaries, read-only posture |
| [docs/DATA_COLLECTION.md](./docs/DATA_COLLECTION.md) | Exact list of what is and is not collected |
| [docs/RBAC_PERMISSIONS.md](./docs/RBAC_PERMISSIONS.md) | Every Kubernetes API verb, annotated |
| [docs/AIRGAPPED_DEPLOYMENT.md](./docs/AIRGAPPED_DEPLOYMENT.md) | Step-by-step air-gapped deployment guide |
| [docs/OPERATIONS_RUNBOOK.md](./docs/OPERATIONS_RUNBOOK.md) | Health checks, config changes, troubleshooting |
| [docs/VLLM_VALIDATION.md](./docs/VLLM_VALIDATION.md) | vLLM metrics, validation guide, known limitations |
| [docs/ROADMAP.md](./docs/ROADMAP.md) | Phase-by-phase roadmap with honest status |

---

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). Open an issue before starting non-trivial work.

## License

Apache 2.0 — see [LICENSE](./LICENSE).
