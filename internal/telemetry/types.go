package telemetry

import "time"

// Runtime identifies the inference server a snapshot came from.
type Runtime string

const (
	RuntimeVLLM   Runtime = "vllm"
	RuntimeTriton Runtime = "triton"
)

// Snapshot is one normalised reading from one inference workload.
//
// Every measurement is a Metric rather than a float64 so that a runtime which
// does not expose a signal is distinguishable from one reporting zero. Fields
// are grouped by what they describe rather than by which runtime produces them:
// the recommender reasons about queues, cache pressure and latency, and the
// runtime adapters are responsible for mapping their own vocabulary onto these.
type Snapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	ClusterName  string    `json:"cluster_name"`
	Namespace    string    `json:"namespace"`
	WorkloadName string    `json:"workload_name"`
	Runtime      Runtime   `json:"runtime"`
	ModelName    string    `json:"model_name"`

	// Throughput. Rates are per second, derived from counter deltas between
	// consecutive scrapes, so the first scrape of a target leaves them unset.
	RequestRatePerSec  Metric `json:"request_rate_per_sec"`
	TokensPerSecond    Metric `json:"tokens_per_second"`
	PromptTokensPerSec Metric `json:"prompt_tokens_per_sec"`

	// Latency, in milliseconds. Percentiles from histogram-backed runtimes are
	// interpolated from bucket boundaries and inherit their resolution.
	P50LatencyMs Metric `json:"p50_latency_ms"`
	P95LatencyMs Metric `json:"p95_latency_ms"`
	P99LatencyMs Metric `json:"p99_latency_ms"`
	TTFTP50Ms    Metric `json:"ttft_p50_ms"`
	TTFTP95Ms    Metric `json:"ttft_p95_ms"`
	TTFTP99Ms    Metric `json:"ttft_p99_ms"`
	// QueueTimeP95Ms is how long a request waits before the scheduler admits
	// it. Separating it from TTFT is what distinguishes "we are out of
	// capacity" from "prefill is slow for this request shape".
	QueueTimeP95Ms Metric `json:"queue_time_p95_ms"`

	// Scheduler state.
	RequestsRunning Metric `json:"requests_running"`
	RequestsWaiting Metric `json:"requests_waiting"`
	// WaitingForCapacity and WaitingDeferred split the waiting queue by cause
	// where the runtime reports it. Requests waiting on capacity mean the
	// workload is genuinely saturated; deferred requests are blocked on a
	// transient constraint such as a LoRA-adapter budget and adding replicas
	// will not help.
	WaitingForCapacity Metric `json:"waiting_for_capacity"`
	WaitingDeferred    Metric `json:"waiting_deferred"`

	// Memory and cache.
	GPUUtilizationPct Metric `json:"gpu_utilization_percent"`
	GPUMemoryUsedPct  Metric `json:"gpu_memory_used_percent"`
	KVCacheUsagePct   Metric `json:"kv_cache_usage_percent"`
	// PreemptionsPerSec is the rate at which the runtime evicts in-flight
	// requests to reclaim KV-cache blocks. Unlike a cache-utilisation
	// threshold it is a symptom rather than a proxy: a workload can sit at 95%
	// KV utilisation indefinitely without harm, but a non-zero preemption rate
	// means work is actively being thrown away and recomputed.
	PreemptionsPerSec Metric `json:"preemptions_per_sec"`
	// PrefixCacheHitRatePct is the share of queried prefill tokens served from
	// the prefix cache.
	PrefixCacheHitRatePct Metric `json:"prefix_cache_hit_rate_percent"`

	// Errors. ErrorRatePct is only populated for runtimes that expose a
	// failure counter; vLLM does not, so it stays unmeasured there rather than
	// being invented from the success counter.
	ErrorRatePct Metric `json:"error_rate_percent"`
	// AbortRatePct is the share of finished requests the runtime recorded as
	// aborted, which usually means clients gave up waiting.
	AbortRatePct Metric `json:"abort_rate_percent"`

	// Kubernetes context, joined in from the workload watcher when available.
	Replicas      Metric `json:"replicas"`
	ReadyReplicas Metric `json:"ready_replicas"`
	MaxReplicas   Metric `json:"max_replicas"`

	// ScrapeDurationMs and MetricsMissing describe the scrape itself and let
	// the API surface a degraded target rather than silently reporting gaps as
	// healthy.
	ScrapeDurationMs Metric   `json:"scrape_duration_ms"`
	MetricsMissing   []string `json:"metrics_missing,omitempty"`
}

// Age returns how long ago the snapshot was taken relative to now.
func (s Snapshot) Age(now time.Time) time.Duration {
	return now.Sub(s.Timestamp)
}

// Key identifies the workload a snapshot belongs to. Workload names are only
// unique within a namespace.
func (s Snapshot) Key() string {
	if s.Namespace == "" {
		return s.WorkloadName
	}
	return s.Namespace + "/" + s.WorkloadName
}

// Severity ranks a recommendation. The ordering is meaningful: callers sort and
// filter on it.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Rank returns a sortable weight, highest severity first.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityWarning:
		return 1
	case SeverityInfo:
		return 2
	default:
		return 3
	}
}

// Evidence is one measurement that contributed to a recommendation, together
// with the threshold it was compared against.
//
// Recommendations carry their evidence so that an operator can check the
// reasoning instead of trusting it. A rule that cannot say which number moved
// and what it was compared to is an alert, not a diagnosis.
type Evidence struct {
	// Metric is the field name in the telemetry payload, e.g. "kv_cache_usage_percent".
	Metric string `json:"metric"`
	// Source is the runtime metric the value was derived from, e.g.
	// "vllm:kv_cache_usage_perc". Empty when the value is computed.
	Source    string  `json:"source,omitempty"`
	Observed  float64 `json:"observed"`
	Threshold float64 `json:"threshold,omitempty"`
	// Comparison is ">", "<", ">=" or "<=" — how Observed was compared to
	// Threshold. Empty when the value is context rather than a trigger.
	Comparison string `json:"comparison,omitempty"`
	Unit       string `json:"unit,omitempty"`
}

// Recommendation is one diagnosis for one workload.
type Recommendation struct {
	// ID is stable for as long as the condition persists: it is derived from
	// the rule code and the workload, not from evaluation order. Two calls to
	// the API while the same condition holds return the same ID, so clients can
	// deduplicate and track a finding over time.
	ID string `json:"id"`
	// Code is the rule identifier, e.g. "IFA-KV-002". Codes are permanent;
	// docs/RECOMMENDATIONS.md documents each one.
	Code         string   `json:"code"`
	Severity     Severity `json:"severity"`
	Namespace    string   `json:"namespace"`
	WorkloadName string   `json:"workload_name"`
	Runtime      Runtime  `json:"runtime"`
	Title        string   `json:"title"`
	// Explanation states what was observed and why it matters.
	Explanation string `json:"explanation"`
	// SuggestedAction is what an operator would do next. It is advice, never
	// an action the system takes.
	SuggestedAction string     `json:"suggested_action"`
	Evidence        []Evidence `json:"evidence"`
	// ObservedAt is the timestamp of the telemetry that triggered the rule,
	// not the time the API was called.
	ObservedAt time.Time `json:"observed_at"`
	// WindowSeconds is the span of telemetry the rule considered. Rules that
	// require a condition to persist report the window they sustained over.
	WindowSeconds int `json:"window_seconds,omitempty"`
}
