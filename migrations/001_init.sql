-- 001_init.sql — schema for the optional TimescaleDB history backend.
--
-- Apply once against the target database:
--   psql "$IFA_DATABASE_DSN" -f migrations/001_init.sql
--
-- IFA never runs migrations itself. Granting the control plane DDL rights to a
-- database it only ever appends to would widen its blast radius for no benefit,
-- so schema changes stay an operator action.
--
-- Every measurement column is nullable. A NULL means "not measured" — the
-- runtime did not expose that signal for that scrape — and is deliberately
-- distinct from 0.0. Queries that average over these columns should therefore
-- use AVG(col) (which skips NULLs) rather than COALESCE(col, 0).

CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE TABLE IF NOT EXISTS telemetry_snapshots (
    time                          TIMESTAMPTZ      NOT NULL,
    cluster_name                  TEXT             NOT NULL,
    namespace                     TEXT             NOT NULL,
    workload_name                 TEXT             NOT NULL,
    runtime                       TEXT,
    model_name                    TEXT,

    -- throughput
    request_rate_per_sec          DOUBLE PRECISION,
    tokens_per_second             DOUBLE PRECISION,

    -- latency, milliseconds
    p50_latency_ms                DOUBLE PRECISION,
    p95_latency_ms                DOUBLE PRECISION,
    p99_latency_ms                DOUBLE PRECISION,
    ttft_p50_ms                   DOUBLE PRECISION,
    ttft_p95_ms                   DOUBLE PRECISION,
    ttft_p99_ms                   DOUBLE PRECISION,
    queue_time_p95_ms             DOUBLE PRECISION,

    -- scheduler state
    requests_running              DOUBLE PRECISION,
    requests_waiting              DOUBLE PRECISION,
    waiting_for_capacity          DOUBLE PRECISION,
    waiting_deferred              DOUBLE PRECISION,

    -- memory and cache
    gpu_utilization_percent       DOUBLE PRECISION,
    gpu_memory_used_percent       DOUBLE PRECISION,
    kv_cache_usage_percent        DOUBLE PRECISION,
    preemptions_per_sec           DOUBLE PRECISION,
    prefix_cache_hit_rate_percent DOUBLE PRECISION,

    -- errors
    error_rate_percent            DOUBLE PRECISION,
    abort_rate_percent            DOUBLE PRECISION,

    -- Kubernetes context
    replicas                      DOUBLE PRECISION,
    ready_replicas                DOUBLE PRECISION
);

SELECT create_hypertable('telemetry_snapshots', 'time', if_not_exists => TRUE);

-- The dominant query shape is "recent history for one workload".
CREATE INDEX IF NOT EXISTS idx_telemetry_workload
    ON telemetry_snapshots (namespace, workload_name, time DESC);

-- Suggested retention. Left commented because the right value depends on how
-- much history the operator wants to keep, and silently dropping someone's
-- data is not a decision a schema file should make for them.
-- SELECT add_retention_policy('telemetry_snapshots', INTERVAL '30 days');
