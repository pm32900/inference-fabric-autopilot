package collector

import (
	"math/rand"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

// workloadProfile defines the baseline characteristics of a simulated workload.
// Each workload drifts slightly each tick to simulate real variance.
type workloadProfile struct {
	name      string
	namespace string
	runtime   string
	model     string
	// baseline values — each profile has its own "normal" operating point
	gpuUtilBase float64 // center of GPU utilization range
	gpuMemBase  float64 // center of GPU memory range
	queueBase   float64 // center of queue depth range
	p95Base     float64 // center of p95 latency range
	errRateBase float64 // center of error rate range
}

// profiles — four workloads with distinct operating characteristics.
// "gpu-stressed" is intentionally unhealthy so recommendations always fire.
var profiles = []workloadProfile{
	// healthy: moderate GPU util, low queue, normal latency
	{name: "llm-serving-a", namespace: "inference", runtime: "vllm", model: "llama-3-8b", gpuUtilBase: 60, gpuMemBase: 65, queueBase: 4, p95Base: 220, errRateBase: 0.4},
	// healthy: high GPU util (busy but fine)
	{name: "llm-serving-b", namespace: "inference", runtime: "triton", model: "mistral-7b", gpuUtilBase: 75, gpuMemBase: 68, queueBase: 3, p95Base: 180, errRateBase: 0.6},
	// healthy: embedding workload, fast latency
	{name: "embedding-svc", namespace: "inference", runtime: "ollama", model: "nomic-embed", gpuUtilBase: 40, gpuMemBase: 55, queueBase: 2, p95Base: 150, errRateBase: 0.3},
	// stressed: low GPU util + high queue + high memory — designed to trigger all rules
	{name: "gpu-stressed", namespace: "inference", runtime: "vllm", model: "llama-3-70b", gpuUtilBase: 18, gpuMemBase: 90, queueBase: 20, p95Base: 650, errRateBase: 3.5},
}

func Start(store *telemetry.Store, interval time.Duration) {
	go func() {
		for {
			for _, p := range profiles {
				snap := generate(p)
				store.Add(snap)
			}
			time.Sleep(interval)
		}
	}()
}

// generate builds one fake Snapshot for a given workload profile.
// Small jitter is applied around each profile's baseline values.
func generate(p workloadProfile) telemetry.Snapshot {
	// jitter returns a float in [base-delta, base+delta]
	jitter := func(base, delta float64) float64 {
		return base + (rand.Float64()*2-1)*delta
	}

	// use the profile's own baselines — small ±5 jitter keeps values realistic
	gpuUtil := clamp(jitter(p.gpuUtilBase, 5), 0, 100)
	gpuMem := clamp(jitter(p.gpuMemBase, 5), 0, 100)
	p95 := clamp(jitter(p.p95Base, 50), 10, 5000)
	p50 := clamp(p95*0.5, 10, p95)
	p99 := clamp(p95*jitter(1.5, 0.2), p95, 10000)
	queueDepth := int(clamp(jitter(p.queueBase, 3), 0, 100))
	rps := clamp(jitter(20.0, 10.0), 0, 100)
	tps := clamp(jitter(800.0, 200.0), 0, 3000)
	errRate := clamp(jitter(p.errRateBase, 0.5), 0, 10)

	return telemetry.Snapshot{
		Timestamp:         time.Now().UTC(),
		ClusterName:       "local-dev",
		Namespace:         p.namespace,
		WorkloadName:      p.name,
		Runtime:           p.runtime,
		ModelName:         p.model,
		RequestRatePerSec: rps,
		P50LatencyMs:      p50,
		P95LatencyMs:      p95,
		P99LatencyMs:      p99,
		QueueDepth:        queueDepth,
		GPUUtilizationPct: gpuUtil,
		GPUMemoryUsedPct:  gpuMem,
		TokensPerSecond:   tps,
		ErrorRatePct:      errRate,
	}
}

// clamp ensures v stays within [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
