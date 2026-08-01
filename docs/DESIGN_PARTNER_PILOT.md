# Design Partner Pilot — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06
**Status:** Seeking first design partners

---

## What This Document Is

A clear, honest description of what a pilot engagement looks like — what IFA
does today, what we ask from a partner, what the partner receives in return,
and what the boundaries are.

This is not a sales document. It is written to help a potential partner make
an informed decision about whether a pilot is appropriate for their environment
and team.

---

## What IFA Does Today

IFA is a read-only Kubernetes-native telemetry and recommendation system for
AI inference workloads. It is in active alpha development.

**It currently does:**
- Watches running inference pods (vLLM, Triton, Ollama) in a Kubernetes cluster
- Scrapes Prometheus `/metrics` endpoints from those pods
- Stores performance snapshots in memory (optionally persisted to TimescaleDB)
- Evaluates 8 rule-based recommendation rules per workload per collection cycle
- Exposes a JSON HTTP API and a CLI for operator use
- Runs without internet access (air-gapped deployment supported)
- Deploys via Helm with a documented air-gapped values file

**It does not yet do:**
- Autonomous scaling or remediation actions
- Multi-cluster federation
- A web dashboard or UI
- ML-based anomaly detection or predictive scaling
- SLO tracking or alerting integration
- Cost attribution or cloud spend analysis

These are on the roadmap. See [ROADMAP.md](./ROADMAP.md).

---

## Pilot Scope

A pilot engagement is a **time-boxed, low-commitment technical evaluation**.

| Parameter | Value |
|---|---|
| Duration | 4–8 weeks |
| Commitment | 2–4 hours/week from one infrastructure or ML platform engineer |
| Environment | One non-production Kubernetes cluster (or a staging namespace) |
| Access required | None — partner deploys and operates IFA themselves |
| Data sent to us | None at runtime — IFA is fully self-contained |
| Changes to partner's cluster | Read-only RBAC, one namespace, one Deployment, one DaemonSet |

---

## What We Ask From a Partner

1. **Deploy IFA** into a non-production cluster running at least one inference
   workload with a Prometheus `/metrics` endpoint.

2. **Run it for 2–4 weeks** and let it collect telemetry from real workloads.

3. **Share structured feedback** (not raw data) on:
   - Whether the recommendations were accurate or misleading
   - Which recommendations were actionable vs. noise
   - What information was missing that would have made a recommendation more useful
   - Any operational friction in deployment or day-2 operations
   - Whether the air-gapped deployment path worked in your environment

4. **One 60-minute debrief call** at the end of the pilot period.

We do not ask for:
- Access to your cluster
- Access to your inference API
- Prompt or response data
- Model weights or configuration
- Any proprietary workload details beyond what is described above

---

## What the Partner Receives

1. **Early access** to Phase 2.5 and subsequent builds before public release.

2. **Direct input into the roadmap** — if a recommendation rule, data source,
   or integration is important to your environment, it moves up in priority.

3. **Air-gapped deployment support** — we will work through your specific
   restricted-environment constraints directly. No support ticket queue.

4. **Full documentation** — architecture, security model, data collection
   inventory, RBAC permissions, and operations runbook are provided upfront
   and kept current.

5. **No lock-in** — IFA is self-contained. There is no vendor service to
   subscribe to, no API key to revoke, no data to delete on your behalf.
   Uninstalling removes everything.

---

## Security and Privacy Commitments for the Pilot

These are hard commitments, not aspirational goals:

| Commitment | Detail |
|---|---|
| No prompt or response data collected | IFA scrapes only Prometheus `/metrics`. No inference API endpoints are contacted. |
| No data transmitted outside your cluster | IFA makes no outbound calls at runtime. Enforced by NetworkPolicy in air-gapped mode. |
| Read-only Kubernetes access | ClusterRole contains only `get`, `list`, `watch`. No write verbs. |
| No external SaaS dependency | IFA is fully self-hosted. No licence callbacks, no telemetry beacons, no analytics. |
| Full code available for review | You can read every file before deploying. The codebase is small and auditable. |

If your security or legal team has specific requirements beyond these, we will
work through them before you deploy anything.

---

## Ideal Partner Profile

IFA is most useful today to teams that:

- Run inference workloads on Kubernetes (self-managed or on-prem)
- Use vLLM, Triton, or any Prometheus-compatible inference runtime
- Have Prometheus already deployed in-cluster (or are willing to deploy it)
- Experience operational pain around GPU utilisation, latency spikes,
  or queue pressure — and currently have no automated visibility into these
- Operate in a restricted environment (air-gapped, no-egress, or regulated)
  and find that most inference tooling assumes cloud connectivity

IFA is **not** a fit today for teams that:
- Need a dashboard or UI immediately
- Need automated remediation (scaling, restarts) rather than recommendations
- Run inference on managed services where Prometheus metrics are not accessible
- Need multi-cluster federation from day one

---

## Pilot Kickoff Checklist

Before deploying, confirm:

- [ ] You have a Kubernetes cluster (1.26+) with at least one inference workload
- [ ] The workload exposes Prometheus `/metrics` (vLLM, Triton, or compatible)
- [ ] You have a CNI that supports `NetworkPolicy` (Calico, Cilium, Antrea)
      — only required for air-gapped mode
- [ ] You have Helm 3 installed on your operator workstation
- [ ] You have reviewed [SECURITY_MODEL.md](./SECURITY_MODEL.md) and
      [DATA_COLLECTION.md](./DATA_COLLECTION.md)
- [ ] Your team has approved the RBAC grant in [RBAC_PERMISSIONS.md](./RBAC_PERMISSIONS.md)
- [ ] For air-gapped environments: you have reviewed
      [AIRGAPPED_DEPLOYMENT.md](./AIRGAPPED_DEPLOYMENT.md) and confirmed
      your image transfer process

---

## Feedback Template

At the end of the pilot, we ask for feedback structured around these questions:

```
1. Which recommendations fired during the pilot?
   - Were they accurate?
   - Were they actionable?
   - Were any misleading or noisy?

2. What was missing?
   - What would have made a recommendation more useful?
   - What workload signals do you wish IFA could see?

3. Operations:
   - How long did deployment take?
   - Did the air-gapped path work? Any blockers?
   - Any day-2 operational issues?

4. Overall:
   - Would you continue using IFA after the pilot?
   - What would need to be true for it to become part of your standard stack?
```

You do not need to share workload names, cluster topology, or any internal
infrastructure details to answer these questions.

---

## Contact

To discuss a pilot, reach out directly. There is no sales process, no NDA
required to start a conversation, and no commitment to deploy before both
sides are comfortable.
