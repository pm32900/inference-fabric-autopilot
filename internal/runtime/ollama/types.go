package ollama

// OllamaSnapshot holds normalised telemetry for one Ollama model at one point in time.
// Ollama does not expose a Prometheus /metrics endpoint. All values are derived
// by polling the Ollama REST API (/api/ps and /api/show) and computing rates
// between successive polls in the OllamaCollector.
//
// Counter fields (suffixed *Total) are raw values from the API response.
// The collector's RateTracker converts them to per-second rates before
// writing to the telemetry store.
type OllamaSnapshot struct {
	// ModelName is the Ollama model tag e.g. "llama3:8b", "nomic-embed-text".
	ModelName string

	// SizeBytes is the size of the loaded model in bytes, from /api/ps.
	SizeBytes int64

	// VRAM used by this model in bytes.
	SizeVRAM int64

	// PromptEvalCount is the cumulative number of prompt tokens processed.
	// Divide the delta by elapsed seconds to get prompt tokens/sec.
	PromptEvalCount int64

	// EvalCount is the cumulative number of generation tokens produced.
	// Divide the delta by elapsed seconds to get tokens/sec.
	EvalCount int64

	// PromptEvalDurationNs is the cumulative time spent on prompt evaluation
	// in nanoseconds. Divide by PromptEvalCount to get avg time-to-first-token.
	PromptEvalDurationNs int64

	// EvalDurationNs is the cumulative time spent on token generation in nanoseconds.
	// Divide by EvalCount to get avg time per token.
	EvalDurationNs int64

	// TotalDurationNs is the cumulative total request duration in nanoseconds.
	TotalDurationNs int64

	// ExpiresAt is the time the model will be evicted from VRAM if idle.
	// Zero value means the model is pinned.
	ExpiresAt string

	// NumGPU is the number of GPU layers offloaded for this model.
	NumGPU int

	// NumCPU is the number of CPU-only layers for this model.
	NumCPU int
}

// PSResponse mirrors the JSON structure returned by GET /api/ps.
// Only the fields used by the adapter are mapped.
type PSResponse struct {
	Models []PSModel `json:"models"`
}

// PSModel is one entry in the /api/ps model list.
type PSModel struct {
	Name      string  `json:"name"`
	Size      int64   `json:"size"`
	SizeVRAM  int64   `json:"size_vram"`
	ExpiresAt string  `json:"expires_at"`
	Details   Details `json:"details"`
}

// Details holds layer distribution info from /api/ps.
type Details struct {
	NumGPU int `json:"num_gpu_layers"`
	NumCPU int `json:"num_cpu_layers"`
}

// GenerateResponse mirrors the relevant fields from POST /api/generate
// (called with stream:false and an empty prompt to get a warm stats ping).
// Ollama returns these counters after each request.
type GenerateResponse struct {
	Model                string `json:"model"`
	PromptEvalCount      int64  `json:"prompt_eval_count"`
	EvalCount            int64  `json:"eval_count"`
	PromptEvalDurationNs int64  `json:"prompt_eval_duration"`
	EvalDurationNs       int64  `json:"eval_duration"`
	TotalDurationNs      int64  `json:"total_duration"`
}
