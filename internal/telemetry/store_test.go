package telemetry

import (
	"math"
	"sync"
	"testing"
	"time"
)

var base = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func snapAt(ns, name string, at time.Time) Snapshot {
	return Snapshot{Timestamp: at, Namespace: ns, WorkloadName: name}
}

// newStore builds a Store whose clock is pinned to the fixture epoch, so that
// age-based retention does not silently discard the test's own samples.
func newStore(t *testing.T, opts ...StoreOption) *Store {
	t.Helper()
	return NewStore(append([]StoreOption{withClock(func() time.Time { return base })}, opts...)...)
}

func TestLatestIsOnePerWorkloadAndOrdered(t *testing.T) {
	s := newStore(t)
	s.Add(snapAt("inference", "b", base))
	s.Add(snapAt("inference", "a", base))
	s.Add(snapAt("inference", "a", base.Add(time.Second)))

	got := s.Latest()
	if len(got) != 2 {
		t.Fatalf("got %d workloads, want 2", len(got))
	}
	if got[0].Key() != "inference/a" || got[1].Key() != "inference/b" {
		t.Errorf("order = %q, %q; want inference/a, inference/b", got[0].Key(), got[1].Key())
	}
	if !got[0].Timestamp.Equal(base.Add(time.Second)) {
		t.Error("Latest did not return the newest snapshot for a workload")
	}
}

// Workload names are only unique within a namespace; keying on name alone
// silently merges two different deployments into one set of recommendations.
func TestSameNameInDifferentNamespacesStaySeparate(t *testing.T) {
	s := newStore(t)
	s.Add(snapAt("team-a", "llm", base))
	s.Add(snapAt("team-b", "llm", base))

	if got := len(s.Latest()); got != 2 {
		t.Fatalf("got %d workloads, want 2 — namespaces were collapsed", got)
	}
}

func TestRetentionByCount(t *testing.T) {
	s := NewStore(WithRetention(3, time.Hour), withClock(func() time.Time { return base }))
	for i := 0; i < 10; i++ {
		s.Add(snapAt("ns", "w", base.Add(time.Duration(i)*time.Second)))
	}

	all := s.All()
	if len(all) != 3 {
		t.Fatalf("retained %d snapshots, want 3", len(all))
	}
	if !all[0].Timestamp.Equal(base.Add(7 * time.Second)) {
		t.Errorf("oldest retained = %v, want the 8th sample", all[0].Timestamp)
	}
	if s.Size() != 3 {
		t.Errorf("Size = %d, want 3", s.Size())
	}
}

func TestRetentionByAge(t *testing.T) {
	now := base
	s := NewStore(WithRetention(1000, 30*time.Second), withClock(func() time.Time { return now }))

	s.Add(snapAt("ns", "w", base.Add(-5*time.Minute))) // well outside the window
	s.Add(snapAt("ns", "w", base.Add(-10*time.Second)))
	s.Add(snapAt("ns", "w", base))

	if got := len(s.All()); got != 2 {
		t.Fatalf("retained %d snapshots, want 2 — the stale one should have aged out", got)
	}
}

// History is what the rule engine evaluates over: the retained samples for one
// workload, oldest first, and nothing for a workload that has none.
func TestHistoryIsOrderedAndScopedToOneWorkload(t *testing.T) {
	s := newStore(t, WithRetention(100, time.Hour))
	for i := 0; i < 6; i++ {
		s.Add(snapAt("ns", "w", base.Add(time.Duration(i)*10*time.Second)))
	}
	s.Add(snapAt("ns", "other", base))

	got := s.History("ns/w")
	if len(got) != 6 {
		t.Fatalf("history returned %d samples, want 6", len(got))
	}
	if !got[0].Timestamp.Equal(base) || !got[5].Timestamp.Equal(base.Add(50*time.Second)) {
		t.Error("history is not ordered oldest first")
	}

	if got := s.History("ns/missing"); got != nil {
		t.Errorf("history for an unknown workload = %v, want nil", got)
	}

	// The returned slice must not alias the store's own backing array, or a
	// caller could mutate retained telemetry.
	got[0].WorkloadName = "mutated"
	if s.History("ns/w")[0].WorkloadName != "w" {
		t.Error("History returned a slice aliasing the store's data")
	}
}

// A workload whose scrape target has gone away must not keep a stale window
// alive: a rule reading it would diagnose a deployment that no longer exists.
func TestPruneRemovesWorkloadsThatStoppedReporting(t *testing.T) {
	now := base
	s := NewStore(WithRetention(100, time.Minute), withClock(func() time.Time { return now }))
	s.Add(snapAt("ns", "live", base))
	s.Add(snapAt("ns", "gone", base))

	now = base.Add(5 * time.Minute)
	s.Add(snapAt("ns", "live", now))

	if removed := s.Prune(); removed != 1 {
		t.Fatalf("pruned %d workloads, want 1", removed)
	}
	if got := s.Workloads(); len(got) != 1 || got[0] != "ns/live" {
		t.Errorf("remaining workloads = %v, want [ns/live]", got)
	}
}

type countingSink struct {
	mu sync.Mutex
	n  int
}

func (c *countingSink) Write(Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.n++
}

func TestSinkReceivesEverySnapshot(t *testing.T) {
	sink := &countingSink{}
	s := newStore(t, WithRetention(2, time.Hour), WithSink(sink))

	for i := 0; i < 5; i++ {
		s.Add(snapAt("ns", "w", base.Add(time.Duration(i)*time.Second)))
	}

	sink.mu.Lock()
	defer sink.mu.Unlock()
	// In-memory retention must not decide what reaches durable storage.
	if sink.n != 5 {
		t.Errorf("sink saw %d snapshots, want 5", sink.n)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := newStore(t, WithRetention(50, time.Hour))
	var wg sync.WaitGroup

	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.Add(snapAt("ns", string(rune('a'+w)), base.Add(time.Duration(i)*time.Millisecond)))
			}
		}(w)
	}
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = s.Latest()
				_ = s.Size()
				_ = s.History("ns/a")
			}
		}()
	}
	wg.Wait()

	if got := len(s.Workloads()); got != 8 {
		t.Errorf("got %d workloads, want 8", got)
	}
}

func TestMetricJSONRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   Metric
		want string
	}{
		{"measured", Observed(12.5), "12.5"},
		{"measured zero", Observed(0), "0"},
		{"unmeasured", Metric{}, "null"},
		{"NaN is unmeasured", Observed(nan()), "null"},
		{"Inf is unmeasured", Observed(inf()), "null"},
		{"ObservedIf false", ObservedIf(99, false), "null"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := tc.in.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.want {
				t.Fatalf("marshalled to %s, want %s", b, tc.want)
			}
			var back Metric
			if err := back.UnmarshalJSON(b); err != nil {
				t.Fatal(err)
			}
			if back.OK != tc.in.OK || (back.OK && back.Value != tc.in.Value) {
				t.Errorf("round trip changed %+v to %+v", tc.in, back)
			}
		})
	}
}

func TestMetricComparisonsIgnoreUnmeasured(t *testing.T) {
	var absent Metric
	if absent.Above(-1) {
		t.Error("an unmeasured metric compared above a threshold")
	}
	if absent.Below(1e9) {
		t.Error("an unmeasured metric compared below a threshold")
	}
	if got := absent.Or(7); got != 7 {
		t.Errorf("Or = %v, want the default 7", got)
	}
	if got := absent.String(); got != "-" {
		t.Errorf("String = %q, want %q", got, "-")
	}
	if !Observed(5).Above(4) || Observed(5).Above(5) {
		t.Error("Above is not a strict comparison on measured values")
	}
}

func nan() float64 { return math.NaN() }
func inf() float64 { return math.Inf(1) }
