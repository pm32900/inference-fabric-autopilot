package telemetry

import (
	"context"
	"testing"
	"time"
)

func snap(name string) Snapshot {
	return Snapshot{
		Timestamp:    time.Now(),
		WorkloadName: name,
		ClusterName:  "test",
		Namespace:    "inference",
	}
}

func TestStore_AddAndLatest(t *testing.T) {
	store := NewStore()
	store.Add(snap("a"))
	store.Add(snap("b"))

	latest := store.Latest()
	if len(latest) != 2 {
		t.Errorf("expected 2 workloads in Latest(), got %d", len(latest))
	}
}

func TestStore_LatestReturnsOnlyMostRecent(t *testing.T) {
	store := NewStore()
	for i := 0; i < 5; i++ {
		s := snap("w")
		s.P95LatencyMs = float64(i * 100)
		store.Add(s)
	}

	latest := store.Latest()
	if len(latest) != 1 {
		t.Fatalf("expected 1 entry in Latest(), got %d", len(latest))
	}
	if latest[0].P95LatencyMs != 400 {
		t.Errorf("expected most recent snapshot (p95=400), got %.0f", latest[0].P95LatencyMs)
	}
}

func TestStore_RingBufferTrimming(t *testing.T) {
	store := NewStore()
	for i := 0; i < maxSnapshotsPerWorkload+5; i++ {
		store.Add(snap("w"))
	}

	store.mu.RLock()
	count := len(store.data["w"])
	store.mu.RUnlock()

	if count != maxSnapshotsPerWorkload {
		t.Errorf("expected ring buffer to trim to %d, got %d", maxSnapshotsPerWorkload, count)
	}
}

func TestStore_All(t *testing.T) {
	store := NewStore()
	store.Add(snap("x"))
	store.Add(snap("x"))
	store.Add(snap("y"))

	all := store.All()
	if len(all) != 3 {
		t.Errorf("expected 3 snapshots from All(), got %d", len(all))
	}
}

func TestStore_QueryRecent_InMemoryFallback(t *testing.T) {
	store := NewStore()
	for i := 0; i < 8; i++ {
		s := snap("w")
		s.P95LatencyMs = float64(i)
		store.Add(s)
	}

	ctx := context.Background()
	results, err := store.QueryRecent(ctx, "w", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}
}

func TestStore_QueryRecent_UnknownWorkload(t *testing.T) {
	store := NewStore()
	results, err := store.QueryRecent(context.Background(), "nonexistent", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for unknown workload, got %d", len(results))
	}
}

func TestStore_ConcurrentWrites(t *testing.T) {
	store := NewStore()
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func(n int) {
			for j := 0; j < 20; j++ {
				store.Add(snap("concurrent"))
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	latest := store.Latest()
	if len(latest) != 1 {
		t.Errorf("expected 1 workload entry after concurrent writes, got %d", len(latest))
	}
}
