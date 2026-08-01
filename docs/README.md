# Inference Fabric Autopilot — Documentation Index

**Version:** Phase 2.5 (Restricted-Environment Readiness)  
**Status:** Alpha — active development, not production-hardened  
**Maintainer:** Solo founder project

---

## What This Product Is

Inference Fabric Autopilot (IFA) is a read-only Kubernetes-native telemetry and
recommendation system for AI inference workloads.

It watches running inference pods (vLLM, Triton, Ollama, or any Prometheus-compatible
runtime), collects performance metrics, and generates actionable recommendations —
without taking any autonomous action on the cluster.

---

## What This Product Is Not

- It does not write to Kubernetes (no deployments, no scaling, no restarts).
- It does not collect prompt bodies, response bodies, or user-identifying data.
- It does not call any external API at runtime.
- It does not require internet access to operate.
- It is not an autonomous agent.

---

## Document Index

| Document | Description |
|---|---|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | Component map, data flow, dependencies |
| [SECURITY_MODEL.md](./SECURITY_MODEL.md) | Threat model, trust boundaries, read-only posture |
| [DATA_COLLECTION.md](./DATA_COLLECTION.md) | Exact list of what is and is not collected |
| [RBAC_PERMISSIONS.md](./RBAC_PERMISSIONS.md) | Every Kubernetes API verb the system uses, annotated |
| [AIRGAPPED_DEPLOYMENT.md](./AIRGAPPED_DEPLOYMENT.md) | Step-by-step guide for restricted/air-gapped environments |
| [OPERATIONS_RUNBOOK.md](./OPERATIONS_RUNBOOK.md) | Day-2 operations: health checks, config changes, upgrades |
| [DESIGN_PARTNER_PILOT.md](./DESIGN_PARTNER_PILOT.md) | Pilot scope, what we ask of partners, what they receive |
| [ROADMAP.md](./ROADMAP.md) | Phase-by-phase roadmap with honest status |

---

## Quick Component Summary

```
ifa-control-plane   Go HTTP server. Exposes /healthz, /telemetry, /recommendations.
                    Reads from Kubernetes API (read-only). Runs recommender engine.

ifa-node-agent      DaemonSet stub. Runs on every node. Currently reports node
                    identity. Designed for future local metric collection.

ifa CLI             Command-line client. Calls control-plane API over port-forward
                    or in-cluster service. Commands: telemetry, recommendations, workloads.
```

---

## Deployment Modes

| Mode | Description |
|---|---|
| `standard` | Default. In-cluster. Requires Prometheus reachable inside the cluster. |
| `airgapped` | Restricted environments. No external image pulls. No egress. Local registry. NetworkPolicy enforced. |

---

## Source Layout

```
cmd/                   Entrypoints: control-plane, node-agent, CLI
internal/
  api/                 HTTP handlers
  collector/           Prometheus + simulated metric collection
  config/              Config loading and validation
  k8s/                 Kubernetes client, pod watcher
  recommender/         Rule engine (8 rules)
  telemetry/           In-memory + TimescaleDB backends
deploy/
  kubernetes/          Raw manifests for kubectl apply
  helm/autopilot/      Helm chart (standard + air-gapped values)
  examples/            Example workload manifests (vLLM, Triton, Ollama)
scripts/               Local run, air-gap bundle export/load
docs/                  This folder
```

---

## Running Locally

```bash
# Start control plane (simulated mode, no Kubernetes required)
go run ./cmd/control-plane/

# Check health
curl http://localhost:8080/healthz

# Get recommendations
curl http://localhost:8080/recommendations | jq .

# Via CLI
./ifa telemetry
./ifa recommendations
./ifa workloads
```

---

## Current Test Status

```bash
go test ./...
# Expected: 18/18 passing
```

---

## Contact

This is a solo-founder infrastructure product in active alpha development.
Not yet open source. Do not redistribute without permission.
