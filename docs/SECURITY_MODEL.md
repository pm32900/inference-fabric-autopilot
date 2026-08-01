# Security Model — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06

---

## Summary

IFA is a read-only passive observer. It holds no write permissions on the
cluster. It does not transmit data outside the cluster. It does not read
inference request or response content. The attack surface is intentionally
minimal.

---

## Trust Boundaries

```
┌─────────────────────────────────────────────────┐
│  Kubernetes Cluster (trusted boundary)          │
│                                                 │
│  ┌────────────────────┐                         │
│  │  inference ns      │  IFA pods run here      │
│  │                    │  NetworkPolicy: deny     │
│  │  control-plane ────┼──► K8s API (read-only)  │
│  │  node-agent        │  ◄── Prometheus (pull)  │
│  └────────────────────┘                         │
│                                                 │
│  Cluster boundary — no data crosses this line   │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  Operator workstation (trusted, out-of-band)    │
│                                                 │
│  ifa CLI ──► kubectl port-forward ──► API :8080 │
└─────────────────────────────────────────────────┘

External internet: NOT reachable from IFA pods at runtime.
```

---

## Kubernetes RBAC Posture

IFA uses a single `ClusterRole` with **read-only verbs only**: `get`, `list`, `watch`.

Resources accessed:
- `pods`, `nodes`, `namespaces` (core API group)
- `deployments`, `daemonsets`, `replicasets` (apps API group)

**No write verbs are granted.** There is no `create`, `update`, `patch`,
`delete`, `deletecollection`, or `escalate` in any IFA role.

See [RBAC_PERMISSIONS.md](./RBAC_PERMISSIONS.md) for the full annotated permission set.

---

## What IFA Can Read from the Cluster

| Resource | Fields used | Purpose |
|---|---|---|
| Pod | `name`, `namespace`, `labels`, `status.phase`, `spec.nodeName` | Identify running inference workloads |
| Node | `name`, `status.conditions`, `status.capacity` | Node health context |
| Deployment | `name`, `namespace`, `spec.replicas`, `status.readyReplicas` | Replica count for RPS-per-replica rule |
| DaemonSet | `name`, `namespace`, `status` | Agent health tracking |
| Namespace | `name` | Namespace enumeration |

**IFA does not read:** ConfigMaps, Secrets, ServiceAccounts, RBAC resources,
admission webhooks, or any resource in `kube-system`.

---

## What IFA Does Not Collect

The following data is **never read, stored, or transmitted** by IFA:

| Data type | Status | Notes |
|---|---|---|
| Prompt bodies | Never collected | IFA scrapes only Prometheus `/metrics`, not inference API endpoints |
| Response bodies | Never collected | Same as above |
| HTTP request headers | Never collected | Not part of Prometheus metric format |
| User identifiers / tokens | Never collected | Not accessible via `/metrics` |
| Model weights or files | Never collected | IFA has no access to model storage |
| Kubernetes Secrets | Never collected | Not in RBAC grant |
| ConfigMaps | Never collected | Not in RBAC grant |
| Inter-pod network traffic | Never collected | No packet capture, no eBPF |

See [DATA_COLLECTION.md](./DATA_COLLECTION.md) for the complete field-level list.

---

## What IFA Does Collect

IFA collects only **aggregated performance metrics** from the Prometheus
`/metrics` endpoint exposed by inference runtimes:

- GPU utilisation percentage
- GPU memory used percentage
- P95 request latency (milliseconds)
- Time-to-first-token P95 (milliseconds)
- Request queue depth
- Error rate percentage
- Requests per second
- KV-cache usage percentage
- Workload name, namespace, runtime type, model name (metadata labels)

These are infrastructure-level counters. None of them contain user data.

---

## Network Exposure

### Control plane pod

- Listens on TCP 8080 inside the cluster only.
- No `Ingress` or `LoadBalancer` Service is created by default.
- Access is via `ClusterIP` service or `kubectl port-forward`.
- In air-gapped mode, a `NetworkPolicy` enforces default-deny-egress
  on the `inference` namespace with explicit allow-only rules for
  in-cluster destinations.

### Node agent pod

- No listening port. Outbound only (to control plane, future use).
- Current stub makes no network calls.

---

## Image Supply Chain

### Standard mode
- Images pulled from wherever `controlPlane.image.repository` is set.
- Operator is responsible for image provenance.

### Air-gapped mode
- Images must be pre-loaded via `scripts/load-airgap-bundle.sh` or
  pushed to an internal registry before deployment.
- `imagePullPolicy: Never` prevents any pull attempt at runtime.
- The export script (`scripts/export-airgap-bundle.sh`) produces
  `checksums.sha256` alongside each image tarball for integrity verification.
- IFA does not sign images itself. If your environment requires image signing
  (e.g. Cosign, Notary), that is the operator's responsibility.

---

## Secrets Management

IFA currently has one optional secret: the TimescaleDB DSN.

- In development it is stored in `config.yaml` (plaintext, local only).
- In Kubernetes, the DSN should be passed via a Kubernetes Secret mounted
  as an environment variable, not hardcoded in the ConfigMap.
- The Helm chart does not currently automate Secret creation. The operator
  must create it manually and reference it in `values.yaml`.

No other credentials, API keys, or tokens are used by IFA.

---

## Threat Model

### Threats considered and mitigated

| Threat | Mitigation |
|---|---|
| IFA pod compromised — attacker escalates to cluster write | No write verbs in ClusterRole. Compromise is limited to read access on listed resources. |
| IFA exfiltrates inference data | IFA never reads inference API endpoints. Only Prometheus `/metrics` is scraped. |
| IFA makes outbound calls to exfiltrate data | NetworkPolicy (air-gapped mode) blocks all egress except explicit in-cluster allowlist. Standard mode relies on cluster-level egress controls. |
| Malicious image injected into air-gapped bundle | Checksums provided. Operator should verify before loading. |
| TimescaleDB DSN leaked via ConfigMap | DSN should be in a Kubernetes Secret, not in the ConfigMap. See Secrets Management above. |
| Control plane API abused to enumerate cluster state | API is read-only and only accessible in-cluster or via port-forward. No auth on the HTTP API currently — see Limitations. |

### Threats not yet mitigated (known gaps)

| Gap | Notes |
|---|---|
| HTTP API has no authentication | The `/telemetry` and `/recommendations` endpoints are unauthenticated. Anyone with network access to port 8080 can read them. Acceptable for alpha with port-forward access only. Should be addressed before any external exposure. |
| No TLS on the HTTP API | All traffic is plaintext inside the cluster. Acceptable for in-cluster ClusterIP + port-forward. Would need TLS termination before any in-cluster multi-tenant exposure. |
| Node agent has no mutual auth with control plane | Currently a stub. Will need to be addressed when the agent sends real data. |
| No audit log | IFA does not log who called which API endpoint. Adding request logging is low-effort and should be done before production use. |

---

## Security Posture by Deployment Mode

| Control | Standard mode | Air-gapped mode |
|---|---|---|
| Read-only RBAC | Yes | Yes |
| No external API calls | Yes | Yes (enforced by NetworkPolicy) |
| No prompt/response collection | Yes | Yes |
| NetworkPolicy | Not applied | Applied (default-deny-egress) |
| imagePullPolicy | IfNotPresent | Never |
| Image registry | Configurable | Internal registry only |
| Egress config flag | `egress.enabled: true` | `egress.enabled: false` |

---

## Compliance Notes

IFA does not process personal data as defined under GDPR, CCPA, or similar
regulations, because it does not read prompt or response content, user
identifiers, or any user-attributable data. The metrics it collects are
infrastructure counters equivalent to CPU utilisation or request throughput.

This is a design property, not a legal opinion. Operators should conduct their
own review before deploying in regulated environments.
