package recommender

import (
	"testing"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/config"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

var defaultThresholds = config.ThresholdConfig{
	LowGPUUtilPct:     30.0,
	HighGPUMemPct:     85.0,
	HighP95LatencyMs:  500.0,
	HighQueueDepth:    10,
	HighErrorRatePct:  2.0,
	MinReplicasForRPS: 10.0,
}

func baseSnap(name string) telemetry.Snapshot {
	return telemetry.Snapshot{
		Timestamp:    time.Now(),
		ClusterName:  "test",
		Namespace:    "inference",
		WorkloadName: name,
		Runtime:      "vllm",
	}
}

func titlesOf(recs []telemetry.Recommendation) []string {
	out := make([]string, len(recs))
	for i, r := range recs {
		out[i] = r.Title
	}
	return out
}

func TestAnalyze_NoRecsForHealthyWorkload(t *testing.T) {
	snap := baseSnap("healthy")
	snap.GPUUtilizationPct = 60
	snap.GPUMemoryUsedPct = 65
	snap.QueueDepth = 3
	snap.P95LatencyMs = 200
	snap.P99LatencyMs = 300
	snap.ErrorRatePct = 0.5
	snap.RequestRatePerSec = 5
	snap.TokensPerSecond = 500

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations for healthy workload, got %d: %v", len(recs), titlesOf(recs))
	}
}

func TestRule1_LowGPUHighQueue(t *testing.T) {
	snap := baseSnap("w1")
	snap.GPUUtilizationPct = 10 // below 30
	snap.QueueDepth = 15        // above 10

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "Low GPU utilization with high queue depth" {
			return
		}
	}
	t.Error("expected Rule 1 to fire but it did not")
}

func TestRule2_HighP95Latency(t *testing.T) {
	snap := baseSnap("w2")
	snap.P95LatencyMs = 600 // above 500

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "High p95 latency" {
			return
		}
	}
	t.Error("expected Rule 2 to fire but it did not")
}

func TestRule3_HighGPUMemory(t *testing.T) {
	snap := baseSnap("w3")
	snap.GPUMemoryUsedPct = 90 // above 85

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "High GPU memory pressure" {
			return
		}
	}
	t.Error("expected Rule 3 to fire but it did not")
}

func TestRule4_HighErrorRate(t *testing.T) {
	snap := baseSnap("w4")
	snap.ErrorRatePct = 5.0 // above 2.0

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "Elevated error rate" {
			return
		}
	}
	t.Error("expected Rule 4 to fire but it did not")
}

func TestRule5_ReplicasTooLow(t *testing.T) {
	snap := baseSnap("w5")
	snap.RequestRatePerSec = 50   // above MinReplicasForRPS=10
	snap.P99LatencyMs = 900       // above HighP95LatencyMs*1.5 = 750
	snap.P95LatencyMs = 550

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "Replica count may be too low for current traffic" {
			return
		}
	}
	t.Error("expected Rule 5 to fire but it did not")
}

func TestRule6_GPUSaturated(t *testing.T) {
	snap := baseSnap("w6")
	snap.GPUUtilizationPct = 90 // above 85
	snap.QueueDepth = 15        // above 10

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "GPU saturated with requests queuing" {
			return
		}
	}
	t.Error("expected Rule 6 to fire but it did not")
}

func TestRule7_LowTokenThroughput(t *testing.T) {
	snap := baseSnap("w7")
	snap.GPUUtilizationPct = 80 // above 70
	snap.TokensPerSecond = 100  // below 200

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "Low token throughput despite high GPU utilization" {
			return
		}
	}
	t.Error("expected Rule 7 to fire but it did not")
}

func TestRule8_P99P95Gap(t *testing.T) {
	snap := baseSnap("w8")
	snap.P95LatencyMs = 100
	snap.P99LatencyMs = 400 // ratio = 4x, above threshold of 3x

	recs := Analyze([]telemetry.Snapshot{snap}, defaultThresholds)
	for _, r := range recs {
		if r.Title == "Large p99/p95 latency gap — outlier requests detected" {
			return
		}
	}
	t.Error("expected Rule 8 to fire but it did not")
}

func TestAnalyze_IDsAreUnique(t *testing.T) {
	snaps := []telemetry.Snapshot{
		func() telemetry.Snapshot {
			s := baseSnap("stressed")
			s.GPUUtilizationPct = 10
			s.QueueDepth = 20
			s.P95LatencyMs = 700
			s.GPUMemoryUsedPct = 92
			s.ErrorRatePct = 5
			return s
		}(),
	}
	recs := Analyze(snaps, defaultThresholds)
	seen := make(map[string]bool)
	for _, r := range recs {
		if seen[r.ID] {
			t.Errorf("duplicate recommendation ID: %s", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestAnalyze_EmptyInput(t *testing.T) {
	recs := Analyze([]telemetry.Snapshot{}, defaultThresholds)
	if len(recs) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(recs))
	}
}
