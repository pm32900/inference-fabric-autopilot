package collector

import (
	"context"
	"fmt"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/runtime/ollama"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

// OllamaTarget defines one Ollama server to poll.
// A single Ollama server may serve multiple models simultaneously;
// the collector creates one telemetry.Snapshot per loaded model per poll cycle.
type OllamaTarget struct {
	WorkloadName string // logical name for this Ollama deployment
	Namespace    string
	BaseURL      string // e.g. "http://ollama:11434"
}

// OllamaCollector polls one or more Ollama servers and writes snapshots
// to the telemetry store. It uses the shared RateTracker to compute
// per-second rates from Ollama's cumulative counters.
type OllamaCollector struct {
	targets     []OllamaTarget
	store       *telemetry.Store
	interval    time.Duration
	rateTracker *RateTracker
}

// NewOllamaCollector creates an OllamaCollector for the given targets.
func NewOllamaCollector(targets []OllamaTarget, store *telemetry.Store, interval time.Duration) *OllamaCollector {
	return &OllamaCollector{
		targets:     targets,
		store:       store,
		interval:    interval,
		rateTracker: NewRateTracker(),
	}
}

// Start launches the poll loop in a background goroutine.
// It exits cleanly when ctx is cancelled.
func (c *OllamaCollector) Start(ctx context.Context) {
	go func() {
		for {
			for _, target := range c.targets {
				if err := c.poll(ctx, target); err != nil {
					fmt.Printf("warn: ollama poll failed for %s: %v\n", target.WorkloadName, err)
				}
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(c.interval):
			}
		}
	}()
}

// poll queries one Ollama server, iterates its loaded models, and writes
// one telemetry.Snapshot per model into the store.
func (c *OllamaCollector) poll(ctx context.Context, target OllamaTarget) error {
	now := time.Now().UTC()
	client := ollama.NewClient(target.BaseURL)

	models, err := client.ListRunning(ctx)
	if err != nil {
		return fmt.Errorf("listing running models: %w", err)
	}

	if len(models) == 0 {
		fmt.Printf("info: ollama target %s has no models loaded\n", target.WorkloadName)
		return nil
	}

	for _, model := range models {
		snap, err := client.Snapshot(ctx, model)
		if err != nil {
			fmt.Printf("warn: snapshot failed for model %s on %s: %v\n", model.Name, target.WorkloadName, err)
			continue
		}
		c.store.Add(c.toTelemetry(snap, target, now))
	}

	return nil
}

// toTelemetry converts an OllamaSnapshot into the shared telemetry.Snapshot format.
// Rates are computed from cumulative counter deltas using the RateTracker.
// GPU memory usage is estimated from SizeVRAM — Ollama does not expose total
// VRAM so GPUMemoryUsedPct is left as 0 (unknown) rather than a misleading value.
func (c *OllamaCollector) toTelemetry(snap ollama.OllamaSnapshot, target OllamaTarget, now time.Time) telemetry.Snapshot {
	// Use workload+model as the rate tracker key namespace so multiple
	// models on the same server are tracked independently.
	key := target.WorkloadName + "/" + snap.ModelName

	tokensPerSec := c.rateTracker.Rate(key, "eval_count", float64(snap.EvalCount), now)
	promptTokPerSec := c.rateTracker.Rate(key, "prompt_eval_count", float64(snap.PromptEvalCount), now)
	_ = promptTokPerSec // available for future rules; not in shared Snapshot yet

	// Derive average request latency in milliseconds from cumulative nanosecond counters.
	// total_duration / (eval_count + prompt_eval_count) gives avg ms per request.
	totalReqs := float64(snap.EvalCount + snap.PromptEvalCount)
	var avgLatencyMs float64
	if totalReqs > 0 && snap.TotalDurationNs > 0 {
		avgLatencyMs = (float64(snap.TotalDurationNs) / totalReqs) / 1e6
	}

	// Estimate TTFT from prompt eval duration per prompt token.
	// PromptEvalDurationNs / PromptEvalCount = ns per prompt token → convert to ms.
	var ttftMs float64
	if snap.PromptEvalCount > 0 && snap.PromptEvalDurationNs > 0 {
		ttftMs = (float64(snap.PromptEvalDurationNs) / float64(snap.PromptEvalCount)) / 1e6
	}

	return telemetry.Snapshot{
		Timestamp:    now,
		ClusterName:  "local-dev",
		Namespace:    target.Namespace,
		WorkloadName: target.WorkloadName + "/" + snap.ModelName,
		Runtime:      "ollama",
		ModelName:    snap.ModelName,

		TokensPerSecond:   tokensPerSec,
		P50LatencyMs:      avgLatencyMs,
		P95LatencyMs:      avgLatencyMs, // Ollama does not expose percentiles; avg is the best available
		TTFTP95Ms:         ttftMs,

		// GPU memory: SizeVRAM is bytes used; total is unknown without NVML/DCGM
		// so we report 0 to avoid a misleading percentage.
		GPUMemoryUsedPct:  0,
		GPUUtilizationPct: 0,
	}
}
