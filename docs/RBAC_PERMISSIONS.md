# RBAC Permissions — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06

---

## Summary

IFA requires one `ClusterRole` with read-only verbs across a small set of
Kubernetes resources. It requests no write permissions, no escalation, and no
access to sensitive resources such as Secrets, ConfigMaps, or cluster-admin
equivalents.

---

## ClusterRole Definition

This is the exact role applied by both the raw manifest
(`deploy/kubernetes/clusterrole.yaml`) and the Helm chart
(`deploy/helm/autopilot/templates/rbac.yaml`).

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: inference-autopilot-reader
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "replicasets"]
    verbs: ["get", "list", "watch"]
```

---

## Permission Breakdown — Annotated

### Core API group (`""`)

| Resource | Verbs | Why needed | Risk |
|---|---|---|---|
| `pods` | `get`, `list`, `watch` | Identify running inference workloads by label; track pod-to-node mapping | Read-only. Pod spec and status only. No exec, no log access. |
| `nodes` | `get`, `list`, `watch` | Node health context for recommendations (capacity, conditions) | Read-only. No node-level exec or privileged access. |
| `namespaces` | `get`, `list`, `watch` | Enumerate namespaces when watching cross-namespace workloads | Read-only. Namespace metadata only. |

### Apps API group (`apps`)

| Resource | Verbs | Why needed | Risk |
|---|---|---|---|
| `deployments` | `get`, `list`, `watch` | Replica count for RPS-per-replica recommendation rule | Read-only. No rollout, scale, or patch operations. |
| `daemonsets` | `get`, `list`, `watch` | Track node-agent DaemonSet health | Read-only. |
| `replicasets` | `get`, `list`, `watch` | Resolve deployment → replicaset → pod ownership chain | Read-only. |

---

## Permissions Explicitly Not Requested

The following permissions are **not present** in any IFA role and are never
needed:

| Category | Examples | Status |
|---|---|---|
| Write verbs | `create`, `update`, `patch`, `delete`, `deletecollection` | Not granted |
| Privileged resources | `secrets`, `configmaps`, `serviceaccounts` | Not granted |
| Auth/authz resources | `clusterroles`, `rolebindings`, `tokenreviews` | Not granted |
| Workload mutation | `scale`, `rollout`, `exec`, `attach`, `portforward` | Not granted |
| Admission control | `mutatingwebhookconfigurations`, `validatingwebhookconfigurations` | Not granted |
| Cluster-level control | `nodes/proxy`, `nodes/stats`, `nodes/log` | Not granted |
| Pod logs | `pods/log` | Not granted |
| Pod exec | `pods/exec` | Not granted |
| Custom resources | Any CRD | Not granted |

---

## ClusterRoleBinding

IFA uses a single `ClusterRoleBinding` that binds the role above to the
`ifa` `ServiceAccount` in the `inference` namespace.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: inference-autopilot-reader
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: inference-autopilot-reader
subjects:
  - kind: ServiceAccount
    name: ifa
    namespace: inference
```

The binding is cluster-scoped so IFA can watch resources across namespaces.
This is necessary for multi-namespace inference fleet visibility. The
permissions themselves remain read-only regardless of scope.

---

## ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ifa
  namespace: inference
```

- No `imagePullSecrets` attached by default.
- No `automountServiceAccountToken: true` override — uses cluster default.
- In hardened environments, set `automountServiceAccountToken: false` on
  pods that do not need Kubernetes API access (e.g. if node-agent is
  separated from control-plane SA in future).

---

## Namespace Scope Considerations

The `ClusterRoleBinding` grants read access across all namespaces. If your
security policy requires namespace-scoped access only, you can replace the
`ClusterRole` + `ClusterRoleBinding` with a `Role` + `RoleBinding` scoped
to the `inference` namespace. The trade-off is that IFA will only see
workloads in that single namespace.

To apply namespace-scoped RBAC instead:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: inference-autopilot-reader
  namespace: inference
rules:
  - apiGroups: [""]
    resources: ["pods", "nodes", "namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["apps"]
    resources: ["deployments", "daemonsets", "replicasets"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: inference-autopilot-reader
  namespace: inference
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: inference-autopilot-reader
subjects:
  - kind: ServiceAccount
    name: ifa
    namespace: inference
```

Note: `nodes` is a cluster-scoped resource. A namespace-scoped `Role` cannot
grant access to `nodes`. Remove it from the rule if you switch to `Role`,
and accept that node-level context will not be available.

---

## Verifying Permissions at Runtime

To confirm what the IFA service account can actually do in a running cluster:

```bash
# Check if IFA SA can list pods
kubectl auth can-i list pods \
  --as=system:serviceaccount:inference:ifa \
  -n inference
# Expected: yes

# Check it cannot create pods
kubectl auth can-i create pods \
  --as=system:serviceaccount:inference:ifa \
  -n inference
# Expected: no

# Check it cannot read secrets
kubectl auth can-i get secrets \
  --as=system:serviceaccount:inference:ifa \
  -n inference
# Expected: no

# List all permissions for the IFA SA
kubectl auth can-i --list \
  --as=system:serviceaccount:inference:ifa \
  -n inference
```

---

## rbac.mode Config Field

As of Phase 2.5, `config.yaml` exposes an `rbac.mode` field:

```yaml
rbac:
  mode: read-only   # only valid value currently; documents posture explicitly
```

This field is informational in Phase 2.5 — it documents the declared RBAC
posture and is validated at startup. If a future phase introduces an optional
narrower role (namespace-only), this field will gate that behaviour.
