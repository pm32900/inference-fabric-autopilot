package recommender

import (
	"fmt"

	"github.com/pm32900/inference-fabric-autopilot/internal/config"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

// Analyze runs all rules against the latest snapshots and returns recommendations.
// It accepts thresholds from config so they can be tuned without recompiling.
func Analyze(snapshots []telemetry.Snapshot, cfg config.ThresholdConfig) []telemetry.Recommendation {
	var recs []telemetry.Recommendation
	counter := 1

	for _, snap := range snapshots {
		recs = append(recs, runRules(snap, cfg, &counter)...)
	}

	if recs == nil {
		return []telemetry.Recommendation{}
	}
	return recs
}

// runRules evaluates every rule against a single snapshot.
// Each rule is independent — a workload can trigger multiple recommendations.
func runRules(snap telemetry.Snapshot, cfg config.ThresholdConfig, counter *int) []telemetry.Recommendation {
	var recs []telemetry.Recommendation

	add := func(r telemetry.Recommendation) {
		r.ID = fmt.Sprintf("rec-%03d", *counter)
		*counter++
		recs = append(recs, r)
	}

	// ── Rule 1: Low GPU util + high queue depth ───────────────────────────────
	// GPU is idle while requests pile up → batching or concurrency misconfiguration
	if snap.GPUUtilizationPct < cfg.LowGPUUtilPct && snap.QueueDepth > cfg.HighQueueDepth {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "Low GPU utilization with high queue depth",
			Explanation:     fmt.Sprintf("GPU utilization is %.1f%% while queue depth is %d. The GPU is idle but requests are piling up.", snap.GPUUtilizationPct, snap.QueueDepth),
			SuggestedAction: "Increase max_num_seqs or concurrency slots in your inference runtime. Check if batching is disabled.",
			RelatedMetric:   "gpu_utilization_percent, queue_depth",
		})
	}

	// ── Rule 2: High p95 latency ──────────────────────────────────────────────
	if snap.P95LatencyMs > cfg.HighP95LatencyMs {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "High p95 latency",
			Explanation:     fmt.Sprintf("p95 latency is %.1fms, exceeding the %.0fms threshold.", snap.P95LatencyMs, cfg.HighP95LatencyMs),
			SuggestedAction: "Consider scaling replicas, reducing max batch delay, or profiling for slow requests.",
			RelatedMetric:   "p95_latency_ms",
		})
	}

	// ── Rule 3: High GPU memory pressure ─────────────────────────────────────
	if snap.GPUMemoryUsedPct > cfg.HighGPUMemPct {
		add(telemetry.Recommendation{
			Severity:        "critical",
			WorkloadName:    snap.WorkloadName,
			Title:           "High GPU memory pressure",
			Explanation:     fmt.Sprintf("GPU memory usage is %.1f%%, approaching capacity.", snap.GPUMemoryUsedPct),
			SuggestedAction: "Reduce max sequence length, enable KV cache eviction, or move to a larger GPU.",
			RelatedMetric:   "gpu_memory_used_percent",
		})
	}

	// ── Rule 4: High error rate ───────────────────────────────────────────────
	if snap.ErrorRatePct > cfg.HighErrorRatePct {
		add(telemetry.Recommendation{
			Severity:        "critical",
			WorkloadName:    snap.WorkloadName,
			Title:           "Elevated error rate",
			Explanation:     fmt.Sprintf("Error rate is %.2f%%, exceeding the %.1f%% threshold.", snap.ErrorRatePct, cfg.HighErrorRatePct),
			SuggestedAction: "Check runtime logs for OOM kills, malformed requests, or model loading failures.",
			RelatedMetric:   "error_rate_percent",
		})
	}

	// ── Rule 5: Replica count too low for current RPS ─────────────────────────
	// If RPS per replica exceeds the configured threshold, we likely need more replicas.
	// We don't have replica count here yet (that comes from k8s watcher in a later phase),
	// so we use RPS alone as a proxy: very high RPS + high p99 latency = scale signal.
	if snap.RequestRatePerSec > cfg.MinReplicasForRPS && snap.P99LatencyMs > cfg.HighP95LatencyMs*1.5 {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "Replica count may be too low for current traffic",
			Explanation:     fmt.Sprintf("Request rate is %.1f req/s and p99 latency is %.1fms. Traffic may be outpacing available replicas.", snap.RequestRatePerSec, snap.P99LatencyMs),
			SuggestedAction: "Consider increasing replica count or enabling the Horizontal Pod Autoscaler with a custom latency metric.",
			RelatedMetric:   "request_rate_per_sec, p99_latency_ms",
		})
	}

	// ── Rule 6: High queue + high GPU util → GPU is saturated ────────────────
	// Different from Rule 1: here the GPU IS busy but still can't keep up.
	// This means the model is too large or batch size is too small.
	if snap.QueueDepth > cfg.HighQueueDepth && snap.GPUUtilizationPct > 85.0 {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "GPU saturated with requests queuing",
			Explanation:     fmt.Sprintf("GPU utilization is %.1f%% and queue depth is %d. The GPU is at capacity.", snap.GPUUtilizationPct, snap.QueueDepth),
			SuggestedAction: "Consider adding a replica, using a smaller/quantized model, or reducing max batch size to lower latency.",
			RelatedMetric:   "gpu_utilization_percent, queue_depth",
		})
	}

	// ── Rule 7: Low token throughput relative to GPU util ────────────────────
	// High GPU use but low tokens/sec → model may be inefficiently loaded or
	// spending most time on prefill (long-context requests).
	if snap.GPUUtilizationPct > 70.0 && snap.TokensPerSecond < 200.0 {
		add(telemetry.Recommendation{
			Severity:        "info",
			WorkloadName:    snap.WorkloadName,
			Title:           "Low token throughput despite high GPU utilization",
			Explanation:     fmt.Sprintf("GPU utilization is %.1f%% but tokens/sec is only %.1f. The GPU may be spending time on prefill or context processing.", snap.GPUUtilizationPct, snap.TokensPerSecond),
			SuggestedAction: "Check for long-context requests dominating the batch. Consider chunked prefill or context caching if your runtime supports it.",
			RelatedMetric:   "gpu_utilization_percent, tokens_per_second",
		})
	}

	// ── Rule 8: p99 latency is much worse than p95 ───────────────────────────
	// A large p99/p95 ratio indicates a small number of very slow outlier requests
	// which often means long-context requests are being mixed with short ones.
	if snap.P95LatencyMs > 0 && snap.P99LatencyMs/snap.P95LatencyMs > 3.0 {
		add(telemetry.Recommendation{
			Severity:        "info",
			WorkloadName:    snap.WorkloadName,
			Title:           "Large p99/p95 latency gap — outlier requests detected",
			Explanation:     fmt.Sprintf("p99 latency (%.1fms) is %.1fx higher than p95 (%.1fms). A small number of requests are very slow.", snap.P99LatencyMs, snap.P99LatencyMs/snap.P95LatencyMs, snap.P95LatencyMs),
			SuggestedAction: "Consider separating long-context requests into a dedicated queue or deployment to avoid head-of-line blocking.",
			RelatedMetric:   "p99_latency_ms, p95_latency_ms",
		})
	}

	// ── Rule 9 (vLLM): Queue pressure — waiting requests sustained ───────────
	if snap.Runtime == "vllm" && snap.NumRequestsWaiting > cfg.HighQueueDepth {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "vLLM queue pressure — waiting requests sustained",
			Explanation:     fmt.Sprintf("%d requests are waiting in the vLLM scheduler queue (threshold %d). New requests will see elevated TTFT.", snap.NumRequestsWaiting, cfg.HighQueueDepth),
			SuggestedAction: "Increase max_num_seqs, add a replica, or enable chunked prefill to drain the queue faster.",
			RelatedMetric:   "vllm:num_requests_waiting",
		})
	}

	// ── Rule 10 (vLLM): KV cache pressure ────────────────────────────────────
	if snap.Runtime == "vllm" && snap.KVCacheUsagePct > cfg.HighKVCacheUsagePct {
		add(telemetry.Recommendation{
			Severity:        "critical",
			WorkloadName:    snap.WorkloadName,
			Title:           "vLLM KV cache near capacity",
			Explanation:     fmt.Sprintf("KV cache usage is %.1f%% (threshold %.0f%%). vLLM will begin evicting blocks, causing recomputation and latency spikes.", snap.KVCacheUsagePct, cfg.HighKVCacheUsagePct),
			SuggestedAction: "Reduce max_model_len or max_num_seqs, enable prefix caching, or use a GPU with more HBM.",
			RelatedMetric:   "vllm:gpu_cache_usage_perc",
		})
	}

	// ── Rule 11 (vLLM): TTFT degradation ─────────────────────────────────────
	if snap.Runtime == "vllm" && snap.TTFTP95Ms > cfg.HighTTFTP95Ms {
		add(telemetry.Recommendation{
			Severity:        "warning",
			WorkloadName:    snap.WorkloadName,
			Title:           "vLLM p95 time-to-first-token too high",
			Explanation:     fmt.Sprintf("p95 TTFT is %.0fms (threshold %.0fms). Users are waiting too long for the first token, likely due to long prefill or queue depth.", snap.TTFTP95Ms, cfg.HighTTFTP95Ms),
			SuggestedAction: "Enable chunked prefill, reduce concurrent long-context requests, or scale out replicas.",
			RelatedMetric:   "vllm:time_to_first_token_seconds",
		})
	}

	return recs
}
