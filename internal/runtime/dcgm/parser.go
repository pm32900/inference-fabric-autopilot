package dcgm

import (
	"strconv"
	"strings"
)

// Parse reads a Prometheus text-format payload from a DCGM Exporter /metrics
// endpoint and returns one DCGMSnapshot per GPU device found.
//
// The returned slice is ordered by GPUIndex ascending.
// If the payload is empty or contains no recognised DCGM metrics, an empty
// slice is returned — never nil.
func Parse(text string) []DCGMSnapshot {
	type gpuRow struct {
		gpuUtil     float64
		memCopyUtil float64
		fbUsed      float64
		fbFree      float64
		smClock     float64
		temp        float64
	}
	rows := make(map[int]*gpuRow)

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		metricName, gpuIdx, value, ok := parseDCGMLine(line)
		if !ok {
			continue
		}

		row, exists := rows[gpuIdx]
		if !exists {
			row = &gpuRow{}
			rows[gpuIdx] = row
		}

		switch metricName {
		case MetricGPUUtil:
			row.gpuUtil = value
		case MetricMemCopyUtil:
			row.memCopyUtil = value
		case MetricFBUsed:
			row.fbUsed = value
		case MetricFBFree:
			row.fbFree = value
		case MetricSMClock:
			row.smClock = value
		case MetricGPUTemp:
			row.temp = value
		}
	}

	if len(rows) == 0 {
		return []DCGMSnapshot{}
	}

	maxIdx := 0
	for idx := range rows {
		if idx > maxIdx {
			maxIdx = idx
		}
	}

	snaps := make([]DCGMSnapshot, 0, len(rows))
	for i := 0; i <= maxIdx; i++ {
		row, ok := rows[i]
		if !ok {
			continue
		}
		total := row.fbUsed + row.fbFree
		usedPct := 0.0
		if total > 0 {
			usedPct = (row.fbUsed / total) * 100.0
		}
		snaps = append(snaps, DCGMSnapshot{
			GPUIndex:       i,
			GPUUtilPct:     row.gpuUtil,
			MemCopyUtilPct: row.memCopyUtil,
			FBUsedMiB:      row.fbUsed,
			FBFreeMiB:      row.fbFree,
			FBTotalMiB:     total,
			FBUsedPct:      usedPct,
			SMClockMHZ:     row.smClock,
			TempCelsius:    row.temp,
		})
	}
	return snaps
}

// parseDCGMLine parses one Prometheus text line of the form:
//
//	DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="GPU-abc"} 42
//
// Returns the metric name, gpu index, float value, and whether parsing succeeded.
func parseDCGMLine(line string) (metricName string, gpuIdx int, value float64, ok bool) {
	braceOpen := strings.Index(line, "{")
	braceClose := strings.Index(line, "}")

	if braceOpen < 0 || braceClose < 0 || braceClose < braceOpen {
		return "", 0, 0, false
	}

	metricName = strings.TrimSpace(line[:braceOpen])
	labelStr := line[braceOpen+1 : braceClose]
	rest := strings.TrimSpace(line[braceClose+1:])

	valueStr := rest
	if idx := strings.Index(rest, " "); idx >= 0 {
		valueStr = rest[:idx]
	}

	val, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return "", 0, 0, false
	}

	gpuIdx = extractGPUIndex(labelStr)
	return metricName, gpuIdx, val, true
}

// extractGPUIndex pulls the integer value of the gpu="N" label.
// Returns 0 if the label is absent or unparseable.
func extractGPUIndex(labels string) int {
	for _, part := range strings.Split(labels, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, `gpu="`) {
			inner := strings.TrimPrefix(part, `gpu="`)
			inner = strings.TrimSuffix(inner, `"`)
			idx, err := strconv.Atoi(inner)
			if err == nil {
				return idx
			}
		}
	}
	return 0
}
