package telemetry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const maxSnapshotsPerWorkload = 10

// Store is a dual-backend telemetry store.
type Store struct {
	mu   sync.RWMutex
	data map[string][]Snapshot
	pool *pgxpool.Pool
}

func NewStore() *Store {
	return &Store{data: make(map[string][]Snapshot)}
}

// when database enabled is True, need to call newstorewithdb
func NewStoreWithDB(pool *pgxpool.Pool) *Store {
	return &Store{
		data: make(map[string][]Snapshot),
		pool: pool,
	}
}

// Add writes a snapshot to both the in memory buffer and TimescaleDB if enabled
func (s *Store) Add(snap Snapshot) {
	// always write to memory so Latest() is fast
	s.mu.Lock()
	key := snap.WorkloadName
	s.data[key] = append(s.data[key], snap)
	if len(s.data[key]) > maxSnapshotsPerWorkload {
		s.data[key] = s.data[key][len(s.data[key])-maxSnapshotsPerWorkload:]
	}
	s.mu.Unlock()

	// async write to DB
	if s.pool != nil {
		go s.insertToDB(snap)
	}
}

// insertToDB writes a single snapshot to the telemetry_snapshots hypertable.
func (s *Store) insertToDB(snap Snapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO telemetry_snapshots (
			time, cluster_name, namespace, workload_name, runtime, model_name,
			request_rate_per_sec, p50_latency_ms, p95_latency_ms, p99_latency_ms,
			queue_depth, gpu_utilization_percent, gpu_memory_used_percent, 
			tokens_per_second, error_rate_percent
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		snap.Timestamp, snap.ClusterName, snap.Namespace, snap.WorkloadName,
		snap.Runtime, snap.ModelName, snap.RequestRatePerSec,
		snap.P50LatencyMs, snap.P95LatencyMs, snap.P99LatencyMs,
		snap.QueueDepth, snap.GPUUtilizationPct, snap.GPUMemoryUsedPct,
		snap.TokensPerSecond, snap.ErrorRatePct,
	)
	if err != nil {
		// log but dont crash
		fmt.Printf("warn: failed to insert telemetry to DB: %v\n", err)

	}
}

// Latest returns the most recent snapshot for each workload from memory.
func (s *Store) Latest() []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Snapshot, 0, len(s.data))
	for _, snaps := range s.data {
		if len(snaps) > 0 {
			result = append(result, snaps[len(snaps)-1])
		}
	}
	return result
}

// All returns every stored snapshot from memory.
func (s *Store) All() []Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Snapshot
	for _, snaps := range s.data {
		result = append(result, snaps...)
	}
	return result
}

// QueryRecent fetches the last N snapshots for a workload directly from TimescaleDB.
// Falls back to in-memory if DB is not enabled.
func (s *Store) QueryRecent(ctx context.Context, workload string, limit int) ([]Snapshot, error) {
	if s.pool == nil {
		// DB not enabled — return from memory
		s.mu.RLock()
		defer s.mu.RUnlock()
		snaps := s.data[workload]
		if len(snaps) > limit {
			snaps = snaps[len(snaps)-limit:]
		}
		return snaps, nil
	}

	rows, err := s.pool.Query(ctx, `
        SELECT time, cluster_name, namespace, workload_name, runtime, model_name,
               request_rate_per_sec, p50_latency_ms, p95_latency_ms, p99_latency_ms,
               queue_depth, gpu_utilization_percent, gpu_memory_used_percent,
               tokens_per_second, error_rate_percent
        FROM telemetry_snapshots
        WHERE workload_name = $1
        ORDER BY time DESC
        LIMIT $2`, workload, limit)
	if err != nil {
		return nil, fmt.Errorf("querying telemetry: %w", err)
	}
	defer rows.Close()

	var result []Snapshot
	for rows.Next() {
		var snap Snapshot
		err := rows.Scan(
			&snap.Timestamp, &snap.ClusterName, &snap.Namespace, &snap.WorkloadName,
			&snap.Runtime, &snap.ModelName, &snap.RequestRatePerSec,
			&snap.P50LatencyMs, &snap.P95LatencyMs, &snap.P99LatencyMs,
			&snap.QueueDepth, &snap.GPUUtilizationPct, &snap.GPUMemoryUsedPct,
			&snap.TokensPerSecond, &snap.ErrorRatePct,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning telemetry row: %w", err)
		}
		result = append(result, snap)
	}
	return result, nil
}
