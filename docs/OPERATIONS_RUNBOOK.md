# Operations Runbook — Inference Fabric Autopilot

**Version:** Phase 2.5
**Last updated:** 2026-06

---

## Purpose

Day-2 operations reference. Covers health checks, config changes, log
inspection, common failure modes, and upgrade/rollback procedures.

---

## Quick Reference

```bash
# Health check
curl http://localhost:8080/healthz

# Current telemetry
curl http://localhost:8080/telemetry | jq .

# Current recommendations
curl http://localhost:8080/recommendations | jq .

# Pod status
kubectl get pods -n inference

# Control plane logs
kubectl logs -n inference deploy/autopilot-control-plane

# Node agent logs (all nodes)
kubectl logs -n inference daemonset/autopilot-node-agent
```

---

## Health Checks

### 1. Control plane HTTP health

```bash
kubectl port-forward -n inference svc/autopilot 8080:8080 &
curl -s http://localhost:8080/healthz
```

Expected:
```json
{"status":"ok"}
```

If this returns non-200 or times out, check pod status and logs first.

### 2. Pod readiness

```bash
kubectl get pods -n inference
```

Expected: all pods `1/1 Running`, zero restarts.

If a pod is in `CrashLoopBackOff`:
```bash
kubectl logs -n inference <pod-name> --previous
```

### 3. Telemetry flowing

```bash
curl -s http://localhost:8080/telemetry | jq 'length'
```

- In `simulated` mode: expect 4 workloads returned immediately.
- In `prometheus` mode: expect workloads matching your `prometheus_targets`
  config. If 0 returned, the Prometheus scrape is failing — see
  Troubleshooting section.

### 4. Recommendations generating

```bash
curl -s http://localhost:8080/recommendations | jq 'length'
```

Returns 0 if all workloads are within thresholds. That is a valid result.
Returns recommendations if any threshold is breached.

---

## Accessing the API

### Via port-forward (standard operator access)

```bash
kubectl port-forward -n inference svc/autopilot 8080:8080
# Keep this running in a separate terminal, then:
curl http://localhost:8080/telemetry
```

### Via CLI

```bash
./ifa telemetry
./ifa recommendations
./ifa workloads
```

The CLI assumes port-forward is active on `localhost:8080` by default.

### In-cluster (from another pod)

```bash
curl http://autopilot.inference.svc.cluster.local:8080/healthz
```

---

## Config Changes

### Editing config in Kubernetes (Helm-managed)

1. Edit your local `values.yaml` or `values-airgapped.yaml`.
2. Upgrade the release:

```bash
helm upgrade autopilot deploy/helm/autopilot \
  --namespace inference \
  --values deploy/helm/autopilot/values.yaml \
  --wait
```

3. The ConfigMap is updated and the control-plane pod restarts automatically.

### Editing config directly (kubectl, non-Helm)

```bash
kubectl edit configmap autopilot-config -n inference
# Save and exit — pod does NOT auto-restart on ConfigMap change.
# Restart manually:
kubectl rollout restart deployment/autopilot-control-plane -n inference
```

### Key config fields

| Field | Location | Effect |
|---|---|---|
| `collector.mode` | `config.yaml` | `simulated` or `prometheus` |
| `collector.interval_seconds` | `config.yaml` | How often metrics are scraped |
| `collector.prometheus_targets` | `config.yaml` | List of workloads to scrape |
| `kubernetes.enabled` | `config.yaml` | Enable/disable K8s API watch |
| `database.enabled` | `config.yaml` | Enable/disable TimescaleDB persistence |
| `logging.level` | `config.yaml` | `debug`, `info`, `warn`, `error` |
| `deploymentMode` | `config.yaml` | `standard` or `airgapped` |
| `egress.enabled` | `config.yaml` | `false` in air-gapped mode |

---

## Log Inspection

### Control plane

```bash
# Live logs
kubectl logs -n inference deploy/autopilot-control-plane -f

# Last 100 lines
kubectl logs -n inference deploy/autopilot-control-plane --tail=100

# Previous crashed container
kubectl logs -n inference deploy/autopilot-control-plane --previous
```

Log levels are set via `logging.level` in config. Use `debug` to see
per-scrape metric collection and recommender rule evaluations.

### Node agent

```bash
kubectl logs -n inference daemonset/autopilot-node-agent -f
```

Currently the stub logs only startup and node identity.

### Structured log format

Set `logging.format: json` for log aggregation pipelines:

```bash
kubectl logs -n inference deploy/autopilot-control-plane | jq .
```

---

## Troubleshooting

### Pod stuck in `Pending`

```bash
kubectl describe pod -n inference <pod-name>
```

Common causes:
- **Insufficient resources:** Check node capacity with `kubectl describe nodes`.
- **Image pull failure (air-gapped):** Ensure image was loaded before deploy.
  Check `imagePullPolicy: Never` is set in `values-airgapped.yaml`.
- **PVC unbound (TimescaleDB):** Check PersistentVolumeClaim status.

### Pod in `CrashLoopBackOff`

```bash
kubectl logs -n inference <pod-name> --previous
```

Common causes:
- Config parse error — malformed `config.yaml`. Check YAML syntax.
- TimescaleDB unreachable but `database.enabled: true` — either fix DSN
  or set `database.enabled: false`.
- Port conflict — check `server.port` matches the container port in the
  Deployment spec.

### No telemetry returned (`/telemetry` returns empty array)

Check `collector.mode`:
- If `simulated`: should always return 4 workloads. If not, check logs for
  startup errors.
- If `prometheus`: check each target in `collector.prometheus_targets`.

Test a Prometheus target manually:
```bash
kubectl exec -n inference deploy/autopilot-control-plane -- \
  wget -qO- http://<prometheus-target-url>/metrics | head -20
```

If connection refused: Prometheus is not reachable at that URL.
If timeout: NetworkPolicy may be blocking the scrape — check egress rules.

### No recommendations returned

This is expected if all workload metrics are within configured thresholds.
Lower a threshold to verify the recommender is running:

```bash
# Temporarily lower GPU util threshold to trigger a recommendation
# Edit config, set low_gpu_util_pct: 99.0, then:
kubectl rollout restart deployment/autopilot-control-plane -n inference
curl http://localhost:8080/recommendations | jq .
```

Restore the original threshold after testing.

### NetworkPolicy blocking Prometheus scrape (air-gapped)

Symptom: `/telemetry` returns empty, logs show connection refused to Prometheus.

Check the NetworkPolicy allows egress to the Prometheus namespace and port:

```bash
kubectl describe networkpolicy -n inference
```

If missing, verify `networkPolicy.enabled: true` in `values-airgapped.yaml`
and re-run `helm upgrade`.

If CNI does not support NetworkPolicy, the policy is silently ignored. Verify
CNI compatibility in [AIRGAPPED_DEPLOYMENT.md](./AIRGAPPED_DEPLOYMENT.md).

### `ifa` CLI returns connection refused

Ensure port-forward is running:
```bash
kubectl port-forward -n inference svc/autopilot 8080:8080
```

Check the service exists:
```bash
kubectl get svc -n inference
```

---

## Scaling

The control plane is a single-replica Deployment. It is stateful when using
the in-memory telemetry store — running multiple replicas would cause each to
hold an independent state and the CLI would hit different replicas per request.

**Do not scale to >1 replica unless TimescaleDB is enabled** and the in-memory
store is replaced with a shared read path.

Node agent scales automatically — it is a DaemonSet and adds a pod per node
as nodes join the cluster.

---

## Backup and Recovery

### In-memory mode

No backup needed. Data is ephemeral and regenerated on the next collection
cycle after restart.

### TimescaleDB mode

Back up the `autopilot` database using standard PostgreSQL tooling:

```bash
pg_dump -h <timescaledb-host> -U postgres autopilot > autopilot-backup.sql
```

Restore:
```bash
psql -h <timescaledb-host> -U postgres autopilot < autopilot-backup.sql
```

---

## Upgrade Procedure

1. Update image tags in `values.yaml` (or `values-airgapped.yaml`).
2. In air-gapped mode, load new images first (see Steps 1–3 in
   [AIRGAPPED_DEPLOYMENT.md](./AIRGAPPED_DEPLOYMENT.md)).
3. Run Helm upgrade:

```bash
helm upgrade autopilot deploy/helm/autopilot \
  --namespace inference \
  --values deploy/helm/autopilot/values.yaml \
  --wait --timeout 120s
```

4. Verify:

```bash
kubectl get pods -n inference
curl http://localhost:8080/healthz
go test ./...   # if running from repo
```

---

## Rollback Procedure

```bash
# Rollback to previous Helm revision
helm rollback autopilot --namespace inference

# Check rollback succeeded
helm history autopilot -n inference
kubectl get pods -n inference
```

---

## Uninstall

```bash
helm uninstall autopilot --namespace inference

# If namespace should also be removed:
kubectl delete namespace inference
```

TimescaleDB data persists in its PersistentVolume after uninstall.
Delete the PVC explicitly if a clean removal is needed:

```bash
kubectl delete pvc -n inference --all
```

---

## Monitoring IFA Itself

IFA does not yet expose its own Prometheus metrics endpoint. To monitor it:

- Watch pod restart count: `kubectl get pods -n inference`
- Watch logs for `ERROR` level entries
- Set up an external health check against `/healthz` using your existing
  monitoring stack (Alertmanager, Datadog, PagerDuty, etc.)

A self-metrics endpoint (`/metrics` for the control plane itself) is on the
roadmap for Phase 3.
