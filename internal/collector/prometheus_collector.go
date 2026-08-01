package collector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/runtime/vllm"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

// PrometheusTarget defines one scrape target — a running inference workload
// that exposes a /metrics endpoint in Prometheus text format.
type PrometheusTarget struct {
	WorkloadName string
	Namespace    string
	Runtime      string
	ModelName    string
	MetricsURL   string // e.g. "http://llm-serving:8000/metrics"
}

// PrometheusCollector scrapes one or more targets and writes snapshots to the store.
type PrometheusCollector struct {
	targets  []PrometheusTarget
	store    *telemetry.Store
	client   *http.Client
	interval time.Duration
}

// NewPrometheusCollector creates a collector for the given targets.
func NewPrometheusCollector(targets []PrometheusTarget, store *telemetry.Store, interval time.Duration) *PrometheusCollector {
	return &PrometheusCollector{
		targets:  targets,
		store:    store,
		interval: interval,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Start launches the scrape loop in a background goroutine.
func (c *PrometheusCollector) Start(ctx context.Context) {
	go func() {
		for {
			for _, target := range c.targets {
				snap, err := c.scrape(ctx, target)
				if err != nil {
					fmt.Printf("warn: scrape failed for %s: %v\n", target.WorkloadName, err)
					continue
				}
				c.store.Add(*snap)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(c.interval):
			}
		}
	}()
}

// scrape fetches and parses the /metrics endpoint for one target.
// For vLLM targets it delegates all metric extraction to vllm.Parse().
func (c *PrometheusCollector) scrape(ctx context.Context, target PrometheusTarget) (*telemetry.Snapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.MetricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, target.MetricsURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading metrics body: %w", err)
	}

	snap := &telemetry.Snapshot{
		Timestamp:    time.Now().UTC(),
		ClusterName:  "local-dev",
		Namespace:    target.Namespace,
		WorkloadName: target.WorkloadName,
		Runtime:      target.Runtime,
		ModelName:    target.ModelName,
	}

	if target.Runtime == "vllm" {
		v := vllm.Parse(string(body))

		// Shared telemetry fields — mapped from vLLM-native signals
		snap.RequestRatePerSec = v.RequestSuccessTotal
		snap.P50LatencyMs = v.E2ELatencyP50Ms
		snap.P95LatencyMs = v.E2ELatencyP95Ms
		snap.P99LatencyMs = v.E2ELatencyP99Ms
		snap.TokensPerSecond = v.GenerationTokensTotal
		snap.ErrorRatePct = v.RequestFailureTotal
		snap.GPUUtilizationPct = v.KVCacheUsagePct
		snap.GPUMemoryUsedPct = v.KVCacheUsagePct

		// vLLM-native fields
		snap.NumRequestsRunning = v.NumRequestsRunning
		snap.NumRequestsWaiting = v.NumRequestsWaiting
		snap.KVCacheUsagePct = v.KVCacheUsagePct
		snap.TTFTP95Ms = v.TTFTP95Ms

		// Mirror waiting count into shared QueueDepth so generic rules fire correctly
		snap.QueueDepth = v.NumRequestsWaiting
	}

	return snap, nil
}
