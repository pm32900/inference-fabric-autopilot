// Package demo runs a self-contained demonstration of IFA against simulated
// inference workloads.
//
// The simulation is deliberately at the wrong layer to be a shortcut. It does
// not fabricate telemetry snapshots or recommendations: it stands up an HTTP
// server that emits Prometheus exposition in vLLM's own format, with vLLM's own
// metric names and histogram bucket boundaries, and lets the real collector
// scrape it over a real socket. Everything downstream — the exposition parser,
// the histogram quantile estimation, counter-to-rate conversion, the rule engine
// — is the code that runs in production. A demo that printed canned JSON would
// prove nothing about any of it.
//
// Each scenario models a failure mode that inference platforms actually hit,
// and each is designed to produce a specific, checkable diagnosis.
package demo

import (
	"math"
	"math/rand"
	"time"
)

// Scenario describes one simulated workload and the behaviour it exhibits.
type Scenario struct {
	// Name is the workload name that appears in the API.
	Name string
	// Model is the model_name label the simulated server reports.
	Model string
	// Description is shown by `make demo` so a reader knows what they are
	// looking at.
	Description string
	// Expect names the rule code the scenario is built to trigger, or an empty
	// string for the healthy workload. It is asserted in tests, so a scenario
	// that stops reproducing its failure mode fails the build rather than
	// quietly becoming a healthy workload in the demo output.
	Expect string
	// Replicas and MaxReplicas simulate Kubernetes context for the scaling
	// rules. Zero means the workload is not backed by a known deployment.
	Replicas    int32
	MaxReplicas int32
	// SparseMetrics makes the simulated server expose only scheduler state,
	// modelling a runtime too old or too locked down to report the rest.
	SparseMetrics bool

	// advance mutates the engine state by one tick of dt.
	advance func(s *engineState, dt time.Duration, rng *rand.Rand)
}

// Scenarios returns the demo workload set, in display order.
func Scenarios() []Scenario {
	return []Scenario{
		healthy(),
		kvPressure(),
		capacityBound(),
		batchingMisconfigured(),
		prefillBound(),
		autoscaleCeiling(),
		partiallyInstrumented(),
	}
}

// healthy is the control. A demo in which everything is on fire proves only
// that the thresholds are low.
func healthy() Scenario {
	return Scenario{
		Name:        "chat-llama3-8b",
		Model:       "meta-llama/Llama-3.1-8B-Instruct",
		Description: "Healthy interactive chat serving. Should produce no findings.",
		Expect:      "",
		Replicas:    3, MaxReplicas: 10,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 8 + rng.Float64()*4
			s.waiting = rng.Float64() * 2
			s.waitingCapacity = s.waiting
			s.kvCache = 0.55 + rng.Float64()*0.1
			s.gpuUtil = 62 + rng.Float64()*8
			s.gpuMemPct = 71 + rng.Float64()*2

			n := s.completeRequests(dt, 22, 0.004, rng)
			for i := 0; i < n; i++ {
				s.observeTTFT(lognormal(rng, 0.12, 0.35))
				s.observeQueue(lognormal(rng, 0.01, 0.5))
				s.observeE2E(lognormal(rng, 1.8, 0.4))
			}
			s.addTokens(dt, 900, 2600, rng)
			s.addPrefixCache(dt, 2600, 0.62, rng)
		},
	}
}

// kvPressure models the failure mode a KV-cache utilisation threshold cannot
// distinguish from healthy operation: the cache is full *and* the engine is
// preempting to keep it that way.
func kvPressure() Scenario {
	return Scenario{
		Name:        "summarise-mixtral",
		Model:       "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Description: "Long-context summarisation exhausting the KV cache; the engine is preempting.",
		Expect:      "IFA-KV-001",
		Replicas:    2, MaxReplicas: 8,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 14 + rng.Float64()*3
			s.waiting = 4 + rng.Float64()*3
			s.waitingCapacity = s.waiting
			s.kvCache = 0.965 + rng.Float64()*0.03
			s.gpuUtil = 88 + rng.Float64()*6
			s.gpuMemPct = 96 + rng.Float64()*1.5

			s.preemptions += dt.Seconds() * (1.6 + rng.Float64())

			n := s.completeRequests(dt, 6, 0.01, rng)
			for i := 0; i < n; i++ {
				s.observeTTFT(lognormal(rng, 0.7, 0.5))
				s.observeQueue(lognormal(rng, 0.25, 0.6))
				s.observeE2E(lognormal(rng, 14, 0.5))
			}
			s.addTokens(dt, 420, 9000, rng)
			s.addPrefixCache(dt, 9000, 0.31, rng)
		},
	}
}

// capacityBound is the classic shortage: the accelerator is busy, the queue is
// deep, and it is getting deeper.
func capacityBound() Scenario {
	return Scenario{
		Name:        "assistant-qwen-14b",
		Model:       "Qwen/Qwen2.5-14B-Instruct",
		Description: "Traffic beyond capacity: GPU saturated, queue growing, callers timing out.",
		Expect:      "IFA-CAP-001",
		Replicas:    4, MaxReplicas: 12,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			// The queue climbs steadily so the trend rule has something real
			// to detect rather than a static high value.
			s.waiting = math.Min(s.waiting+dt.Seconds()*0.9, 140) + rng.Float64()
			if s.waiting < 14 {
				s.waiting = 14 + rng.Float64()
			}
			s.waitingCapacity = s.waiting
			s.running = 30 + rng.Float64()*4
			s.kvCache = 0.78 + rng.Float64()*0.05
			s.gpuUtil = 93 + rng.Float64()*5
			s.gpuMemPct = 88 + rng.Float64()*2

			n := s.completeRequests(dt, 12, 0.11, rng)
			for i := 0; i < n; i++ {
				queue := lognormal(rng, 3.0, 0.4)
				s.observeQueue(queue)
				s.observeTTFT(queue + lognormal(rng, 0.25, 0.3))
				s.observeE2E(queue + lognormal(rng, 6, 0.4))
			}
			s.addTokens(dt, 700, 1800, rng)
			s.addPrefixCache(dt, 1800, 0.55, rng)
		},
	}
}

// batchingMisconfigured is the mirror image of capacityBound and the reason
// single-signal queue alerts are not actionable: identical queue depth, opposite
// fix.
func batchingMisconfigured() Scenario {
	return Scenario{
		Name:        "embeddings-bge",
		Model:       "BAAI/bge-large-en-v1.5",
		Description: "Same queue depth as the saturated workload, but the GPU is idle — an admission limit, not a shortage.",
		Expect:      "IFA-CAP-002",
		Replicas:    2, MaxReplicas: 6,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 2
			s.waiting = 24 + rng.Float64()*6
			s.waitingCapacity = s.waiting
			s.kvCache = 0.08 + rng.Float64()*0.03
			s.gpuUtil = 11 + rng.Float64()*6
			s.gpuMemPct = 34 + rng.Float64()*2

			n := s.completeRequests(dt, 9, 0.005, rng)
			for i := 0; i < n; i++ {
				queue := lognormal(rng, 2.2, 0.3)
				s.observeQueue(queue)
				s.observeTTFT(queue + lognormal(rng, 0.05, 0.3))
				s.observeE2E(queue + lognormal(rng, 0.4, 0.3))
			}
			s.addTokens(dt, 60, 900, rng)
			s.addPrefixCache(dt, 900, 0.4, rng)
		},
	}
}

// prefillBound has a slow TTFT that adding replicas would not fix, plus the
// prefix-cache miss rate that explains it.
func prefillBound() Scenario {
	return Scenario{
		Name:        "rag-llama3-70b",
		Model:       "meta-llama/Llama-3.1-70B-Instruct",
		Description: "Retrieval-augmented traffic: prompts are long, prefix caching is missing, TTFT is prefill-bound.",
		Expect:      "IFA-LAT-002",
		Replicas:    3, MaxReplicas: 9,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 6 + rng.Float64()*2
			s.waiting = rng.Float64() * 2
			s.waitingCapacity = s.waiting
			s.kvCache = 0.6 + rng.Float64()*0.08
			s.gpuUtil = 90 + rng.Float64()*6
			s.gpuMemPct = 82 + rng.Float64()*2

			n := s.completeRequests(dt, 3, 0.006, rng)
			for i := 0; i < n; i++ {
				// Admitted immediately, then a long prefill.
				s.observeQueue(lognormal(rng, 0.02, 0.4))
				s.observeTTFT(lognormal(rng, 4.5, 0.35))
				s.observeE2E(lognormal(rng, 12, 0.35))
			}
			// High prompt-token throughput with very few generated tokens is
			// what a prefill-dominated workload looks like.
			s.addTokens(dt, 90, 26000, rng)
			s.addPrefixCache(dt, 26000, 0.02, rng)
		},
	}
}

// autoscaleCeiling is queueing that no autoscaler can fix.
func autoscaleCeiling() Scenario {
	return Scenario{
		Name:        "batch-scoring",
		Model:       "meta-llama/Llama-3.1-8B-Instruct",
		Description: "Queueing while already at the HPA's maximum replica count.",
		Expect:      "IFA-SCL-001",
		Replicas:    6, MaxReplicas: 6,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 24 + rng.Float64()*3
			s.waiting = 31 + rng.Float64()*5
			s.waitingCapacity = s.waiting
			s.kvCache = 0.72 + rng.Float64()*0.06
			s.gpuUtil = 91 + rng.Float64()*6
			s.gpuMemPct = 85 + rng.Float64()*2

			n := s.completeRequests(dt, 16, 0.008, rng)
			for i := 0; i < n; i++ {
				queue := lognormal(rng, 1.8, 0.3)
				s.observeQueue(queue)
				s.observeTTFT(queue + lognormal(rng, 0.2, 0.3))
				s.observeE2E(queue + lognormal(rng, 5, 0.3))
			}
			s.addTokens(dt, 1400, 2200, rng)
			s.addPrefixCache(dt, 2200, 0.5, rng)
		},
	}
}

// partiallyInstrumented models the most common real-world integration problem:
// a runtime that exposes some of what IFA expects and none of the rest.
func partiallyInstrumented() Scenario {
	return Scenario{
		Name:          "legacy-serving",
		Model:         "internal/legacy-7b",
		Description:   "A runtime exposing only scheduler state — IFA reports reduced coverage rather than health.",
		Expect:        "IFA-OBS-001",
		SparseMetrics: true,
		advance: func(s *engineState, dt time.Duration, rng *rand.Rand) {
			s.running = 3 + rng.Float64()*2
			s.waiting = rng.Float64()
			s.waitingCapacity = s.waiting
		},
	}
}

// lognormal draws a positive latency with the given median and shape. Request
// latencies are right-skewed; a normal distribution would produce a p99 barely
// above the p95 and hide the tail behaviour the rules look for.
func lognormal(rng *rand.Rand, median, sigma float64) float64 {
	return median * math.Exp(rng.NormFloat64()*sigma)
}
