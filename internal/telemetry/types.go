package telemetry

import "time"

// Snapshot holds a single telemetry reading from one inference workload.
// In Phase 1 this is simulated. In later phases it will come from real node agents.
type Snapshot struct {
	Timestamp         time.Time `json:"timestamp"`
	ClusterName       string    `json:"cluster_name"`
	Namespace         string    `json:"namespace"`
	WorkloadName      string    `json:"workload_name"`
	Runtime           string    `json:"runtime"` // e.g. "vllm", "triton", "ollama"
	ModelName         string    `json:"model_name"`
	RequestRatePerSec float64   `json:"request_rate_per_sec"`
	P50LatencyMs      float64   `json:"p50_latency_ms"`
	P95LatencyMs      float64   `json:"p95_latency_ms"`
	P99LatencyMs      float64   `json:"p99_latency_ms"`
	QueueDepth        int       `json:"queue_depth"`
	GPUUtilizationPct float64   `json:"gpu_utilization_percent"`
	GPUMemoryUsedPct  float64   `json:"gpu_memory_used_percent"`
	TokensPerSecond   float64   `json:"tokens_per_second"`
	ErrorRatePct      float64   `json:"error_rate_percent"`

	// vLLM-native fields — zero for non-vLLM runtimes
	NumRequestsRunning int     `json:"num_requests_running"`
	NumRequestsWaiting int     `json:"num_requests_waiting"`
	KVCacheUsagePct    float64 `json:"kv_cache_usage_pct"`
	TTFTP95Ms          float64 `json:"ttft_p95_ms"`
}

// Recommendation is a single actionable insight produced by the recommender.
type Recommendation struct {
	ID              string `json:"id"`
	Severity        string `json:"severity"` // "info", "warning", "critical"
	WorkloadName    string `json:"workload_name"`
	Title           string `json:"title"`
	Explanation     string `json:"explanation"`
	SuggestedAction string `json:"suggested_action"`
	RelatedMetric   string `json:"related_metric"`
}
