// Package metrics exposes IFA's own operational metrics in Prometheus text
// format.
//
// It writes the exposition format directly rather than depending on the
// Prometheus client library. The set of metrics here is small and fixed, and
// the library would pull in a substantial dependency tree — including a second
// exposition parser — into a binary whose whole job is to read that format. If
// the metric set ever grows beyond what one file can hold, that trade flips.
//
// Everything a reader needs to judge whether IFA itself is healthy is here:
// whether scrapes are succeeding, how long they take, how much telemetry is
// retained, and whether the optional database backend is keeping up.
package metrics

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Registry holds IFA's operational counters and gauges. It is safe for
// concurrent use.
type Registry struct {
	mu sync.RWMutex

	scrapes       map[targetKey]*targetStats
	recsBySeverit map[string]int64
	activeRecs    int
	storeSize     int
	buildInfo     BuildInfo
	started       time.Time

	// dbStats is supplied by the durable sink when one is configured.
	dbStats func() (written, failed, dropped int64)
	// alertStats is supplied by the webhook sender when one is configured.
	alertStats func() AlertStats

	now func() time.Time
}

// AlertStats is the webhook sender's view of its own delivery outcomes.
//
// It is a struct rather than the bare multiple returns SetDatabaseStats uses
// because five unnamed int64s at a call site is a bug waiting to happen — two
// of these are counters that mean almost opposite things, and transposing
// Dropped and Suppressed would misreport a healthy instance as a lossy one.
type AlertStats struct {
	Sent    int64
	Failed  int64
	Dropped int64
	// Suppressed counts findings the sender deliberately did not deliver
	// because they were already open and unchanged. It is expected to be large
	// and growing on a healthy instance.
	Suppressed int64
	// Open is the number of findings currently tracked as delivered and not yet
	// resolved.
	Open int
}

// BuildInfo is surfaced as a labelled gauge so a scrape identifies which build
// produced it.
type BuildInfo struct {
	Version string
	Commit  string
	GoVer   string
}

type targetKey struct {
	workload string
	runtime  string
}

type targetStats struct {
	scrapes        int64
	errors         int64
	lastSuccess    time.Time
	lastDurationMs float64
	missingMetrics int
}

// New returns an initialised Registry.
func New(build BuildInfo) *Registry {
	now := time.Now
	return &Registry{
		scrapes:       make(map[targetKey]*targetStats),
		recsBySeverit: make(map[string]int64),
		buildInfo:     build,
		started:       now(),
		now:           now,
	}
}

// RecordScrape records a successful scrape.
func (r *Registry) RecordScrape(workload, runtime string, took time.Duration, missing int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.statsFor(workload, runtime)
	s.scrapes++
	s.lastSuccess = r.now()
	s.lastDurationMs = float64(took.Microseconds()) / 1000.0
	s.missingMetrics = missing
}

// RecordScrapeError records a failed scrape. Failures are tracked per target
// rather than globally: one unreachable workload out of thirty is a very
// different situation from thirty unreachable workloads, and a single counter
// cannot tell them apart.
func (r *Registry) RecordScrapeError(workload, runtime string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statsFor(workload, runtime).errors++
}

// statsFor must be called with the lock held.
func (r *Registry) statsFor(workload, runtime string) *targetStats {
	k := targetKey{workload: workload, runtime: runtime}
	s, ok := r.scrapes[k]
	if !ok {
		s = &targetStats{}
		r.scrapes[k] = s
	}
	return s
}

// RecordAnalysis records the outcome of one rule-engine run.
func (r *Registry) RecordAnalysis(severities []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeRecs = len(severities)
	for _, s := range severities {
		r.recsBySeverit[s]++
	}
}

// SetStoreSize updates the retained-snapshot gauge.
func (r *Registry) SetStoreSize(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeSize = n
}

// SetDatabaseStats registers a callback supplying durable-sink counters.
func (r *Registry) SetDatabaseStats(f func() (written, failed, dropped int64)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dbStats = f
}

// SetAlertStats registers a callback supplying webhook-sender counters.
func (r *Registry) SetAlertStats(f func() AlertStats) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.alertStats = f
}

// Handler serves the registry in Prometheus text format.
func (r *Registry) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodGet && req.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.Write(w)
	}
}

// Write renders the registry to w. Exported so tests can assert on the output
// without an HTTP server, and so the exposition can be checked by a parser.
func (r *Registry) Write(w io.Writer) {
	r.mu.RLock()
	dbStats := r.dbStats
	alertStats := r.alertStats
	keys := make([]targetKey, 0, len(r.scrapes))
	for k := range r.scrapes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].workload != keys[j].workload {
			return keys[i].workload < keys[j].workload
		}
		return keys[i].runtime < keys[j].runtime
	})
	snapshot := make(map[targetKey]targetStats, len(keys))
	for _, k := range keys {
		snapshot[k] = *r.scrapes[k]
	}
	severities := make([]string, 0, len(r.recsBySeverit))
	for s := range r.recsBySeverit {
		severities = append(severities, s)
	}
	sort.Strings(severities)
	recCounts := make(map[string]int64, len(severities))
	for _, s := range severities {
		recCounts[s] = r.recsBySeverit[s]
	}
	active, size, build, started := r.activeRecs, r.storeSize, r.buildInfo, r.started
	now := r.now()
	r.mu.RUnlock()

	fam(w, "ifa_build_info", "Build metadata for the running control plane.", "gauge")
	fmt.Fprintf(w, "ifa_build_info{version=\"%s\",commit=\"%s\",go_version=\"%s\"} 1\n",
		esc(build.Version), esc(build.Commit), esc(build.GoVer))

	fam(w, "ifa_uptime_seconds", "Seconds since the control plane started.", "gauge")
	fmt.Fprintf(w, "ifa_uptime_seconds %.3f\n", now.Sub(started).Seconds())

	fam(w, "ifa_scrapes_total", "Successful scrapes, by target workload.", "counter")
	for _, k := range keys {
		fmt.Fprintf(w, "ifa_scrapes_total{workload=\"%s\",runtime=\"%s\"} %d\n",
			esc(k.workload), esc(k.runtime), snapshot[k].scrapes)
	}

	fam(w, "ifa_scrape_errors_total", "Failed scrapes, by target workload.", "counter")
	for _, k := range keys {
		fmt.Fprintf(w, "ifa_scrape_errors_total{workload=\"%s\",runtime=\"%s\"} %d\n",
			esc(k.workload), esc(k.runtime), snapshot[k].errors)
	}

	fam(w, "ifa_scrape_duration_milliseconds",
		"Duration of the most recent successful scrape, by target workload.", "gauge")
	for _, k := range keys {
		fmt.Fprintf(w, "ifa_scrape_duration_milliseconds{workload=\"%s\",runtime=\"%s\"} %.3f\n",
			esc(k.workload), esc(k.runtime), snapshot[k].lastDurationMs)
	}

	fam(w, "ifa_last_successful_scrape_timestamp_seconds",
		"Unix time of the most recent successful scrape, by target workload. "+
			"Alert on time() minus this rather than on error counters: a target that "+
			"stopped being scraped at all increments no error counter.", "gauge")
	for _, k := range keys {
		var ts float64
		if !snapshot[k].lastSuccess.IsZero() {
			ts = float64(snapshot[k].lastSuccess.Unix())
		}
		fmt.Fprintf(w, "ifa_last_successful_scrape_timestamp_seconds{workload=\"%s\",runtime=\"%s\"} %.0f\n",
			esc(k.workload), esc(k.runtime), ts)
	}

	fam(w, "ifa_target_missing_metrics",
		"Expected runtime metrics absent from the most recent scrape, by target workload. "+
			"Non-zero means some rules cannot be evaluated for that workload.", "gauge")
	for _, k := range keys {
		fmt.Fprintf(w, "ifa_target_missing_metrics{workload=\"%s\",runtime=\"%s\"} %d\n",
			esc(k.workload), esc(k.runtime), snapshot[k].missingMetrics)
	}

	fam(w, "ifa_store_snapshots", "Telemetry snapshots retained in memory.", "gauge")
	fmt.Fprintf(w, "ifa_store_snapshots %d\n", size)

	fam(w, "ifa_recommendations_total",
		"Recommendations produced across all analysis runs, by severity.", "counter")
	for _, s := range severities {
		fmt.Fprintf(w, "ifa_recommendations_total{severity=\"%s\"} %d\n", esc(s), recCounts[s])
	}

	fam(w, "ifa_active_recommendations",
		"Recommendations produced by the most recent analysis run.", "gauge")
	fmt.Fprintf(w, "ifa_active_recommendations %d\n", active)

	if dbStats != nil {
		written, failed, dropped := dbStats()
		fam(w, "ifa_database_writes_total",
			"Telemetry rows written to the durable backend.", "counter")
		fmt.Fprintf(w, "ifa_database_writes_total %d\n", written)
		fam(w, "ifa_database_write_failures_total",
			"Telemetry rows the durable backend rejected.", "counter")
		fmt.Fprintf(w, "ifa_database_write_failures_total %d\n", failed)
		fam(w, "ifa_database_dropped_total",
			"Telemetry rows dropped because the write queue was full. "+
				"Non-zero means history is incomplete; diagnostics are unaffected.", "counter")
		fmt.Fprintf(w, "ifa_database_dropped_total %d\n", dropped)
	}

	if alertStats != nil {
		a := alertStats()
		fam(w, "ifa_alerts_sent_total",
			"Alerts delivered to the webhook endpoint, by transition rather than by "+
				"evaluation: a finding that holds for an hour is sent once.", "counter")
		fmt.Fprintf(w, "ifa_alerts_sent_total %d\n", a.Sent)

		fam(w, "ifa_alert_failures_total",
			"Alerts the webhook endpoint did not accept, after retries. "+
				"Delivery is at most once, so these alerts are not resent: alert on "+
				"any increase.", "counter")
		fmt.Fprintf(w, "ifa_alert_failures_total %d\n", a.Failed)

		fam(w, "ifa_alerts_dropped_total",
			"Alerts dropped because the send queue was full, meaning the endpoint "+
				"is slower than findings are produced. Diagnostics are unaffected; "+
				"the API still reports every finding.", "counter")
		fmt.Fprintf(w, "ifa_alerts_dropped_total %d\n", a.Dropped)

		fam(w, "ifa_alerts_suppressed_total",
			"Findings not sent because they were already open and unchanged. "+
				"Expected to be large and growing: this is the deduplication working, "+
				"not a fault.", "counter")
		fmt.Fprintf(w, "ifa_alerts_suppressed_total %d\n", a.Suppressed)

		fam(w, "ifa_alerts_open",
			"Findings currently delivered and not yet resolved.", "gauge")
		fmt.Fprintf(w, "ifa_alerts_open %d\n", a.Open)
	}
}

func fam(w io.Writer, name, help, typ string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}

// esc escapes a label value per the exposition format. Workload names come from
// configuration, so a quote or backslash in one would otherwise produce a
// payload that no Prometheus can parse.
//
// Callers must interpolate the result with %s inside explicit quotes, never
// with %q: Go's %q applies its own escaping on top and the label comes out
// double-escaped.
func esc(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(s)
}
