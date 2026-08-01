# Roadmap — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06
**Maintainer:** Solo founder project

---

## Principles

- Ship working, testable software at each phase — no vaporware milestones.
- Keep the system read-only until autonomous action is explicitly scoped and
  security-reviewed.
- Prioritise design-partner feedback over assumed requirements.
- Stay Go-first. No new language runtimes without a clear justification.

---

## Status Legend

| Symbol | Meaning |
|---|---|
| ✅ | Complete |
| 🔄 | In progress |
| 📋 | Planned — committed to next phase |
| 💡 | Considered — not yet committed |
| ❌ | Out of scope for now |

---

## Phase 1 — Core Foundation
**Status: ✅ Complete**

| Feature | Status |
|---|---|
| Go control plane with HTTP API (`/healthz`, `/telemetry`, `/recommendations`) | ✅ |
| Simulated telemetry for 4 workloads (vLLM, Triton, Ollama) | ✅ |
| In-memory telemetry store with rolling window | ✅ |
| Rule-based recommender — 8 rules | ✅ |
| TimescaleDB backend (optional) | ✅ |
| Typed config with YAML loading and defaults | ✅ |
| client-go pod watcher (read-only) | ✅ |
| Prometheus metric collector | ✅ |
| Node-agent DaemonSet stub | ✅ |
| `ifa` CLI (telemetry, recommendations, workloads commands) | ✅ |
| Dockerfiles for control plane and node agent | ✅ |
| Helm chart | ✅ |
| Kubernetes raw manifests | ✅ |
| 18/18 tests passing | ✅ |
| Local kind cluster deployment | ✅ |

---

## Phase 2 — Alpha Hardening
**Status: ✅ Complete**

| Feature | Status |
|---|---|
| Prometheus collector connected to real workload metrics | ✅ |
| Configurable scrape targets per workload | ✅ |
| KV-cache usage rule | ✅ |
| TTFT P95 rule | ✅ |
| Full typed config including all recommender thresholds | ✅ |
| Helm chart with configmap-driven config | ✅ |
| kind deployment with port-forward access | ✅ |

---

## Phase 2.5 — Restricted-Environment Readiness
**Status: 🔄 In progress**

Goal: Make IFA deployable and auditable in air-gapped, regulated, or
security-reviewed environments. No new runtime behaviour — foundation only.

| Feature | Status |
|---|---|
| `docs/ARCHITECTURE.md` | ✅ |
| `docs/SECURITY_MODEL.md` | ✅ |
| `docs/DATA_COLLECTION.md` | ✅ |
| `docs/RBAC_PERMISSIONS.md` | ✅ |
| `docs/AIRGAPPED_DEPLOYMENT.md` | ✅ |
| `docs/OPERATIONS_RUNBOOK.md` | ✅ |
| `docs/DESIGN_PARTNER_PILOT.md` | ✅ |
| `docs/ROADMAP.md` | ✅ |
| `internal/config/config.go` — add `deploymentMode`, `egress`, `privacy`, `rbac`, `prometheus` fields | 📋 |
| `internal/config/config_test.go` — air-gap defaults, privacy defaults | 📋 |
| `deploy/helm/autopilot/values-airgapped.yaml` | 📋 |
| `deploy/helm/autopilot/templates/networkpolicy.yaml` | 📋 |
| `scripts/export-airgap-bundle.sh` | 📋 |
| `scripts/load-airgap-bundle.sh` | 📋 |
| `README.md` update pointing to `docs/` | 📋 |

---

## Phase 3 — Observability and Operator UX
**Status: 📋 Planned**

Goal: Make IFA useful enough for a design partner to run daily without manual
intervention. Still read-only.

| Feature | Status |
|---|---|
| Control plane self-metrics endpoint (`/metrics` for Prometheus scrape of IFA itself) | 📋 |
| Structured JSON logging throughout (request logging, scrape outcomes) | 📋 |
| Per-workload recommendation history (last N recommendations, not just current) | 📋 |
| `ifa` CLI — `watch` mode (continuous refresh, like `kubectl get pods -w`) | 📋 |
| `ifa` CLI — `--output json/yaml/table` flag for all commands | 📋 |
| Config hot-reload without pod restart | 📋 |
| Helm chart — resource limits configurable per environment size | 📋 |
| Helm chart — separate ServiceAccounts for control plane and node agent | 📋 |
| Recommendation severity levels (info / warning / critical) | 📋 |
| TimescaleDB retention policy configuration | 📋 |

---

## Phase 4 — Signal Expansion
**Status: 💡 Considered**

Goal: Increase the quality and coverage of telemetry signals to reduce false
positives and enable more nuanced recommendations.

| Feature | Status |
|---|---|
| Node-agent real metric collection (GPU device metrics without eBPF) | 💡 |
| Per-pod resource request vs. actual usage comparison | 💡 |
| Model cold-start detection (first-token latency spike on scale-up) | 💡 |
| Batch vs. streaming workload differentiation | 💡 |
| Multi-namespace fleet view (single control plane, N namespaces) | 💡 |
| OpenTelemetry trace integration (read-only, span latency bucketing) | 💡 |
| Workload cost estimation (GPU-hours × request volume) | 💡 |

---

## Phase 5 — Controlled Automation
**Status: 💡 Considered — requires explicit security review before commitment**

Goal: Allow IFA to take narrow, well-defined, reversible actions — only with
explicit operator opt-in and hard safety constraints.

This phase will not start until:
- At least one design partner has validated Phase 3 recommendations as
  accurate and actionable
- A write-permission threat model has been reviewed
- Each action type is individually gated behind a config flag defaulting to `false`

| Feature | Status |
|---|---|
| Horizontal scaling recommendations with optional dry-run mode | 💡 |
| Annotation-based action approval (operator annotates a workload to allow a specific action) | 💡 |
| Audit log of every action attempted and its outcome | 💡 |
| Rollback tracking (did the action improve or worsen the signal?) | 💡 |
| Hard limits (max replica count, min replica count, no action on prod namespaces without explicit opt-in) | 💡 |

**Autonomous action without operator approval is not on the roadmap.**

---

## Permanently Out of Scope

These will not be built unless a design partner presents a compelling and
specific case:

| Item | Reason |
|---|---|
| Prompt or response body collection | Privacy. No use case justifies it for infrastructure recommendations. |
| External SaaS dependency at runtime | Air-gap compatibility. Self-hosted only. |
| eBPF-based kernel tracing | Complexity and privilege requirements outweigh benefit at this stage. |
| Rust rewrite | Go is sufficient. Rewriting for performance is premature. |
| Web dashboard / UI | CLI + API is sufficient for operator use. UI is high-effort, low-priority. |
| Cloud-managed version | Out of scope for a self-hosted, air-gap-first product. |

---

## Versioning

IFA uses a simple phase-based version label during alpha:

| Label | Meaning |
|---|---|
| Phase 1 | Core foundation complete |
| Phase 2 | Alpha hardening complete |
| Phase 2.5 | Restricted-environment readiness complete |
| Phase 3+ | Follows semantic versioning once stable enough for design-partner production use |

There is no SemVer commitment until Phase 3 is complete. Breaking changes
between phases are expected and will be documented in release notes.
