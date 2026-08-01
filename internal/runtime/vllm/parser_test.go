package vllm

import (
	"testing"
)

// fixture is a realistic vLLM /metrics payload covering all supported metrics.
const fixture = `# HELP vllm:num_requests_running Number of requests currently being processed
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running 4
# HELP vllm:num_requests_waiting Number of requests waiting to be processed
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting 7
# HELP vllm:gpu_cache_usage_perc GPU KV-cache usage fraction [0,1]
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.82
# HELP vllm:e2e_request_latency_seconds End-to-end request latency
# TYPE vllm:e2e_request_latency_seconds summary
vllm:e2e_request_latency_seconds{quantile="0.5"} 0.45
vllm:e2e_request_latency_seconds{quantile="0.95"} 1.20
vllm:e2e_request_latency_seconds{quantile="0.99"} 2.10
# HELP vllm:time_to_first_token_seconds Time to first token
# TYPE vllm:time_to_first_token_seconds summary
vllm:time_to_first_token_seconds{quantile="0.5"} 0.10
vllm:time_to_first_token_seconds{quantile="0.95"} 1.35
vllm:time_to_first_token_seconds{quantile="0.99"} 2.80
# HELP vllm:generation_tokens_total Tokens generated (counter)
# TYPE vllm:generation_tokens_total counter
vllm:generation_tokens_total 98432
# HELP vllm:prompt_tokens_total Prompt tokens processed (counter)
# TYPE vllm:prompt_tokens_total counter
vllm:prompt_tokens_total 12048
# HELP vllm:request_success_total Successful requests (counter)
# TYPE vllm:request_success_total counter
vllm:request_success_total 512
# HELP vllm:request_failure_total Failed requests (counter)
# TYPE vllm:request_failure_total counter
vllm:request_failure_total 3
`

func TestParse_FullPayload(t *testing.T) {
	snap := Parse(fixture)

	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"NumRequestsRunning", float64(snap.NumRequestsRunning), 4},
		{"NumRequestsWaiting", float64(snap.NumRequestsWaiting), 7},
		{"KVCacheUsagePct", snap.KVCacheUsagePct, 82.0},
		{"E2ELatencyP50Ms", snap.E2ELatencyP50Ms, 450.0},
		{"E2ELatencyP95Ms", snap.E2ELatencyP95Ms, 1200.0},
		{"E2ELatencyP99Ms", snap.E2ELatencyP99Ms, 2100.0},
		{"TTFTP50Ms", snap.TTFTP50Ms, 100.0},
		{"TTFTP95Ms", snap.TTFTP95Ms, 1350.0},
		{"TTFTP99Ms", snap.TTFTP99Ms, 2800.0},
		{"GenerationTokensTotal", snap.GenerationTokensTotal, 98432},
		{"PromptTokensTotal", snap.PromptTokensTotal, 12048},
		{"RequestSuccessTotal", snap.RequestSuccessTotal, 512},
		{"RequestFailureTotal", snap.RequestFailureTotal, 3},
	}

	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestParse_EmptyPayload(t *testing.T) {
	snap := Parse("")

	if snap.NumRequestsRunning != 0 || snap.NumRequestsWaiting != 0 ||
		snap.KVCacheUsagePct != 0 || snap.TTFTP95Ms != 0 {
		t.Error("expected all fields to be zero for empty payload")
	}
}

func TestParse_PartialPayload(t *testing.T) {
	partial := `vllm:num_requests_waiting 3
vllm:gpu_cache_usage_perc 0.55
`
	snap := Parse(partial)

	if snap.NumRequestsWaiting != 3 {
		t.Errorf("NumRequestsWaiting: got %d, want 3", snap.NumRequestsWaiting)
	}
	if snap.KVCacheUsagePct < 54.999 || snap.KVCacheUsagePct > 55.001 {
		t.Errorf("KVCacheUsagePct: got %.6f, want ~55.00", snap.KVCacheUsagePct)
	}
	if snap.TTFTP95Ms != 0 {
		t.Errorf("TTFTP95Ms should be 0 when absent, got %.2f", snap.TTFTP95Ms)
	}
	if snap.NumRequestsRunning != 0 {
		t.Errorf("NumRequestsRunning should be 0 when absent, got %d", snap.NumRequestsRunning)
	}
}

func TestParse_MalformedLines(t *testing.T) {
	// Malformed lines must not panic and must not pollute other values
	malformed := `this is not a valid metric line
vllm:num_requests_waiting notanumber
# just a comment
vllm:num_requests_running 2
`
	snap := Parse(malformed)

	if snap.NumRequestsRunning != 2 {
		t.Errorf("NumRequestsRunning: got %d, want 2 — malformed lines should not affect valid ones", snap.NumRequestsRunning)
	}
	if snap.NumRequestsWaiting != 0 {
		t.Errorf("NumRequestsWaiting: got %d, want 0 — non-numeric value should be skipped", snap.NumRequestsWaiting)
	}
}

func TestParse_KVCacheConversion(t *testing.T) {
	// KV cache is [0,1] in Prometheus — must be multiplied by 100
	input := `vllm:gpu_cache_usage_perc 1.0`
	snap := Parse(input)
	if snap.KVCacheUsagePct < 99.999 || snap.KVCacheUsagePct > 100.001 {
		t.Errorf("KVCacheUsagePct: got %.6f, want ~100.00 (1.0 * 100)", snap.KVCacheUsagePct)
	}
}

func TestParse_LatencyConversion(t *testing.T) {
	// Latencies are in seconds in Prometheus — must be converted to milliseconds
	input := `vllm:time_to_first_token_seconds{quantile="0.95"} 1.5`
	snap := Parse(input)
	if snap.TTFTP95Ms != 1500.0 {
		t.Errorf("TTFTP95Ms: got %.2f, want 1500.00 (1.5s * 1000)", snap.TTFTP95Ms)
	}
}

func TestParse_NoPanic_GarbageInput(t *testing.T) {
	// Must not panic on any input
	inputs := []string{
		"",
		"   ",
		"\n\n\n",
		"{}{}{}",
		"vllm:metric",
		"# only comments\n# another comment",
	}
	for _, input := range inputs {
		// if this panics, the test will fail with a runtime error
		_ = Parse(input)
	}
}
