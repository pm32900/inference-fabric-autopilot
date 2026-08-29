package recommender

import (
	"encoding/json"
	"fmt"
	"time"
)

// Thresholds are the tunable inputs to the rule set.
//
// Every field carries its unit in the name. The defaults are starting points
// for interactive serving workloads, not universal truths — a batch summarisation
// service and a chat endpoint disagree about what a bad TTFT is — so each is
// documented with the reasoning behind the number and is expected to be tuned.
type Thresholds struct {
	// SustainFor is how long a condition must hold before a rule fires.
	// Single-scrape thresholds are the main source of false positives in
	// inference monitoring: queues spike for one scheduler step all the time.
	// Rules that use it say so in their evidence.
	SustainFor time.Duration `yaml:"sustain_for"`

	// StaleAfter is how old the newest snapshot may be before the workload is
	// reported as unobserved rather than healthy.
	StaleAfter time.Duration `yaml:"stale_after"`

	// QueueWaitingRequests is the number of admitted-but-not-running requests
	// above which the scheduler is considered backed up.
	QueueWaitingRequests float64 `yaml:"queue_waiting_requests"`

	// GPUUtilHighPct and GPUUtilLowPct bracket "the accelerator is busy" and
	// "the accelerator is idle". The gap between them is deliberate: workloads
	// in between are neither saturated nor obviously misconfigured, and firing
	// on them produces advice nobody can act on.
	GPUUtilHighPct float64 `yaml:"gpu_util_high_pct"`
	GPUUtilLowPct  float64 `yaml:"gpu_util_low_pct"`

	GPUMemoryHighPct float64 `yaml:"gpu_memory_high_pct"`

	// KVCacheHighPct is where KV-cache occupancy becomes worth mentioning.
	// High occupancy on its own is not a fault — vLLM is designed to use the
	// cache it was given — which is why the rule that reads it is a warning
	// and is superseded by the preemption rule.
	KVCacheHighPct float64 `yaml:"kv_cache_high_pct"`

	// PreemptionsPerSec above which the runtime is discarding and recomputing
	// work often enough to matter. The default is deliberately just above zero:
	// preemption is not a normal steady state.
	PreemptionsPerSec float64 `yaml:"preemptions_per_sec"`

	TTFTP95Ms float64 `yaml:"ttft_p95_ms"`
	E2EP95Ms  float64 `yaml:"e2e_p95_ms"`

	// QueueShareOfTTFTPct splits a slow TTFT into "waiting to be admitted" and
	// "slow to prefill". Above this share the wait dominates, and the fix is
	// capacity; below it the fix is request shape or prefill configuration.
	QueueShareOfTTFTPct float64 `yaml:"queue_share_of_ttft_pct"`

	// TailRatioP99P95 is how much worse p99 may be than p95 before a small
	// number of outlier requests is worth calling out.
	TailRatioP99P95 float64 `yaml:"tail_ratio_p99_p95"`

	TokensPerSecondLow float64 `yaml:"tokens_per_second_low"`

	// PrefixCacheHitLowPct is the hit rate below which prefix caching has
	// stopped paying for itself, usually because requests that share a prefix
	// are being spread across replicas.
	PrefixCacheHitLowPct float64 `yaml:"prefix_cache_hit_low_pct"`

	ErrorRatePct float64 `yaml:"error_rate_pct"`
	AbortRatePct float64 `yaml:"abort_rate_pct"`
}

// DefaultThresholds returns the built-in defaults.
func DefaultThresholds() Thresholds {
	return Thresholds{
		SustainFor:           45 * time.Second,
		StaleAfter:           2 * time.Minute,
		QueueWaitingRequests: 8,
		GPUUtilHighPct:       85,
		GPUUtilLowPct:        35,
		GPUMemoryHighPct:     90,
		KVCacheHighPct:       90,
		PreemptionsPerSec:    0.01,
		TTFTP95Ms:            1000,
		E2EP95Ms:             10000,
		QueueShareOfTTFTPct:  50,
		TailRatioP99P95:      3,
		TokensPerSecondLow:   200,
		PrefixCacheHitLowPct: 10,
		ErrorRatePct:         2,
		AbortRatePct:         5,
	}
}

// MarshalJSON renders thresholds with the same snake_case names the
// configuration file uses, and durations as durations.
//
// The default struct encoding would emit Go field names and nanosecond
// integers, which nobody reading /api/v1/rules can match against their
// config.yaml.
func (t Thresholds) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"sustain_for":              t.SustainFor.String(),
		"stale_after":              t.StaleAfter.String(),
		"queue_waiting_requests":   t.QueueWaitingRequests,
		"gpu_util_high_pct":        t.GPUUtilHighPct,
		"gpu_util_low_pct":         t.GPUUtilLowPct,
		"gpu_memory_high_pct":      t.GPUMemoryHighPct,
		"kv_cache_high_pct":        t.KVCacheHighPct,
		"preemptions_per_sec":      t.PreemptionsPerSec,
		"ttft_p95_ms":              t.TTFTP95Ms,
		"e2e_p95_ms":               t.E2EP95Ms,
		"queue_share_of_ttft_pct":  t.QueueShareOfTTFTPct,
		"tail_ratio_p99_p95":       t.TailRatioP99P95,
		"tokens_per_second_low":    t.TokensPerSecondLow,
		"prefix_cache_hit_low_pct": t.PrefixCacheHitLowPct,
		"error_rate_pct":           t.ErrorRatePct,
		"abort_rate_pct":           t.AbortRatePct,
	})
}

// Validate rejects configurations that would make rules unfireable or
// nonsensical. It is called at startup so a bad value is a refusal to start
// rather than silence at 3am.
func (t Thresholds) Validate() error {
	if t.SustainFor < 0 {
		return fmt.Errorf("recommender: sustain_for must not be negative, got %s", t.SustainFor)
	}
	if t.StaleAfter <= 0 {
		return fmt.Errorf("recommender: stale_after must be positive, got %s", t.StaleAfter)
	}
	if t.GPUUtilLowPct >= t.GPUUtilHighPct {
		return fmt.Errorf(
			"recommender: gpu_util_low_pct (%.1f) must be below gpu_util_high_pct (%.1f); "+
				"otherwise the idle-GPU and saturated-GPU rules overlap and both fire on the same workload",
			t.GPUUtilLowPct, t.GPUUtilHighPct)
	}
	percentages := map[string]float64{
		"gpu_util_high_pct":        t.GPUUtilHighPct,
		"gpu_util_low_pct":         t.GPUUtilLowPct,
		"gpu_memory_high_pct":      t.GPUMemoryHighPct,
		"kv_cache_high_pct":        t.KVCacheHighPct,
		"queue_share_of_ttft_pct":  t.QueueShareOfTTFTPct,
		"prefix_cache_hit_low_pct": t.PrefixCacheHitLowPct,
		"error_rate_pct":           t.ErrorRatePct,
		"abort_rate_pct":           t.AbortRatePct,
	}
	for name, v := range percentages {
		if v < 0 || v > 100 {
			return fmt.Errorf("recommender: %s must be between 0 and 100, got %.2f", name, v)
		}
	}
	nonNegative := map[string]float64{
		"queue_waiting_requests": t.QueueWaitingRequests,
		"preemptions_per_sec":    t.PreemptionsPerSec,
		"ttft_p95_ms":            t.TTFTP95Ms,
		"e2e_p95_ms":             t.E2EP95Ms,
		"tokens_per_second_low":  t.TokensPerSecondLow,
	}
	for name, v := range nonNegative {
		if v < 0 {
			return fmt.Errorf("recommender: %s must not be negative, got %.2f", name, v)
		}
	}
	if t.TailRatioP99P95 < 1 {
		return fmt.Errorf(
			"recommender: tail_ratio_p99_p95 must be at least 1 (p99 cannot be below p95), got %.2f",
			t.TailRatioP99P95)
	}
	return nil
}
