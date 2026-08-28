package triton

import (
	"strconv"
	"strings"
)

// Parse reads a Prometheus text-format payload from a Triton Inference Server
// /metrics endpoint and returns one TritonSnapshot per model found.
//
// If modelFilter is non-empty, only the snapshot for that model name is returned
// (slice of length 0 or 1). Pass an empty string to return all models.
//
// GPU metrics (nv_gpu_*) are server-level — not per-model. The parser aggregates
// them as max across GPU devices and writes the same values into every snapshot.
//
// Returns an empty slice (never nil) when no recognised metrics are found.
func Parse(text, modelFilter string) []TritonSnapshot {
	type modelRow struct {
		successTotal        float64
		failureTotal        float64
		inferenceCountTotal float64
		requestDurUs        float64
		queueDurUs          float64
		computeInferUs      float64
		pendingCount        int
	}

	models := make(map[string]*modelRow)

	var maxGPUUtil, maxGPUUsedBytes, maxGPUTotalBytes float64

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		metricName, labels, value, ok := parseTritonLine(line)
		if !ok {
			continue
		}

		// GPU metrics are server-level — no model label
		switch metricName {
		case MetricGPUUtilization:
			if value > maxGPUUtil {
				maxGPUUtil = value
			}
			continue
		case MetricGPUMemoryUsedBytes:
			if value > maxGPUUsedBytes {
				maxGPUUsedBytes = value
			}
			continue
		case MetricGPUMemoryTotalBytes:
			if value > maxGPUTotalBytes {
				maxGPUTotalBytes = value
			}
			continue
		}

		// All other metrics are per-model
		modelName := extractLabel(labels, "model")
		if modelName == "" {
			continue
		}
		if modelFilter != "" && modelName != modelFilter {
			continue
		}

		row, exists := models[modelName]
		if !exists {
			row = &modelRow{}
			models[modelName] = row
		}

		switch metricName {
		case MetricInferenceSuccess:
			row.successTotal = value
		case MetricInferenceFailure:
			row.failureTotal = value
		case MetricInferenceCount:
			row.inferenceCountTotal = value
		case MetricRequestDurationUs:
			row.requestDurUs = value
		case MetricQueueDurationUs:
			row.queueDurUs = value
		case MetricComputeInferDurationUs:
			row.computeInferUs = value
		case MetricPendingRequestCount:
			row.pendingCount = int(value)
		}
	}

	if len(models) == 0 {
		return []TritonSnapshot{}
	}

	gpuMemPct := 0.0
	if maxGPUTotalBytes > 0 {
		gpuMemPct = (maxGPUUsedBytes / maxGPUTotalBytes) * 100.0
	}

	snaps := make([]TritonSnapshot, 0, len(models))
	for name, row := range models {
		snaps = append(snaps, TritonSnapshot{
			ModelName:                   name,
			InferenceSuccessTotal:       row.successTotal,
			InferenceFailureTotal:       row.failureTotal,
			InferenceCountTotal:         row.inferenceCountTotal,
			RequestDurationUsTotal:      row.requestDurUs,
			QueueDurationUsTotal:        row.queueDurUs,
			ComputeInferDurationUsTotal: row.computeInferUs,
			PendingRequestCount:         row.pendingCount,
			GPUUtilizationPct:           maxGPUUtil,
			GPUMemoryUsedBytes:          maxGPUUsedBytes,
			GPUMemoryTotalBytes:         maxGPUTotalBytes,
			GPUMemoryUsedPct:            gpuMemPct,
		})
	}
	return snaps
}

// parseTritonLine parses one Prometheus text line of the form:
//
//	nv_inference_request_success{model="resnet50",version="1"} 1024
//
// Returns metric name, raw label string, float value, and whether parsing succeeded.
func parseTritonLine(line string) (metricName, labels string, value float64, ok bool) {
	braceOpen := strings.Index(line, "{")
	braceClose := strings.Index(line, "}")

	var rest string
	if braceOpen >= 0 && braceClose > braceOpen {
		metricName = strings.TrimSpace(line[:braceOpen])
		labels = line[braceOpen+1 : braceClose]
		rest = strings.TrimSpace(line[braceClose+1:])
	} else {
		// No labels — server-level metric like nv_gpu_utilization 87.3
		parts := strings.Fields(line)
		if len(parts) < 2 {
			return "", "", 0, false
		}
		metricName = parts[0]
		rest = parts[1]
	}

	// Strip optional timestamp
	valueStr := rest
	if idx := strings.Index(rest, " "); idx >= 0 {
		valueStr = rest[:idx]
	}

	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return "", "", 0, false
	}

	return metricName, labels, val, true
}

// extractLabel returns the value of the named label from a Prometheus label string.
// e.g. extractLabel(`model="resnet50",version="1"`, "model") → "resnet50"
func extractLabel(labels, key string) string {
	prefix := key + `="`
	for _, part := range strings.Split(labels, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) {
			inner := strings.TrimPrefix(part, prefix)
			inner = strings.TrimSuffix(inner, `"`)
			return inner
		}
	}
	return ""
}
