package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level config struct — mirrors config.yaml exactly.
type Config struct {
	Server         ServerConfig      `yaml:"server"`
	Collector      CollectorConfig   `yaml:"collector"`
	Kubernetes     KubernetesConfig  `yaml:"kubernetes"`
	Database       DatabaseConfig    `yaml:"database"`
	Recommender    RecommenderConfig `yaml:"recommender"`
	Logging        LoggingConfig     `yaml:"logging"`
	DeploymentMode string            `yaml:"deployment_mode"` // "standard" or "airgapped"
	Egress         EgressConfig      `yaml:"egress"`
	Privacy        PrivacyConfig     `yaml:"privacy"`
	RBAC           RBACConfig        `yaml:"rbac"`
	Prometheus     PrometheusConfig  `yaml:"prometheus"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
}

type CollectorConfig struct {
	Mode              string                   `yaml:"mode"` // "simulated" or "prometheus"
	IntervalSeconds   int                      `yaml:"interval_seconds"`
	PrometheusTargets []PrometheusTargetConfig `yaml:"prometheus_targets"`
}

type PrometheusTargetConfig struct {
	WorkloadName string `yaml:"workload_name"`
	Namespace    string `yaml:"namespace"`
	Runtime      string `yaml:"runtime"`
	ModelName    string `yaml:"model_name"`
	MetricsURL   string `yaml:"metrics_url"`
}

type KubernetesConfig struct {
	Enabled             bool   `yaml:"enabled"`
	Namespace           string `yaml:"namespace"`
	SyncIntervalSeconds int    `yaml:"sync_interval_seconds"`
}

type DatabaseConfig struct {
	Enabled bool   `yaml:"enabled"`
	DSN     string `yaml:"dsn"`
}

type RecommenderConfig struct {
	Thresholds ThresholdConfig `yaml:"thresholds"`
}

type ThresholdConfig struct {
	LowGPUUtilPct       float64 `yaml:"low_gpu_util_pct"`
	HighGPUMemPct       float64 `yaml:"high_gpu_mem_pct"`
	HighP95LatencyMs    float64 `yaml:"high_p95_latency_ms"`
	HighQueueDepth      int     `yaml:"high_queue_depth"`
	HighErrorRatePct    float64 `yaml:"high_error_rate_pct"`
	MinReplicasForRPS   float64 `yaml:"min_replicas_for_rps"`
	HighKVCacheUsagePct float64 `yaml:"high_kv_cache_usage_pct"`
	HighTTFTP95Ms       float64 `yaml:"high_ttft_p95_ms"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// EgressConfig controls whether outbound network calls are permitted.
// In airgapped mode this must be false. Enforced by NetworkPolicy separately,
// but declared here so the posture is auditable at the config layer.
type EgressConfig struct {
	Enabled bool `yaml:"enabled"`
}

// PrivacyConfig documents and declares what sensitive data is never collected.
// All fields default to false. There is no code path that reads this data
// regardless of config, but these fields allow config-layer audits.
type PrivacyConfig struct {
	CollectPromptBodies    bool `yaml:"collect_prompt_bodies"`
	CollectResponseBodies  bool `yaml:"collect_response_bodies"`
	CollectHeaders         bool `yaml:"collect_headers"`
	CollectUserIdentifiers bool `yaml:"collect_user_identifiers"`
}

// RBACConfig documents the declared RBAC posture.
// Only "read-only" is valid in Phase 2.5.
type RBACConfig struct {
	Mode string `yaml:"mode"` // "read-only"
}

// PrometheusConfig holds the global Prometheus base URL used in airgapped mode
// where a single in-cluster Prometheus serves all targets.
type PrometheusConfig struct {
	URL string `yaml:"url"`
}

// Validate checks that config values are internally consistent.
// Returns an error if any invariant is violated.
func (c *Config) Validate() error {
	if c.DeploymentMode != "standard" && c.DeploymentMode != "airgapped" {
		return fmt.Errorf("deployment_mode must be 'standard' or 'airgapped', got: %q", c.DeploymentMode)
	}
	if c.DeploymentMode == "airgapped" && c.Egress.Enabled {
		return fmt.Errorf("egress.enabled must be false when deployment_mode is 'airgapped'")
	}
	if c.Privacy.CollectPromptBodies {
		return fmt.Errorf("privacy.collect_prompt_bodies must be false: prompt collection is not supported")
	}
	if c.Privacy.CollectResponseBodies {
		return fmt.Errorf("privacy.collect_response_bodies must be false: response collection is not supported")
	}
	if c.Privacy.CollectHeaders {
		return fmt.Errorf("privacy.collect_headers must be false: header collection is not supported")
	}
	if c.Privacy.CollectUserIdentifiers {
		return fmt.Errorf("privacy.collect_user_identifiers must be false: user identifier collection is not supported")
	}
	if c.RBAC.Mode != "read-only" {
		return fmt.Errorf("rbac.mode must be 'read-only', got: %q", c.RBAC.Mode)
	}
	return nil
}

// Load reads the YAML config file at the given path and returns a parsed Config.
// Returns sensible defaults if the file doesn't exist.
func Load(path string) (*Config, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// no config file — use defaults, that's fine for local dev
			return cfg, nil
		}
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}

	return cfg, nil
}

// defaults returns a Config pre-filled with safe defaults.
// These match the values in config.yaml so either source works.
func defaults() *Config {
	return &Config{
		Server: ServerConfig{Port: 8080},
		Collector: CollectorConfig{
			Mode:            "simulated",
			IntervalSeconds: 5,
		},
		Kubernetes: KubernetesConfig{
			Enabled:             false,
			Namespace:           "inference",
			SyncIntervalSeconds: 30,
		},
		Database: DatabaseConfig{
			Enabled: false,
			DSN:     "postgres://postgres:autopilot@localhost:5432/autopilot?sslmode=disable",
		},
		Recommender: RecommenderConfig{
			Thresholds: ThresholdConfig{
				LowGPUUtilPct:       30.0,
				HighGPUMemPct:       85.0,
				HighP95LatencyMs:    500.0,
				HighQueueDepth:      10,
				HighErrorRatePct:    2.0,
				MinReplicasForRPS:   10.0,
				HighKVCacheUsagePct: 80.0,
				HighTTFTP95Ms:       2000.0,
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		DeploymentMode: "standard",
		Egress: EgressConfig{
			Enabled: true,
		},
		Privacy: PrivacyConfig{
			CollectPromptBodies:    false,
			CollectResponseBodies:  false,
			CollectHeaders:         false,
			CollectUserIdentifiers: false,
		},
		RBAC: RBACConfig{
			Mode: "read-only",
		},
		Prometheus: PrometheusConfig{
			URL: "",
		},
	}
}
