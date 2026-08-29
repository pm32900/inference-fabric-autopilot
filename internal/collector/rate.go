package collector

import (
	"sync"
	"time"
)

// rateTracker converts cumulative counters into per-second rates by
// differencing successive scrapes.
//
// Two cases have to be distinguished from a genuine zero, because both of them
// otherwise produce a rate of 0 that reads as "this workload is doing nothing":
//
//   - The first observation of a counter. There is no previous value to
//     difference against.
//   - A counter reset, which happens whenever the serving process restarts.
//     The delta goes negative; reporting 0 for that interval would make a pod
//     that just came back look idle, and would fire the low-throughput rule at
//     exactly the moment an operator is already dealing with a restart.
//
// In both cases rate reports "not measured" and re-baselines, so the next
// interval is clean.
type rateTracker struct {
	mu      sync.Mutex
	entries map[string]rateEntry
}

type rateEntry struct {
	value float64
	at    time.Time
}

func newRateTracker() *rateTracker {
	return &rateTracker{entries: make(map[string]rateEntry)}
}

// rate returns the per-second rate for the named counter, and whether a rate
// could be computed.
func (r *rateTracker) rate(key string, current float64, now time.Time) (float64, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	prev, seen := r.entries[key]
	r.entries[key] = rateEntry{value: current, at: now}

	if !seen {
		return 0, false
	}
	elapsed := now.Sub(prev.at).Seconds()
	if elapsed <= 0 {
		// Two scrapes with the same timestamp, or a clock that moved
		// backwards. Neither yields a meaningful rate.
		return 0, false
	}
	delta := current - prev.value
	if delta < 0 {
		return 0, false // counter reset; the new baseline is already stored
	}
	return delta / elapsed, true
}

// forget drops all state for a workload. The collector calls it when a target
// disappears so a returning workload starts from a clean baseline rather than
// differencing against a value from before it went away.
func (r *rateTracker) forget(prefix string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k := range r.entries {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(r.entries, k)
		}
	}
}
