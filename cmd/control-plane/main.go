package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pm32900/inference-fabric-autopilot/internal/api"
	"github.com/pm32900/inference-fabric-autopilot/internal/collector"
	"github.com/pm32900/inference-fabric-autopilot/internal/config"
	k8swatcher "github.com/pm32900/inference-fabric-autopilot/internal/k8s"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

func main() {
	// load config
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("starting inference-fabric-autopilot | collector=%s db=%v k8s=%v",
		cfg.Collector.Mode, cfg.Database.Enabled, cfg.Kubernetes.Enabled)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// telemetry store
	var store *telemetry.Store
	if cfg.Database.Enabled {
		pool, err := pgxpool.New(ctx, cfg.Database.DSN)
		if err != nil {
			log.Fatalf("failed to connect to databade: %v", err)
		}
		defer pool.Close()
		if err := pool.Ping(ctx); err != nil {
			log.Fatalf("database ping failed: %v", err)
		}
		log.Println("connected to TimeScaleDB")
	} else {
		store = telemetry.NewStore()
		log.Println("using in-memory store (database disabled)")
	}

	// 3 Start Kubernetes watcher
	var workloadStore *k8swatcher.WorkloadStore
	if cfg.Kubernetes.Enabled {
		workloadStore = k8swatcher.NewWorkloadStore()
		interval := time.Duration(cfg.Kubernetes.SyncIntervalSeconds) * time.Second
		watcher, err := k8swatcher.NewWatcher(cfg.Kubernetes.Namespace, workloadStore, interval)
		if err != nil {
			log.Printf("warn: k8s watcher disabled: %v", err)
			workloadStore = nil
		} else {
			watcher.Start(ctx)
			log.Printf("k8s watcher started | namespace=%s interval=%s",
				cfg.Kubernetes.Namespace, interval)
		}
	}

	// 4 Start collector
	collectorInterval := time.Duration(cfg.Collector.IntervalSeconds) * time.Second
	switch cfg.Collector.Mode {
	case "prometheus":
		targets := buildPrometheusTargets(cfg)
		if len(targets) == 0 {
			log.Println("warn: prometheus mode selected but no targets configured, falling back to simulated")
			startSimulated(ctx, store, collectorInterval)
		} else {
			c := collector.NewPrometheusCollector(targets, store, collectorInterval)
			c.Start(ctx)
			log.Printf("prometheus collector started | targets=%d", len(targets))
		}
	default:
		startSimulated(ctx, store, collectorInterval)
		log.Println("simulated collector started")
	}

	// 5 Start HTTP server
	mux := http.NewServeMux()
	api.NewServer(store, workloadStore, cfg, mux)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("HTTP server listening on :%d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// 6 Graceful shutdown
	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("stopped")
}

func startSimulated(_ context.Context, store *telemetry.Store, interval time.Duration) {
	collector.Start(store, interval)
}

// buildPrometheusTargets reads scrape targets from config
func buildPrometheusTargets(cfg *config.Config) []collector.PrometheusTarget {
	var targets []collector.PrometheusTarget
	for _, t := range cfg.Collector.PrometheusTargets {
		targets = append(targets, collector.PrometheusTarget{
			WorkloadName: t.WorkloadName,
			Namespace:    t.Namespace,
			Runtime:      t.Runtime,
			ModelName:    t.ModelName,
			MetricsURL:   t.MetricsURL,
		})
	}
	return targets
}
