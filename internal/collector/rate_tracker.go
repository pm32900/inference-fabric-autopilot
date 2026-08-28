package collector

import (
	"sync"
	"time"
)

// RateTracker computes per-second rates for monotonically increasing counters.
// It stores the previous value and timestamp for each named counter per workload,
// and returns the delta-over-time on each call to Rate().
//
// Thread-safe. Safe to call from the scrape goroutine without external locking.
type RateTracker struct {
	mu      sync.Mutex
	entries map[string]*rateEntry // key: workloadName + ":" + metricName
}

type rateEntry struct {
	lastValue float64
	lastTime  time.Time
}

// NewRateTracker creates an empty RateTracker.
func NewRateTracker() *RateTracker {
	return &RateTracker{
		entries: make(map[string]*rateEntry),
	}
}

// Rate returns the per-second rate for the counter identified by (workload, metric).
// On the first call for a given key, it records the baseline and returns 0.
// On subsequent calls it returns (delta / elapsed_seconds), clamped to >= 0
// to handle counter resets.
func (r *RateTracker) Rate(workload, metric string, currentValue float64, now time.Time) float64 {
	key := workload + ":" + metric

	r.mu.Lock()
	defer r.mu.Unlock()

	entry, exists := r.entries[key]
	if !exists {
		r.entries[key] = &rateEntry{lastValue: currentValue, lastTime: now}
		return 0
	}

	elapsed := now.Sub(entry.lastTime).Seconds()
	if elapsed <= 0 {
		return 0
	}

	delta := currentValue - entry.lastValue

	// Update baseline regardless of direction — handles counter resets gracefully
	entry.lastValue = currentValue
	entry.lastTime = now

	if delta < 0 {
		// Counter reset — return 0 for this interval, next call will be clean
		return 0
	}

	return delta / elapsed
}
