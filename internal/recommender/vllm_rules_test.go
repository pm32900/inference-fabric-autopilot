package recommender

import (
	"testing"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/config"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

var vllmThresholds = config.ThresholdConfig{
	LowGPUUtilPct:       30.0,
	HighGPUMemPct:       85.0,
	HighP95LatencyMs:    500.0,
	HighQueueDepth:      10,
	HighErrorRatePct:    2.0,
	MinReplicasForRPS:   10.0,
	HighKVCacheUsagePct: 80.0,
	HighTTFTP95Ms:       2000.0,
}

func vllmSnap(name string) telemetry.Snapshot {
	return telemetry.Snapshot{
		Timestamp:    time.Now(),
		ClusterName:  "test",
		Namespace:    "inference",
		WorkloadName: name,
		Runtime:      "vllm",
	}
}

func TestRule9_VLLMQueuePressure(t *testing.T) {
	snap := vllmSnap("vllm-w1")
	snap.NumRequestsWaiting = 15 // above HighQueueDepth=10
	snap.QueueDepth = 15

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM queue pressure — waiting requests sustained" {
			return
		}
	}
	t.Error("expected Rule 9 (vLLM queue pressure) to fire but it did not")
}

func TestRule9_VLLMQueuePressure_BelowThreshold(t *testing.T) {
	snap := vllmSnap("vllm-w1-ok")
	snap.NumRequestsWaiting = 5 // below HighQueueDepth=10

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM queue pressure — waiting requests sustained" {
			t.Error("Rule 9 fired unexpectedly when below threshold")
			return
		}
	}
}

func TestRule10_VLLMKVCachePressure(t *testing.T) {
	snap := vllmSnap("vllm-w2")
	snap.KVCacheUsagePct = 90.0 // above HighKVCacheUsagePct=80

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM KV cache near capacity" {
			return
		}
	}
	t.Error("expected Rule 10 (vLLM KV cache pressure) to fire but it did not")
}

func TestRule10_VLLMKVCachePressure_BelowThreshold(t *testing.T) {
	snap := vllmSnap("vllm-w2-ok")
	snap.KVCacheUsagePct = 60.0 // below 80

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM KV cache near capacity" {
			t.Error("Rule 10 fired unexpectedly when below threshold")
			return
		}
	}
}

func TestRule11_VLLMTTFTDegradation(t *testing.T) {
	snap := vllmSnap("vllm-w3")
	snap.TTFTP95Ms = 3000.0 // above HighTTFTP95Ms=2000

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM p95 time-to-first-token too high" {
			return
		}
	}
	t.Error("expected Rule 11 (vLLM TTFT degradation) to fire but it did not")
}

func TestRule11_VLLMTTFTDegradation_BelowThreshold(t *testing.T) {
	snap := vllmSnap("vllm-w3-ok")
	snap.TTFTP95Ms = 800.0 // below 2000

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		if r.Title == "vLLM p95 time-to-first-token too high" {
			t.Error("Rule 11 fired unexpectedly when below threshold")
			return
		}
	}
}

func TestVLLMRules_NonVLLMRuntimeDoesNotFire(t *testing.T) {
	// Rules 9–11 must not fire for non-vLLM runtimes even if fields are set
	snap := telemetry.Snapshot{
		Timestamp:          time.Now(),
		WorkloadName:       "triton-w1",
		Runtime:            "triton",
		NumRequestsWaiting: 20,
		KVCacheUsagePct:    95.0,
		TTFTP95Ms:          5000.0,
	}

	recs := Analyze([]telemetry.Snapshot{snap}, vllmThresholds)
	for _, r := range recs {
		switch r.Title {
		case "vLLM queue pressure — waiting requests sustained",
			"vLLM KV cache near capacity",
			"vLLM p95 time-to-first-token too high":
			t.Errorf("vLLM rule %q fired for non-vLLM runtime %q", r.Title, snap.Runtime)
		}
	}
}
