package vllm

import (
	"bufio"
	"strconv"
	"strings"
)

// Parse reads a Prometheus text-format payload and extracts all known
// vLLM metrics into a VLLMSnapshot.
//
// The input is the raw body of a GET /metrics response — pass it as a string.
// Missing metrics are silently zero-valued; the function never returns an error
// for missing or unrecognised metric lines.
//
// Example:
//
//	snap := vllm.Parse(metricsBody)
//	fmt.Println(snap.NumRequestsWaiting)
func Parse(prometheusText string) VLLMSnapshot {
	m := parseText(prometheusText)

	return VLLMSnapshot{
		NumRequestsRunning: int(get(m, MetricNumRequestsRunning)),
		NumRequestsWaiting: int(get(m, MetricNumRequestsWaiting)),

		// KV cache is reported in [0,1] — convert to percentage
		KVCacheUsagePct: get(m, MetricGPUCacheUsagePerc) * 100,

		// Latencies are in seconds — convert to milliseconds
		E2ELatencyP50Ms: get(m, MetricE2ELatencyP50) * 1000,
		E2ELatencyP95Ms: get(m, MetricE2ELatencyP95) * 1000,
		E2ELatencyP99Ms: get(m, MetricE2ELatencyP99) * 1000,

		TTFTP50Ms: get(m, MetricTTFTP50) * 1000,
		TTFTP95Ms: get(m, MetricTTFTP95) * 1000,
		TTFTP99Ms: get(m, MetricTTFTP99) * 1000,

		// Counters are stored as-is; caller computes rate between scrapes
		GenerationTokensTotal: get(m, MetricGenerationTokensTotal),
		PromptTokensTotal:     get(m, MetricPromptTokensTotal),
		RequestSuccessTotal:   get(m, MetricRequestSuccessTotal),
		RequestFailureTotal:   get(m, MetricRequestFailureTotal),
	}
}

// parseText scans Prometheus text format and returns a flat map of
// metric_name{labels} → float64.
//
// Lines starting with # (HELP / TYPE) and blank lines are skipped.
// Lines that cannot be parsed are silently skipped — the function never panics.
//
// Format handled:
//
//	metric_name value
//	metric_name{label="val"} value
//	metric_name{label="val"} value timestamp   (timestamp ignored)
func parseText(text string) map[string]float64 {
	result := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(text))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// skip blank lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// find the last space to split key from value
		// (label sets can contain spaces inside quotes, so we use LastIndex)
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace < 0 {
			continue
		}

		key := strings.TrimSpace(line[:lastSpace])
		rest := strings.TrimSpace(line[lastSpace+1:])

		// strip optional timestamp (a second space-separated token after the value)
		if idx := strings.Index(rest, " "); idx >= 0 {
			rest = rest[:idx]
		}

		val, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			continue
		}

		result[key] = val
	}

	return result
}

// get safely retrieves a value from the map, returning 0 if the key is absent.
func get(m map[string]float64, key string) float64 {
	return m[key]
}
