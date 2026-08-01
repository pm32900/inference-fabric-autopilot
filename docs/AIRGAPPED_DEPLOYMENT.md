# Air-Gapped Deployment — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06

---

## Overview

This guide covers deploying IFA in a network-restricted or fully air-gapped
Kubernetes environment — one where cluster nodes have no outbound internet
access and all container images must be sourced from an internal registry or
pre-loaded onto nodes directly.

IFA was designed from the start to work without external dependencies at
runtime. Air-gapped deployment formalises this into a supported, documented
operational path.

---

## Prerequisites

On the **operator workstation** (internet-connected, used once to build the bundle):

- Docker (to pull and save images)
- Helm 3
- `sha256sum` or `shasum`
- `tar`

In the **target cluster** (air-gapped):

- Kubernetes 1.26+
- A CNI that supports `NetworkPolicy` (Calico, Cilium, Antrea — not Flannel
  without a NetworkPolicy controller)
- Either: an internal container registry reachable from cluster nodes, OR
  a way to `docker load` / `ctr images import` on each node directly
- `kubectl` access with permission to create namespaces, deploy workloads,
  and apply RBAC

---

## Step 1 — Build the Air-Gap Bundle (internet-connected machine)

Run the export script from the repo root:

```bash
chmod +x scripts/export-airgap-bundle.sh
./scripts/export-airgap-bundle.sh
```

This script:
1. Builds `ifa-control-plane:dev` and `ifa-node-agent:dev` Docker images
2. Saves them as `.tar` files
3. Packages the Helm chart as a `.tgz`
4. Copies the `docs/` folder
5. Generates `checksums.sha256` for all artifacts
6. Produces `airgap-bundle.tar.gz` in the current directory

Expected output:
```
airgap-bundle/
  ifa-control-plane.tar
  ifa-node-agent.tar
  autopilot-0.1.0.tgz
  values-airgapped.yaml
  checksums.sha256
  docs/
airgap-bundle.tar.gz
```

Transfer `airgap-bundle.tar.gz` to the air-gapped environment using your
approved out-of-band transfer method (USB, bastion copy, artifact repository).

---

## Step 2 — Verify Checksums

On the air-gapped machine, after transfer:

```bash
tar -xzf airgap-bundle.tar.gz
cd airgap-bundle
sha256sum -c checksums.sha256
```

Expected output — all lines must show `OK`:
```
ifa-control-plane.tar: OK
ifa-node-agent.tar: OK
autopilot-0.1.0.tgz: OK
values-airgapped.yaml: OK
```

Do not proceed if any checksum fails.

---

## Step 3 — Load Images

### Option A — Internal registry (recommended for multi-node clusters)

```bash
# Load images into local Docker daemon
docker load -i ifa-control-plane.tar
docker load -i ifa-node-agent.tar

# Tag for your internal registry
docker tag ifa-control-plane:dev registry.internal.example.com/ifa/control-plane:2.5.0
docker tag ifa-node-agent:dev     registry.internal.example.com/ifa/node-agent:2.5.0

# Push to internal registry
docker push registry.internal.example.com/ifa/control-plane:2.5.0
docker push registry.internal.example.com/ifa/node-agent:2.5.0
```

Then update `values-airgapped.yaml`:

```yaml
image:
  registry: registry.internal.example.com
controlPlane:
  image:
    repository: ifa/control-plane
    tag: "2.5.0"
    pullPolicy: IfNotPresent
nodeAgent:
  image:
    repository: ifa/node-agent
    tag: "2.5.0"
    pullPolicy: IfNotPresent
```

### Option B — Direct node load (single-node or kind clusters)

```bash
# For Docker-based nodes
docker load -i ifa-control-plane.tar
docker load -i ifa-node-agent.tar

# For containerd-based nodes (e.g. kubeadm clusters)
ctr -n k8s.io images import ifa-control-plane.tar
ctr -n k8s.io images import ifa-node-agent.tar
```

Set `imagePullPolicy: Never` in `values-airgapped.yaml` when using this option.

---

## Step 4 — Create Namespace and Apply RBAC

```bash
kubectl create namespace inference

kubectl apply -f - <<EOF
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ifa
  namespace: inference
EOF
```

RBAC is applied by the Helm chart in the next step.

---

## Step 5 — Deploy with Helm

```bash
# Install the chart using the air-gapped values override
helm install autopilot ./autopilot-0.1.0.tgz \
  --namespace inference \
  --values values-airgapped.yaml \
  --wait \
  --timeout 120s
```

Verify the rollout:

```bash
kubectl get pods -n inference
# Expected:
# NAME                                      READY   STATUS    RESTARTS
# autopilot-control-plane-xxxxxxx-xxxxx     1/1     Running   0
# autopilot-node-agent-xxxxx (per node)     1/1     Running   0
```

---

## Step 6 — Verify NetworkPolicy is Applied

```bash
kubectl get networkpolicy -n inference
# Expected: autopilot-egress-deny (or similar name from the Helm chart)

kubectl describe networkpolicy -n inference
# Review: default deny egress, explicit allow for K8s API and Prometheus
```

To confirm egress is blocked, from inside a pod in the `inference` namespace:

```bash
kubectl exec -n inference deploy/autopilot-control-plane -- \
  wget -q --timeout=5 https://google.com -O /dev/null
# Expected: timeout or connection refused — confirms egress deny is working
```

---

## Step 7 — Verify IFA is Working

```bash
# Port-forward the control plane
kubectl port-forward -n inference svc/autopilot 8080:8080 &

# Health check
curl http://localhost:8080/healthz
# Expected: {"status":"ok"}

# Telemetry (simulated mode by default)
curl http://localhost:8080/telemetry | jq .

# Recommendations
curl http://localhost:8080/recommendations | jq .
```

---

## values-airgapped.yaml Reference

The full reference file is at
`deploy/helm/autopilot/values-airgapped.yaml`.

Key fields that differ from the standard `values.yaml`:

```yaml
deploymentMode: airgapped

image:
  registry: ""                  # set to your internal registry hostname

controlPlane:
  image:
    pullPolicy: Never           # no pulls; images must already be on node or in registry

nodeAgent:
  image:
    pullPolicy: Never

egress:
  enabled: false                # signals intent; NetworkPolicy enforces it

privacy:
  collect_prompt_bodies: false
  collect_response_bodies: false
  collect_headers: false
  collect_user_identifiers: false

rbac:
  mode: read-only

networkPolicy:
  enabled: true                 # deploys the default-deny-egress NetworkPolicy

prometheus:
  url: http://prometheus.monitoring.svc.cluster.local:9090
```

---

## NetworkPolicy Details

The Helm chart deploys a `NetworkPolicy` when `networkPolicy.enabled: true`:

```yaml
# Default deny all egress from the inference namespace
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: autopilot-egress-deny
  namespace: inference
spec:
  podSelector: {}
  policyTypes:
    - Egress
  egress:
    # Allow DNS (required for in-cluster service resolution)
    - ports:
        - port: 53
          protocol: UDP
        - port: 53
          protocol: TCP
    # Allow Kubernetes API server
    - ports:
        - port: 443
          protocol: TCP
        - port: 6443
          protocol: TCP
    # Allow Prometheus (in-cluster)
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
      ports:
        - port: 9090
          protocol: TCP
```

### CNI Compatibility

`NetworkPolicy` enforcement requires a CNI that supports it:

| CNI | NetworkPolicy support |
|---|---|
| Calico | Yes |
| Cilium | Yes |
| Antrea | Yes |
| Flannel | No (requires additional NetworkPolicy controller) |
| Canal | Yes (Calico policy engine + Flannel routing) |

If your CNI does not support `NetworkPolicy`, the policy will be accepted by
the API server but silently not enforced. Verify enforcement with the egress
test in Step 6.

---

## Kubernetes API Server Egress Rule Caveat

The Kubernetes API server IP varies by cluster setup:

- `kind`: typically `172.18.0.1` or similar Docker bridge IP
- kubeadm: control plane node IP
- Managed clusters (EKS, GKE, AKS): varies; often not a pod-reachable IP

The NetworkPolicy allows port 443 and 6443 generically. If your cluster requires
a specific IP allowlist for the API server, add an `ipBlock` rule:

```yaml
egress:
  - to:
      - ipBlock:
          cidr: <API_SERVER_IP>/32
    ports:
      - port: 6443
        protocol: TCP
```

Run `kubectl cluster-info` to find the API server address.

---

## Upgrading in Air-Gapped Mode

1. Build a new bundle with the updated images on an internet-connected machine
2. Transfer, verify checksums, and load images as in Steps 1–3
3. Upgrade the Helm release:

```bash
helm upgrade autopilot ./autopilot-NEW_VERSION.tgz \
  --namespace inference \
  --values values-airgapped.yaml \
  --wait
```

4. Verify pods restarted and are running the new image:

```bash
kubectl get pods -n inference -o jsonpath='{.items[*].spec.containers[*].image}'
```

---

## Rollback

```bash
helm rollback autopilot --namespace inference
kubectl get pods -n inference
```

---

## Uninstall

```bash
helm uninstall autopilot --namespace inference
kubectl delete namespace inference
```

This removes all IFA resources. TimescaleDB data (if used) persists in its
PersistentVolume until that is also deleted.
