package metrics

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecordSnapshot_IncrementsCounter(t *testing.T) {
	r := NewRegistry()
	r.RecordSnapshot("llm-serving", "vllm")
	r.RecordSnapshot("llm-serving", "vllm")
	r.RecordSnapshot("embedding-svc", "ollama")

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	if !strings.Contains(out, `ifa_snapshots_ingested_total{workload="llm-serving",runtime="vllm"} 2`) {
		t.Errorf("expected snapshot counter 2 for llm-serving/vllm, got:\n%s", out)
	}
	if !strings.Contains(out, `ifa_snapshots_ingested_total{workload="embedding-svc",runtime="ollama"} 1`) {
		t.Errorf("expected snapshot counter 1 for embedding-svc/ollama, got:\n%s", out)
	}
}

func TestRecordScrapeError_IncrementsCounter(t *testing.T) {
	r := NewRegistry()
	r.RecordScrapeError("llm-serving")
	r.RecordScrapeError("llm-serving")
	r.RecordScrapeError("triton-svc")

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	if !strings.Contains(out, `ifa_scrape_errors_total{workload="llm-serving"} 2`) {
		t.Errorf("expected scrape error count 2 for llm-serving, got:\n%s", out)
	}
	if !strings.Contains(out, `ifa_scrape_errors_total{workload="triton-svc"} 1`) {
		t.Errorf("expected scrape error count 1 for triton-svc, got:\n%s", out)
	}
}

func TestRecordRecommendations_UpdatesActiveAndTotal(t *testing.T) {
	r := NewRegistry()

	// First analysis run: 2 warnings, 1 critical
	r.RecordRecommendations([]string{"warning", "warning", "critical"})

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	if !strings.Contains(out, "ifa_active_recommendations 3") {
		t.Errorf("expected active_recommendations 3, got:\n%s", out)
	}
	if !strings.Contains(out, `ifa_recommendations_total{severity="warning"} 2`) {
		t.Errorf("expected warning total 2, got:\n%s", out)
	}
	if !strings.Contains(out, `ifa_recommendations_total{severity="critical"} 1`) {
		t.Errorf("expected critical total 1, got:\n%s", out)
	}

	// Second run: 1 warning — active should be 1, totals accumulate
	r.RecordRecommendations([]string{"warning"})

	buf.Reset()
	r.WriteMetrics(&buf)
	out = buf.String()

	if !strings.Contains(out, "ifa_active_recommendations 1") {
		t.Errorf("expected active_recommendations 1 after second run, got:\n%s", out)
	}
	if !strings.Contains(out, `ifa_recommendations_total{severity="warning"} 3`) {
		t.Errorf("expected cumulative warning total 3, got:\n%s", out)
	}
}

func TestSetStoreSize_ReflectedInOutput(t *testing.T) {
	r := NewRegistry()
	r.SetStoreSize(42)

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	if !strings.Contains(out, "ifa_store_size 42") {
		t.Errorf("expected ifa_store_size 42, got:\n%s", out)
	}
}

func TestHandler_SetsContentType(t *testing.T) {
	r := NewRegistry()
	r.RecordSnapshot("svc", "vllm")

	req := httptest.NewRequest("GET", "/metrics", nil)
	rr := httptest.NewRecorder()
	r.Handler()(rr, req)

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", ct)
	}
	if rr.Code != 200 {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "ifa_snapshots_ingested_total") {
		t.Errorf("expected metric name in body, got:\n%s", rr.Body.String())
	}
}

func TestLastScrapeTimestamp_SetOnSnapshot(t *testing.T) {
	r := NewRegistry()
	r.RecordSnapshot("my-workload", "triton")

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	if !strings.Contains(out, `ifa_last_scrape_timestamp{workload="my-workload"}`) {
		t.Errorf("expected last scrape timestamp for my-workload, got:\n%s", out)
	}
}

func TestEmptyRegistry_OutputHasAllHelpLines(t *testing.T) {
	r := NewRegistry()

	var buf bytes.Buffer
	r.WriteMetrics(&buf)
	out := buf.String()

	expectedHelp := []string{
		"# HELP ifa_snapshots_ingested_total",
		"# HELP ifa_store_size",
		"# HELP ifa_recommendations_total",
		"# HELP ifa_active_recommendations",
		"# HELP ifa_scrape_errors_total",
		"# HELP ifa_last_scrape_timestamp",
	}
	for _, h := range expectedHelp {
		if !strings.Contains(out, h) {
			t.Errorf("missing HELP line %q in output:\n%s", h, out)
		}
	}
}
