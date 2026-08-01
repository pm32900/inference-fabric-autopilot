package vllm

// Metric name constants for vLLM's Prometheus /metrics endpoint.
//
// These are the names vLLM emits as of v0.4.x / v0.5.x.
// vLLM uses a colon separator (vllm:metric_name) which is non-standard
// but intentional — it avoids collisions with other exporters.
//
// If your vLLM version emits different names, update the constants here.
// The rest of the package will automatically pick up the changes.
const (
	// MetricNumRequestsRunning is the number of requests currently
	// being actively processed by the vLLM engine (prefill + decode).
	MetricNumRequestsRunning = "vllm:num_requests_running"

	// MetricNumRequestsWaiting is the number of requests queued,
	// waiting for the scheduler to admit them.
	MetricNumRequestsWaiting = "vllm:num_requests_waiting"

	// MetricGPUCacheUsagePerc is the fraction of KV-cache blocks in use,
	// reported as a value in [0, 1]. Multiply by 100 for percentage.
	MetricGPUCacheUsagePerc = "vllm:gpu_cache_usage_perc"

	// MetricE2ELatencyP50, P95, P99 are end-to-end request latency
	// summary quantiles in seconds.
	MetricE2ELatencyP50 = `vllm:e2e_request_latency_seconds{quantile="0.5"}`
	MetricE2ELatencyP95 = `vllm:e2e_request_latency_seconds{quantile="0.95"}`
	MetricE2ELatencyP99 = `vllm:e2e_request_latency_seconds{quantile="0.99"}`

	// MetricTTFTP50, P95, P99 are time-to-first-token summary quantiles
	// in seconds. Only emitted if the runtime tracks prefill latency.
	MetricTTFTP50 = `vllm:time_to_first_token_seconds{quantile="0.5"}`
	MetricTTFTP95 = `vllm:time_to_first_token_seconds{quantile="0.95"}`
	MetricTTFTP99 = `vllm:time_to_first_token_seconds{quantile="0.99"}`

	// MetricGenerationTokensTotal is a counter of tokens generated
	// (decode-phase tokens only). Use rate() in PromQL for throughput.
	// P95 Labs reads the raw counter and derives a delta between scrapes.
	MetricGenerationTokensTotal = "vllm:generation_tokens_total"

	// MetricPromptTokensTotal is a counter of prompt tokens processed.
	MetricPromptTokensTotal = "vllm:prompt_tokens_total"

	// MetricRequestSuccessTotal counts successfully completed requests.
	MetricRequestSuccessTotal = "vllm:request_success_total"

	// MetricRequestFailureTotal counts failed requests.
	// Divide by MetricRequestSuccessTotal for error rate.
	MetricRequestFailureTotal = "vllm:request_failure_total"
)

// VLLMSnapshot holds all vLLM-specific metric values extracted from a single
// Prometheus scrape. Values are normalized (e.g. KVCacheUsagePct is in [0,100],
// latencies are in milliseconds).
//
// Fields are zero-valued for metrics that were absent in the scrape payload.
// Callers should treat zero as "not observed" rather than "zero activity".
type VLLMSnapshot struct {
	// Scheduler state
	NumRequestsRunning int
	NumRequestsWaiting int

	// KV cache — percentage [0, 100]
	KVCacheUsagePct float64

	// End-to-end latency in milliseconds
	E2ELatencyP50Ms float64
	E2ELatencyP95Ms float64
	E2ELatencyP99Ms float64

	// Time to first token in milliseconds
	TTFTP50Ms float64
	TTFTP95Ms float64
	TTFTP99Ms float64

	// Throughput counters (raw; caller computes rate between scrapes)
	GenerationTokensTotal float64
	PromptTokensTotal     float64

	// Request counters (raw)
	RequestSuccessTotal float64
	RequestFailureTotal float64
}
