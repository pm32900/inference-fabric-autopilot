package demo

import (
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// vLLM's own histogram bucket boundaries, from vllm/v1/metrics/loggers.py.
// Reusing them matters: the demo's job is to prove the quantile estimation
// works at the resolution real vLLM provides, and a finer bucket set would
// flatter it.
var (
	ttftBuckets = []float64{
		0.001, 0.005, 0.01, 0.02, 0.04, 0.06, 0.08, 0.1, 0.25, 0.5, 0.75, 1.0,
		2.5, 5.0, 7.5, 10.0, 20.0, 40.0, 80.0, 160.0, 640.0, 2560.0,
	}
	latencyBuckets = []float64{
		0.3, 0.5, 0.8, 1.0, 1.5, 2.0, 2.5, 5.0, 10.0, 15.0, 20.0, 30.0, 40.0,
		50.0, 60.0, 120.0, 240.0, 480.0, 960.0, 1920.0, 7680.0,
	}
)

// histogram accumulates observations into cumulative Prometheus buckets.
type histogram struct {
	bounds []float64
	counts []float64 // per-bucket, not cumulative; rendered cumulatively
	sum    float64
	count  float64
}

func newHistogram(bounds []float64) *histogram {
	return &histogram{bounds: bounds, counts: make([]float64, len(bounds)+1)}
}

func (h *histogram) observe(v float64) {
	if math.IsNaN(v) || v < 0 {
		return
	}
	idx := sort.SearchFloat64s(h.bounds, v)
	h.counts[idx]++
	h.sum += v
	h.count++
}

// render writes the cumulative bucket, sum and count series.
func (h *histogram) render(b *strings.Builder, name, labels string) {
	fmt.Fprintf(b, "# TYPE %s histogram\n", name)
	cum := 0.0
	for i, bound := range h.bounds {
		cum += h.counts[i]
		fmt.Fprintf(b, "%s_bucket{%s,le=\"%s\"} %.1f\n", name, labels, formatLE(bound), cum)
	}
	cum += h.counts[len(h.counts)-1]
	fmt.Fprintf(b, "%s_bucket{%s,le=\"+Inf\"} %.1f\n", name, labels, cum)
	fmt.Fprintf(b, "%s_count{%s} %.1f\n", name, labels, h.count)
	fmt.Fprintf(b, "%s_sum{%s} %.4f\n", name, labels, h.sum)
}

// formatLE renders a bucket boundary the way Prometheus clients do.
//
// It uses strconv rather than trimming trailing zeros from %g output: trimming
// turns 10 into 1 and 2560 into 256, which silently collides distinct
// boundaries and corrupts every quantile computed from the payload.
func formatLE(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// engineState is the simulated vLLM engine.
type engineState struct {
	// Gauges.
	running         float64
	waiting         float64
	waitingCapacity float64
	waitingDeferred float64
	kvCache         float64 // fraction in [0,1], as vLLM reports it
	gpuUtil         float64 // percent, as DCGM reports it
	gpuMemPct       float64

	// Counters. These only ever increase, so the collector's rate conversion
	// has something real to difference.
	promptTokens  float64
	genTokens     float64
	finished      float64
	aborted       float64
	preemptions   float64
	prefixQueries float64
	prefixHits    float64

	ttft  *histogram
	e2e   *histogram
	queue *histogram

	// remainder and abortRemainder carry fractional counts between ticks so
	// that a rate of 0.5 per tick does not truncate to zero forever.
	remainder      float64
	abortRemainder float64
}

func newEngineState() *engineState {
	return &engineState{
		ttft:  newHistogram(ttftBuckets),
		e2e:   newHistogram(latencyBuckets),
		queue: newHistogram(latencyBuckets),
	}
}

// completeRequests advances the finished counter by ratePerSec over dt and
// returns how many whole requests completed this tick.
// abortShare is the fraction of completions the caller gave up on. It is a
// share of completions rather than a per-tick coin flip so that the resulting
// abort rate does not depend on the tick interval — a low-throughput workload
// would otherwise show a far higher abort rate than a busy one for no reason.
func (s *engineState) completeRequests(dt time.Duration, ratePerSec, abortShare float64, _ *rand.Rand) int {
	exact := ratePerSec*dt.Seconds() + s.remainder
	n := int(exact)
	s.remainder = exact - float64(n)
	s.finished += float64(n)

	s.abortRemainder += float64(n) * abortShare
	if s.abortRemainder >= 1 {
		whole := math.Floor(s.abortRemainder)
		s.aborted += whole
		s.abortRemainder -= whole
	}
	return n
}

func (s *engineState) observeTTFT(v float64)  { s.ttft.observe(v) }
func (s *engineState) observeE2E(v float64)   { s.e2e.observe(v) }
func (s *engineState) observeQueue(v float64) { s.queue.observe(v) }

func (s *engineState) addTokens(dt time.Duration, genPerSec, promptPerSec float64, rng *rand.Rand) {
	jitter := 0.9 + rng.Float64()*0.2
	s.genTokens += genPerSec * dt.Seconds() * jitter
	s.promptTokens += promptPerSec * dt.Seconds() * jitter
}

func (s *engineState) addPrefixCache(dt time.Duration, queriedPerSec, hitRate float64, rng *rand.Rand) {
	queried := queriedPerSec * dt.Seconds() * (0.9 + rng.Float64()*0.2)
	s.prefixQueries += queried
	s.prefixHits += queried * hitRate
}

// workload pairs a scenario with its evolving state.
type workload struct {
	scenario Scenario
	state    *engineState
	rng      *rand.Rand
	mu       sync.Mutex
}

// Server serves simulated vLLM and DCGM endpoints over HTTP.
type Server struct {
	workloads []*workload
	httpSrv   *http.Server
	listener  net.Listener
	tick      time.Duration
	stop      chan struct{}
	wg        sync.WaitGroup
}

// tickInterval is how often the simulation advances. It is much faster than the
// scrape interval so that counters move smoothly between scrapes.
const tickInterval = 200 * time.Millisecond

// NewServer starts a simulated fleet on an ephemeral loopback port.
//
// The listener binds to 127.0.0.1 explicitly: the demo exists to be run on a
// laptop, and a simulator that also listened on every interface would be an
// unpleasant surprise on a shared network.
func NewServer(seed int64) (*Server, error) { return newServer(seed, true) }

// NewManualServer starts the simulated fleet without the background clock, so a
// caller can step it with Advance. Tests use it to run the whole pipeline —
// scrape, parse, rate, evaluate — over minutes of simulated time in
// milliseconds of real time.
func NewManualServer(seed int64) (*Server, error) { return newServer(seed, false) }

func newServer(seed int64, autoAdvance bool) (*Server, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("demo: listening: %w", err)
	}

	scenarios := Scenarios()
	s := &Server{
		workloads: make([]*workload, 0, len(scenarios)),
		listener:  ln,
		tick:      tickInterval,
		stop:      make(chan struct{}),
	}
	for i, sc := range scenarios {
		s.workloads = append(s.workloads, &workload{
			scenario: sc,
			state:    newEngineState(),
			// One generator per workload, seeded deterministically, so a demo
			// run is reproducible and one workload's draws do not depend on
			// another's scrape timing.
			rng: rand.New(rand.NewSource(seed + int64(i)*7919)),
		})
	}

	mux := http.NewServeMux()
	for _, w := range s.workloads {
		w := w
		mux.HandleFunc("/metrics/"+w.scenario.Name, func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = rw.Write([]byte(w.renderVLLM()))
		})
		mux.HandleFunc("/dcgm/"+w.scenario.Name, func(rw http.ResponseWriter, _ *http.Request) {
			rw.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
			_, _ = rw.Write([]byte(w.renderDCGM()))
		})
	}

	s.httpSrv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Prime the state so the first scrape sees a running system rather than a
	// fleet that has served nothing.
	s.advance(30 * time.Second)

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		_ = s.httpSrv.Serve(ln)
	}()
	if autoAdvance {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.run()
		}()
	}
	return s, nil
}

// Advance steps the simulation by d. It is only meaningful on a server created
// with NewManualServer.
func (s *Server) Advance(d time.Duration) { s.advance(d) }

// BaseURL is the address the simulated fleet is served from.
func (s *Server) BaseURL() string {
	return "http://" + s.listener.Addr().String()
}

// MetricsURL returns the vLLM endpoint for a workload.
func (s *Server) MetricsURL(name string) string { return s.BaseURL() + "/metrics/" + name }

// DCGMURL returns the DCGM endpoint for a workload.
func (s *Server) DCGMURL(name string) string { return s.BaseURL() + "/dcgm/" + name }

// Scenarios returns the simulated workloads.
func (s *Server) Scenarios() []Scenario {
	out := make([]Scenario, 0, len(s.workloads))
	for _, w := range s.workloads {
		out = append(out, w.scenario)
	}
	return out
}

// Replicas implements the collector's WorkloadLookup so the scaling rules have
// the Kubernetes context they need without a cluster.
func (s *Server) Replicas(_, name string) (desired, ready, max int32, ok bool) {
	for _, w := range s.workloads {
		if w.scenario.Name == name && w.scenario.Replicas > 0 {
			return w.scenario.Replicas, w.scenario.Replicas, w.scenario.MaxReplicas, true
		}
	}
	return 0, 0, 0, false
}

// Close stops the simulation and the HTTP server.
func (s *Server) Close() error {
	close(s.stop)
	err := s.httpSrv.Close()
	s.wg.Wait()
	return err
}

func (s *Server) run() {
	ticker := time.NewTicker(s.tick)
	defer ticker.Stop()
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.advance(s.tick)
		}
	}
}

func (s *Server) advance(dt time.Duration) {
	for _, w := range s.workloads {
		w.mu.Lock()
		w.scenario.advance(w.state, dt, w.rng)
		w.mu.Unlock()
	}
}

// renderVLLM produces a vLLM-shaped exposition payload for this workload.
func (w *workload) renderVLLM() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	st := w.state
	labels := fmt.Sprintf("model_name=%q,engine=\"0\"", w.scenario.Model)

	var b strings.Builder
	fmt.Fprintf(&b, "# TYPE vllm:num_requests_running gauge\n")
	fmt.Fprintf(&b, "vllm:num_requests_running{%s} %.1f\n", labels, st.running)
	fmt.Fprintf(&b, "# TYPE vllm:num_requests_waiting gauge\n")
	fmt.Fprintf(&b, "vllm:num_requests_waiting{%s} %.1f\n", labels, st.waiting)

	if w.scenario.SparseMetrics {
		// Stop here: this workload models a runtime that exposes nothing else.
		return b.String()
	}

	fmt.Fprintf(&b, "# TYPE vllm:num_requests_waiting_by_reason gauge\n")
	fmt.Fprintf(&b, "vllm:num_requests_waiting_by_reason{%s,reason=\"capacity\"} %.1f\n", labels, st.waitingCapacity)
	fmt.Fprintf(&b, "vllm:num_requests_waiting_by_reason{%s,reason=\"deferred\"} %.1f\n", labels, st.waitingDeferred)

	fmt.Fprintf(&b, "# TYPE vllm:kv_cache_usage_perc gauge\n")
	fmt.Fprintf(&b, "vllm:kv_cache_usage_perc{%s} %.4f\n", labels, st.kvCache)

	fmt.Fprintf(&b, "# TYPE vllm:num_preemptions_total counter\n")
	fmt.Fprintf(&b, "vllm:num_preemptions_total{%s} %.1f\n", labels, math.Floor(st.preemptions))

	fmt.Fprintf(&b, "# TYPE vllm:prompt_tokens_total counter\n")
	fmt.Fprintf(&b, "vllm:prompt_tokens_total{%s} %.1f\n", labels, math.Floor(st.promptTokens))
	fmt.Fprintf(&b, "# TYPE vllm:generation_tokens_total counter\n")
	fmt.Fprintf(&b, "vllm:generation_tokens_total{%s} %.1f\n", labels, math.Floor(st.genTokens))

	fmt.Fprintf(&b, "# TYPE vllm:prefix_cache_queries_total counter\n")
	fmt.Fprintf(&b, "vllm:prefix_cache_queries_total{%s} %.1f\n", labels, math.Floor(st.prefixQueries))
	fmt.Fprintf(&b, "# TYPE vllm:prefix_cache_hits_total counter\n")
	fmt.Fprintf(&b, "vllm:prefix_cache_hits_total{%s} %.1f\n", labels, math.Floor(st.prefixHits))

	fmt.Fprintf(&b, "# TYPE vllm:request_success_total counter\n")
	stop := math.Max(math.Floor(st.finished)-math.Floor(st.aborted), 0)
	fmt.Fprintf(&b, "vllm:request_success_total{%s,finished_reason=\"stop\"} %.1f\n", labels, stop)
	fmt.Fprintf(&b, "vllm:request_success_total{%s,finished_reason=\"abort\"} %.1f\n", labels, math.Floor(st.aborted))

	st.ttft.render(&b, "vllm:time_to_first_token_seconds", labels)
	st.e2e.render(&b, "vllm:e2e_request_latency_seconds", labels)
	st.queue.render(&b, "vllm:request_queue_time_seconds", labels)

	// Real vLLM servers also expose the Python client library's process
	// metrics. Including them keeps the demo honest about what the parser has
	// to ignore.
	b.WriteString("# TYPE process_resident_memory_bytes gauge\n")
	b.WriteString("process_resident_memory_bytes 2.6e+10\n")

	return b.String()
}

// renderDCGM produces a DCGM Exporter payload for this workload's GPU.
func (w *workload) renderDCGM() string {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.scenario.SparseMetrics {
		return ""
	}
	const totalMiB = 81920.0 // an 80GB accelerator
	used := totalMiB * w.state.gpuMemPct / 100

	var b strings.Builder
	labels := `gpu="0",UUID="GPU-demo-0000",device="nvidia0"`
	fmt.Fprintf(&b, "# TYPE DCGM_FI_DEV_GPU_UTIL gauge\n")
	fmt.Fprintf(&b, "DCGM_FI_DEV_GPU_UTIL{%s} %.0f\n", labels, w.state.gpuUtil)
	fmt.Fprintf(&b, "# TYPE DCGM_FI_DEV_FB_USED gauge\n")
	fmt.Fprintf(&b, "DCGM_FI_DEV_FB_USED{%s} %.0f\n", labels, used)
	fmt.Fprintf(&b, "# TYPE DCGM_FI_DEV_FB_FREE gauge\n")
	fmt.Fprintf(&b, "DCGM_FI_DEV_FB_FREE{%s} %.0f\n", labels, totalMiB-used)
	fmt.Fprintf(&b, "# TYPE DCGM_FI_DEV_GPU_TEMP gauge\n")
	fmt.Fprintf(&b, "DCGM_FI_DEV_GPU_TEMP{%s} 71\n", labels)
	return b.String()
}
