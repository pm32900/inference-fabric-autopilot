package collector

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pm32900/inference-fabric-autopilot/internal/runtime/dcgm"
	"github.com/pm32900/inference-fabric-autopilot/internal/runtime/triton"
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
	DCGMUrl      string // optional — DCGM Exporter endpoint e.g. "http://dcgm-exporter:9400/metrics"
}

// PrometheusCollector scrapes one or more targets and writes snapshots to the store.
type PrometheusCollector struct {
	targets     []PrometheusTarget
	store       *telemetry.Store
	client      *http.Client
	interval    time.Duration
	rateTracker *RateTracker
}

// NewPrometheusCollector creates a collector for the given targets.
func NewPrometheusCollector(targets []PrometheusTarget, store *telemetry.Store, interval time.Duration) *PrometheusCollector {
	return &PrometheusCollector{
		targets:     targets,
		store:       store,
		interval:    interval,
		client:      &http.Client{Timeout: 10 * time.Second},
		rateTracker: NewRateTracker(),
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
// For vLLM targets it delegates metric extraction to vllm.Parse().
// If DCGMUrl is set, it additionally scrapes the DCGM Exporter and populates
// real GPU utilization and memory fields, replacing any placeholder values.
func (c *PrometheusCollector) scrape(ctx context.Context, target PrometheusTarget) (*telemetry.Snapshot, error) {
	now := time.Now().UTC()

	body, err := c.fetch(ctx, target.MetricsURL)
	if err != nil {
		return nil, err
	}

	snap := &telemetry.Snapshot{
		Timestamp:    now,
		ClusterName:  "local-dev",
		Namespace:    target.Namespace,
		WorkloadName: target.WorkloadName,
		Runtime:      target.Runtime,
		ModelName:    target.ModelName,
	}

	if target.Runtime == "vllm" {
		v := vllm.Parse(string(body))

		// Latency and queue
		snap.P50LatencyMs = v.E2ELatencyP50Ms
		snap.P95LatencyMs = v.E2ELatencyP95Ms
		snap.P99LatencyMs = v.E2ELatencyP99Ms
		snap.NumRequestsRunning = v.NumRequestsRunning
		snap.NumRequestsWaiting = v.NumRequestsWaiting
		snap.KVCacheUsagePct = v.KVCacheUsagePct
		snap.TTFTP95Ms = v.TTFTP95Ms
		snap.QueueDepth = v.NumRequestsWaiting

		// Counter-derived rates
		snap.TokensPerSecond = c.rateTracker.Rate(
			target.WorkloadName, "generation_tokens_total",
			v.GenerationTokensTotal, now,
		)
		snap.RequestRatePerSec = c.rateTracker.Rate(
			target.WorkloadName, "request_success_total",
			v.RequestSuccessTotal, now,
		)
		failRate := c.rateTracker.Rate(
			target.WorkloadName, "request_failure_total",
			v.RequestFailureTotal, now,
		)
		totalRate := snap.RequestRatePerSec + failRate
		if totalRate > 0 {
			snap.ErrorRatePct = (failRate / totalRate) * 100.0
		}

		// GPU fields: use KV cache as a proxy only when DCGM is not available.
		// When DCGMUrl is set these will be overwritten below with real values.
		if target.DCGMUrl == "" {
			snap.GPUUtilizationPct = v.KVCacheUsagePct
			snap.GPUMemoryUsedPct = v.KVCacheUsagePct
		}
	}

	if target.Runtime == "triton" {
		snaps := triton.Parse(string(body), target.ModelName)
		if len(snaps) == 0 {
			fmt.Printf("warn: no Triton metrics found for model %q in %s\n", target.ModelName, target.WorkloadName)
			return snap, nil
		}
		// Use the first (and typically only) matching model snapshot
		t := snaps[0]

		// GPU — Triton reports these natively; DCGM will overwrite if also configured
		snap.GPUUtilizationPct = t.GPUUtilizationPct
		snap.GPUMemoryUsedPct = t.GPUMemoryUsedPct

		// Queue depth
		snap.QueueDepth = t.PendingRequestCount

		// Counter-derived rates
		snap.RequestRatePerSec = c.rateTracker.Rate(
			target.WorkloadName, "inference_success_total",
			t.InferenceSuccessTotal, now,
		)
		failRate := c.rateTracker.Rate(
			target.WorkloadName, "inference_failure_total",
			t.InferenceFailureTotal, now,
		)
		totalRate := snap.RequestRatePerSec + failRate
		if totalRate > 0 {
			snap.ErrorRatePct = (failRate / totalRate) * 100.0
		}

		// Derive average request latency from cumulative counters.
		// RequestDurationUsTotal / InferenceSuccessTotal gives avg microseconds per request.
		reqDurRate := c.rateTracker.Rate(
			target.WorkloadName, "request_duration_us_total",
			t.RequestDurationUsTotal, now,
		)
		if snap.RequestRatePerSec > 0 {
			avgLatencyMs := (reqDurRate / snap.RequestRatePerSec) / 1000.0
			snap.P95LatencyMs = avgLatencyMs
			snap.P50LatencyMs = avgLatencyMs
		}
	}

	// DCGM scrape — overwrites GPU fields with real hardware readings.
	// Aggregation strategy: take the maximum across all GPUs so that a single
	// saturated GPU surfaces as a recommendation even in a multi-GPU pod.
	if target.DCGMUrl != "" {
		dcgmBody, dcgmErr := c.fetch(ctx, target.DCGMUrl)
		if dcgmErr != nil {
			fmt.Printf("warn: DCGM scrape failed for %s: %v — using proxy values\n", target.WorkloadName, dcgmErr)
		} else {
			gpus := dcgm.Parse(string(dcgmBody))
			if len(gpus) > 0 {
				var maxUtil, maxMemPct float64
				for _, g := range gpus {
					if g.GPUUtilPct > maxUtil {
						maxUtil = g.GPUUtilPct
					}
					if g.FBUsedPct > maxMemPct {
						maxMemPct = g.FBUsedPct
					}
				}
				snap.GPUUtilizationPct = maxUtil
				snap.GPUMemoryUsedPct = maxMemPct
			}
		}
	}

	return snap, nil
}

// fetch performs a GET request and returns the response body as a byte slice.
func (c *PrometheusCollector) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", url, err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", url, err)
	}

	return body, nil
}
