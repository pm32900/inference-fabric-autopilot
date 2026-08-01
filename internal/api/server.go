package api

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/pm32900/inference-fabric-autopilot/internal/config"
	"github.com/pm32900/inference-fabric-autopilot/internal/k8s"
	"github.com/pm32900/inference-fabric-autopilot/internal/recommender"
	"github.com/pm32900/inference-fabric-autopilot/internal/telemetry"
)

// Server holds the dependencies needed by all HTTP handlers.
type Server struct {
	store         *telemetry.Store
	workloadStore *k8s.WorkloadStore
	cfg           *config.Config
}

// NewServer wires up all routes and returns the server.
func NewServer(store *telemetry.Store, workloadStore *k8s.WorkloadStore, cfg *config.Config, mux *http.ServeMux) *Server {
	s := &Server{
		store:         store,
		workloadStore: workloadStore,
		cfg:           cfg,
	}

	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/telemetry", s.handleTelemetry)
	mux.HandleFunc("/recommendations", s.handleRecommendations)
	mux.HandleFunc("/workloads", s.handleWorkloads)
	mux.HandleFunc("/metrics", s.handleMetricsStub)

	return s
}

// GET /healthz
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":      "ok",
		"collector":   s.cfg.Collector.Mode,
		"db_enabled":  boolStr(s.cfg.Database.Enabled),
		"k8s_enabled": boolStr(s.cfg.Kubernetes.Enabled),
	})
}

// GET /telemetry (latest snapshot per workload)
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	snapshots := s.store.Latest()
	writeJSON(w, http.StatusOK, snapshots)
}

// GET /recommendations (run rule engine and get active recommendations)
func (s *Server) handleRecommendations(w http.ResponseWriter, r *http.Request) {
	snapshots := s.store.Latest()
	recs := recommender.Analyze(snapshots, s.cfg.Recommender.Thresholds)
	writeJSON(w, http.StatusOK, recs)
}

// GET /workloads (kubernetes discovered)
func (s *Server) handleWorkloads(w http.ResponseWriter, r *http.Request) {
	if s.workloadStore == nil {
		writeJSON(w, http.StatusOK, []*k8s.WorkloadInfo{})
		return
	}
	writeJSON(w, http.StatusOK, s.workloadStore.All())
}

// GET /metrics (stub) (Future prometheus exposition)
func (s *Server) handleMetricsStub(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.WriteHeader(http.StatusOK)
	// Phase 3 TODO (expose real prometheus metrics here)
	_, _ = w.Write([]byte("# Inference Fabric Autopilot metrics\n# TODO: Implement\n"))
}

// writeJSON sets content type and encodes payload as JSON
func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("error encoding JSON response: %v", err)
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
