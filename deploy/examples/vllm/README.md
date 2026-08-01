# vLLM Example — Local Kubernetes Validation

Minimal manifests to run a vLLM pod in Kubernetes and validate IFA's
Prometheus collector and recommendation rules against real vLLM metrics.

---

## Prerequisites

- A running Kubernetes cluster (kind, minikube, Docker Desktop, or real)
- `kubectl` configured and pointing at that cluster
- Internet access from cluster nodes (pulls `vllm/vllm-openai:latest` from
  Docker Hub and `facebook/opt-125m` from HuggingFace on first start)
- ~1 GB free memory on the node (opt-125m CPU path)

> **GPU note:** For GPU clusters, uncomment the `resources` block in
> `deploy/examples/vllm/deployment.yaml` and change `--dtype float32` to `--dtype auto`.
> Ensure `nvidia-device-plugin` is installed on the cluster.

---

## 1. Create the namespace (if it doesn't exist)

```bash
kubectl create namespace inference --dry-run=client -o yaml | kubectl apply -f -
```

---

## 2. Apply the manifests

```bash
kubectl apply -f deploy/examples/vllm/deployment.yaml
kubectl apply -f deploy/examples/vllm/service.yaml
```

---

## 3. Wait for the pod to become ready

Model download + load takes 1–3 minutes on first run.

```bash
kubectl -n inference rollout status deployment/vllm-example
```

Check pod events if it takes longer:

```bash
kubectl -n inference describe pod -l app=vllm-example
```

---

## 4. Port-forward the service

```bash
kubectl -n inference port-forward svc/vllm-example 8000:8000
```

Leave this running in a separate terminal.

---

## 5. Curl the metrics endpoint

```bash
curl -s http://localhost:8000/metrics | head -60
```

---

## 6. Confirm expected vLLM metric names are present

```bash
curl -s http://localhost:8000/metrics | grep -E \
  'vllm:num_requests_running|vllm:num_requests_waiting|vllm:gpu_cache_usage_perc|vllm:time_to_first_token_seconds'
```

Expected output (values will vary):

```
vllm:num_requests_running 0
vllm:num_requests_waiting 0
vllm:gpu_cache_usage_perc 0.0
vllm:time_to_first_token_seconds{quantile="0.5"} NaN
vllm:time_to_first_token_seconds{quantile="0.95"} NaN
vllm:time_to_first_token_seconds{quantile="0.99"} NaN
```

`NaN` on TTFT quantiles is normal when no requests have been served yet.
Send a request first to populate them:

```bash
curl -s http://localhost:8000/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"facebook/opt-125m","prompt":"Hello","max_tokens":5}'
```

Then re-run the metrics grep — quantile values should now be floats.

---

## 7. Wire IFA's Prometheus collector to this deployment

Add the following target to your `config.yaml` under
`collector.prometheus_targets`:

```yaml
collector:
  mode: prometheus
  interval_seconds: 15
  prometheus_targets:
    - workload_name: vllm-example
      namespace: inference
      runtime: vllm
      model_name: facebook/opt-125m
      metrics_url: http://vllm-example.inference.svc.cluster.local:8000/metrics
```

When IFA runs inside the same cluster it will reach the service by DNS.
For local development outside the cluster, use the port-forward address:

```yaml
metrics_url: http://localhost:8000/metrics
```

IFA's Prometheus collector calls `vllm.Parse()` (`internal/runtime/vllm/parser.go`)
to extract all vLLM metrics and populate `NumRequestsRunning`, `NumRequestsWaiting`,
`KVCacheUsagePct`, and `TTFTP95Ms` from each scrape.
Rules 9, 10, and 11 in the recommender will fire when thresholds are crossed.

---

## 8. Clean up

```bash
kubectl delete -f deploy/examples/vllm/service.yaml
kubectl delete -f deploy/examples/vllm/deployment.yaml
```

The `inference` namespace itself is shared — do not delete it here unless
you are sure no other workloads are using it.