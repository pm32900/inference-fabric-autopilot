// Package metrics provides a lightweight Prometheus text-format exposition
// of IFA's own operational metrics. It does not depend on the Prometheus
// Go client library — metrics are written directly in the text exposition
// format (https://prometheus.io/docs/instrumenting/exposition_formats/).
//
// Exposed metrics:
//
//	ifa_snapshots_ingested_total    counter  Total telemetry snapshots ingested, by workload and runtime
//	ifa_store_size                  gauge    Current number of snapshots held in the in-memory store
//	ifa_recommendations_total       counter  Total recommendations ever produced, by severity
//	ifa_active_recommendations      gauge    Number of recommendations from the most recent analysis run
//	ifa_scrape_errors_total         counter  Cumulative scrape errors, by target workload
//	ifa_last_scrape_timestamp       gauge    Unix timestamp of the last successful scrape per target
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Registry holds all IFA operational counters and gauges.
// It is safe for concurrent use. Wire it into the control plane at startup
// and pass it to collectors so they can record events.
type Registry struct {
	mu sync.RWMutex

	// snapshotsIngested[workload][runtime] = count
	snapshotsIngested map[string]map[string]int64

	// recommendationsTotal[severity] = count
	recommendationsTotal map[string]int64

	// activeRecommendations is the count from the most recent analysis run
	activeRecommendations int

	// scrapeErrors[workload] = count
	scrapeErrors map[string]int64

	// lastScrape[workload] = unix timestamp
	lastScrape map[string]int64

	// storeSize is set by the store on each Add
	storeSize int
}

// NewRegistry creates an initialised Registry.
func NewRegistry() *Registry {
	return &Registry{
		snapshotsIngested:    make(map[string]map[string]int64),
		recommendationsTotal: make(map[string]int64),
		scrapeErrors:         make(map[string]int64),
		lastScrape:           make(map[string]int64),
	}
}

// RecordSnapshot increments the snapshot ingestion counter for the given
// workload and runtime. Call this from the collector after every successful scrape.
func (r *Registry) RecordSnapshot(workload, runtime string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.snapshotsIngested[workload] == nil {
		r.snapshotsIngested[workload] = make(map[string]int64)
	}
	r.snapshotsIngested[workload][runtime]++
	r.lastScrape[workload] = time.Now().Unix()
}

// RecordScrapeError increments the error counter for the given workload.
func (r *Registry) RecordScrapeError(workload string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scrapeErrors[workload]++
}

// RecordRecommendations records the result of one analysis run.
// active is the number of recommendations produced; each item's severity
// is counted in the cumulative total.
func (r *Registry) RecordRecommendations(severities []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeRecommendations = len(severities)
	for _, s := range severities {
		r.recommendationsTotal[s]++
	}
}

// SetStoreSize updates the gauge tracking in-memory snapshot count.
func (r *Registry) SetStoreSize(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeSize = n
}

// Handler returns an http.HandlerFunc that writes all metrics in
// Prometheus text exposition format. Wire it to GET /metrics.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.WriteMetrics(w)
	}
}

// WriteMetrics renders all metrics to w in Prometheus text format.
// Exported so tests can capture output without an HTTP server.
func (r *Registry) WriteMetrics(w io.Writer) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// ── ifa_snapshots_ingested_total ─────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_snapshots_ingested_total Total telemetry snapshots ingested by IFA`)
	fmt.Fprintln(w, `# TYPE ifa_snapshots_ingested_total counter`)
	for workload, runtimes := range r.snapshotsIngested {
		for runtime, count := range runtimes {
			fmt.Fprintf(w, "ifa_snapshots_ingested_total{workload=%q,runtime=%q} %d\n",
				workload, runtime, count)
		}
	}

	// ── ifa_store_size ────────────────────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_store_size Current number of snapshots held in the in-memory telemetry store`)
	fmt.Fprintln(w, `# TYPE ifa_store_size gauge`)
	fmt.Fprintf(w, "ifa_store_size %d\n", r.storeSize)

	// ── ifa_recommendations_total ────────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_recommendations_total Cumulative recommendations produced by the rule engine, by severity`)
	fmt.Fprintln(w, `# TYPE ifa_recommendations_total counter`)
	for severity, count := range r.recommendationsTotal {
		fmt.Fprintf(w, "ifa_recommendations_total{severity=%q} %d\n", severity, count)
	}

	// ── ifa_active_recommendations ───────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_active_recommendations Number of recommendations produced in the most recent analysis run`)
	fmt.Fprintln(w, `# TYPE ifa_active_recommendations gauge`)
	fmt.Fprintf(w, "ifa_active_recommendations %d\n", r.activeRecommendations)

	// ── ifa_scrape_errors_total ──────────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_scrape_errors_total Cumulative scrape errors per target workload`)
	fmt.Fprintln(w, `# TYPE ifa_scrape_errors_total counter`)
	for workload, count := range r.scrapeErrors {
		fmt.Fprintf(w, "ifa_scrape_errors_total{workload=%q} %d\n", workload, count)
	}

	// ── ifa_last_scrape_timestamp ────────────────────────────────────────────
	fmt.Fprintln(w, `# HELP ifa_last_scrape_timestamp Unix timestamp of the last successful scrape per target`)
	fmt.Fprintln(w, `# TYPE ifa_last_scrape_timestamp gauge`)
	for workload, ts := range r.lastScrape {
		fmt.Fprintf(w, "ifa_last_scrape_timestamp{workload=%q} %d\n", workload, ts)
	}
}
