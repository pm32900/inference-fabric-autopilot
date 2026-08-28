package triton

// Triton Inference Server Prometheus metric names.
// Triton exposes these at /metrics when started with --allow-metrics=true.
// Metrics are labelled by model name and version: {model="resnet50",version="1"}.
// Ref: https://docs.nvidia.com/deeplearning/triton-inference-server/user-guide/docs/user_guide/metrics.html
const (
	// MetricInferenceSuccess is the total number of successful inference requests per model.
	MetricInferenceSuccess = "nv_inference_request_success"

	// MetricInferenceFailure is the total number of failed inference requests per model.
	MetricInferenceFailure = "nv_inference_request_failure"

	// MetricInferenceCount is the total number of inference executions (batches) per model.
	MetricInferenceCount = "nv_inference_count"

	// MetricInferenceExecCount is the number of model executions (may differ from request count due to batching).
	MetricInferenceExecCount = "nv_inference_exec_count"

	// MetricRequestDurationUs is cumulative request duration in microseconds per model.
	MetricRequestDurationUs = "nv_inference_request_duration_us"

	// MetricQueueDurationUs is cumulative time requests spent queued in microseconds per model.
	// High values relative to request duration indicate queue pressure.
	MetricQueueDurationUs = "nv_inference_queue_duration_us"

	// MetricComputeInputDurationUs is cumulative input processing time in microseconds.
	MetricComputeInputDurationUs = "nv_inference_compute_input_duration_us"

	// MetricComputeInferDurationUs is cumulative model inference time in microseconds.
	MetricComputeInferDurationUs = "nv_inference_compute_infer_duration_us"

	// MetricComputeOutputDurationUs is cumulative output processing time in microseconds.
	MetricComputeOutputDurationUs = "nv_inference_compute_output_duration_us"

	// MetricGPUUtilization is GPU utilization as a percentage [0, 100].
	// This is a server-level metric, not per-model.
	MetricGPUUtilization = "nv_gpu_utilization"

	// MetricGPUMemoryUsedBytes is GPU memory used in bytes.
	MetricGPUMemoryUsedBytes = "nv_gpu_memory_used_bytes"

	// MetricGPUMemoryTotalBytes is total GPU memory in bytes.
	MetricGPUMemoryTotalBytes = "nv_gpu_memory_total_bytes"

	// MetricPendingRequestCount is the number of requests currently queued, per model.
	MetricPendingRequestCount = "nv_inference_pending_request_count"
)

// TritonSnapshot holds the normalised telemetry for one Triton model at one point in time.
// Triton can serve multiple models simultaneously; the collector aggregates across models
// or targets one model per PrometheusTarget using the ModelName filter.
//
// Counter fields (suffixed *Total) are raw cumulative values.
// The collector's RateTracker converts them to per-second rates.
type TritonSnapshot struct {
	// ModelName is extracted from the "model" label on each metric line.
	ModelName string

	// Request counters (raw cumulative)
	InferenceSuccessTotal float64
	InferenceFailureTotal float64
	InferenceCountTotal   float64

	// Duration counters in microseconds (raw cumulative)
	// Divide by InferenceSuccessTotal to get average latency per request.
	RequestDurationUsTotal      float64
	QueueDurationUsTotal        float64
	ComputeInferDurationUsTotal float64

	// Queue depth — instantaneous gauge
	PendingRequestCount int

	// GPU metrics — server-level, not per-model
	// These are the max across all GPU devices found in the payload.
	GPUUtilizationPct  float64
	GPUMemoryUsedBytes float64
	GPUMemoryTotalBytes float64
	GPUMemoryUsedPct   float64 // derived: (Used/Total)*100
}
