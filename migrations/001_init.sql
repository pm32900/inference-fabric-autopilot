-- migrations/001_init.sql
-- Run this once against your TimescaleDB instance to set up the schema.

-- Enable the TimescaleDB extension
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- Main telemetry table, one row per snapshot per workload
CREATE TABLE IF NOT EXISTS telemetry_snapshots (
    time                    TIMESTAMPTZ     NOT NULL,
    cluster_name            TEXT            NOT NULL,
    namespace               TEXT            NOT NULL,
    workload_name           TEXT            NOT NULL,
    runtime                 TEXT,
    model_name              TEXT,
    request_rate_per_sec    DOUBLE PRECISION,
    p50_latency_ms          DOUBLE PRECISION,
    p95_latency_ms          DOUBLE PRECISION,
    p99_latency_ms          DOUBLE PRECISION,
    queue_depth             INTEGER,
    gpu_utilization_percent DOUBLE PRECISION,
    gpu_memory_used_percent DOUBLE PRECISION,
    tokens_per_second       DOUBLE PRECISION,
    error_rate_percent      DOUBLE PRECISION
);

-- convert to hypertable
SELECT create_hypertable('telemetry_snapshots', 'time', if_not_exists => TRUE);

-- Index for fast per-workload queries
CREATE INDEX IF NOT EXISTS idx_telemetry_workload
    ON telemetry_snapshots (workload_name, time DESC);

-- Recommendations table, storing generated recommendations with timestamp
CREATE TABLE IF NOT EXISTS recommendations (
    id                  TEXT            NOT NULL,
    generated_at        TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    severity            TEXT            NOT NULL,
    workload_name       TEXT            NOT NULL,
    title               TEXT            NOT NULL,
    explanation         TEXT,
    suggested_action    TEXT,
    related_metric      TEXT
);

CREATE INDEX IF NOT EXISTS idx_recommendations_workload
    ON recommendations (workload_name, generated_at DESC);


CREATE UNIQUE INDEX IF NOT EXISTS idx_recommendations_unique
    ON recommendations (workload_name, generated_at);
