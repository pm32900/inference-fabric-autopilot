package telemetry

import (
	"sort"
	"sync"
	"time"
)

// Store holds recent telemetry in memory, keyed by workload.
//
// Retention is bounded twice over — by sample count and by age — because the
// two failure modes are different. A count bound protects against a
// fast-scraping deployment exhausting memory; an age bound stops a workload
// that stopped reporting from keeping stale samples alive and letting rules
// diagnose a workload that no longer exists.
//
// The in-memory window is what the rule engine reads. Long-term storage, when
// configured, is a separate concern handled by a Sink.
type Store struct {
	mu   sync.RWMutex
	data map[string][]Snapshot

	maxPerWorkload int
	maxAge         time.Duration
	now            func() time.Time

	sink Sink
}

// Sink receives every snapshot for durable storage. Implementations must be
// safe for concurrent use and must not block: the store calls Write on the
// caller's goroutine and a slow sink would stall collection.
type Sink interface {
	Write(Snapshot)
}

// StoreOption configures a Store.
type StoreOption func(*Store)

// WithRetention sets the in-memory window. A non-positive count or age leaves
// the corresponding bound at its default.
func WithRetention(maxPerWorkload int, maxAge time.Duration) StoreOption {
	return func(s *Store) {
		if maxPerWorkload > 0 {
			s.maxPerWorkload = maxPerWorkload
		}
		if maxAge > 0 {
			s.maxAge = maxAge
		}
	}
}

// WithSink attaches a durable sink.
func WithSink(sink Sink) StoreOption {
	return func(s *Store) { s.sink = sink }
}

// withClock is used by tests to make retention deterministic.
func withClock(now func() time.Time) StoreOption {
	return func(s *Store) { s.now = now }
}

// Default retention: enough history for the rule engine's longest evaluation
// window with headroom, and no more.
const (
	defaultMaxPerWorkload = 120
	defaultMaxAge         = 15 * time.Minute
)

// NewStore returns an empty Store.
func NewStore(opts ...StoreOption) *Store {
	s := &Store{
		data:           make(map[string][]Snapshot),
		maxPerWorkload: defaultMaxPerWorkload,
		maxAge:         defaultMaxAge,
		now:            time.Now,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Add records a snapshot and forwards it to the sink, if any.
func (s *Store) Add(snap Snapshot) {
	key := snap.Key()

	s.mu.Lock()
	series := append(s.data[key], snap)
	cutoff := s.now().Add(-s.maxAge)
	// Samples arrive in time order, so the first one inside the window marks
	// the start of what to keep.
	drop := 0
	for drop < len(series) && series[drop].Timestamp.Before(cutoff) {
		drop++
	}
	series = series[drop:]
	if len(series) > s.maxPerWorkload {
		series = series[len(series)-s.maxPerWorkload:]
	}
	// Re-slice into a fresh backing array periodically so the trimmed prefix
	// can be collected rather than pinned by the slice header.
	if cap(series) > 4*s.maxPerWorkload {
		compact := make([]Snapshot, len(series))
		copy(compact, series)
		series = compact
	}
	s.data[key] = series
	s.mu.Unlock()

	if s.sink != nil {
		s.sink.Write(snap)
	}
}

// Latest returns the most recent snapshot for each workload, ordered by
// workload key. The ordering is deterministic so that recommendation IDs and
// API responses do not depend on Go's map iteration order.
func (s *Store) Latest() []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Snapshot, 0, len(s.data))
	for _, series := range s.data {
		if len(series) > 0 {
			out = append(out, series[len(series)-1])
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

// History returns every retained snapshot for one workload, oldest first. It is
// what the rule engine evaluates over; retention bounds its size.
func (s *Store) History(key string) []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	series := s.data[key]
	if len(series) == 0 {
		return nil
	}
	out := make([]Snapshot, len(series))
	copy(out, series)
	return out
}

// All returns every retained snapshot, ordered by workload then time.
func (s *Store) All() []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []Snapshot
	for _, k := range keys {
		out = append(out, s.data[k]...)
	}
	return out
}

// Size returns the total number of retained snapshots. It backs the
// ifa_store_snapshots gauge.
func (s *Store) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, series := range s.data {
		n += len(series)
	}
	return n
}

// Workloads returns the keys of every workload with retained telemetry.
func (s *Store) Workloads() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Prune drops workloads whose newest sample is older than the retention window.
// The collector calls it periodically so that a workload that is deleted from
// the cluster eventually disappears from the API instead of lingering forever.
func (s *Store) Prune() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := s.now().Add(-s.maxAge)
	removed := 0
	for k, series := range s.data {
		if len(series) == 0 || series[len(series)-1].Timestamp.Before(cutoff) {
			delete(s.data, k)
			removed++
		}
	}
	return removed
}
