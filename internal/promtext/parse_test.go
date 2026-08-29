package promtext

import (
	"math"
	"strings"
	"testing"
)

func mustParse(t *testing.T, text string) *MetricFamilies {
	t.Helper()
	mf, err := ParseString(text)
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return mf
}

func TestParseSampleShapes(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantName   string
		wantValue  float64
		wantLabels Labels
		wantSkip   bool
	}{
		{name: "unlabelled", line: `go_goroutines 42`, wantName: "go_goroutines", wantValue: 42},
		{
			name:       "single label",
			line:       `vllm:num_requests_waiting{model_name="meta-llama/Llama-3.1-8B"} 7.0`,
			wantName:   "vllm:num_requests_waiting",
			wantValue:  7,
			wantLabels: Labels{"model_name": "meta-llama/Llama-3.1-8B"},
		},
		{
			name:       "multiple labels and trailing timestamp",
			line:       `nv_inference_count{model="resnet50",version="1"} 1024 1699999999000`,
			wantName:   "nv_inference_count",
			wantValue:  1024,
			wantLabels: Labels{"model": "resnet50", "version": "1"},
		},
		{
			name:       "label value containing a brace, comma and space",
			line:       `weird{note="a{b},c d",other="x"} 1`,
			wantName:   "weird",
			wantValue:  1,
			wantLabels: Labels{"note": "a{b},c d", "other": "x"},
		},
		{
			name:       "escaped quote and backslash in label value",
			line:       `esc{path="C:\\tmp",msg="say \"hi\""} 3`,
			wantName:   "esc",
			wantValue:  3,
			wantLabels: Labels{"path": `C:\tmp`, "msg": `say "hi"`},
		},
		{
			name:       "scientific notation",
			line:       `bytes_total{a="1"} 1.048576e+06`,
			wantName:   "bytes_total",
			wantValue:  1048576,
			wantLabels: Labels{"a": "1"},
		},
		{
			name:      "negative value",
			line:      `temp_delta -12.5`,
			wantName:  "temp_delta",
			wantValue: -12.5,
		},
		{name: "no value", line: `lonely_metric`, wantSkip: true},
		{name: "non numeric value", line: `bad_metric notanumber`, wantSkip: true},
		{name: "unterminated label block", line: `bad{a="1" 5`, wantSkip: true},
		{name: "unterminated quote", line: `bad{a="1} 5`, wantSkip: true},
		{name: "name starting with a digit", line: `9lives 1`, wantSkip: true},
		{name: "garbage", line: `{}{}{}`, wantSkip: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mf := mustParse(t, tc.line)
			if tc.wantSkip {
				if mf.LinesSkipped != 1 {
					t.Fatalf("expected the line to be skipped, LinesSkipped=%d names=%v",
						mf.LinesSkipped, mf.Names())
				}
				return
			}
			if mf.LinesSkipped != 0 {
				t.Fatalf("line was unexpectedly skipped")
			}
			got := mf.Select(tc.wantName, nil)
			if len(got) != 1 {
				t.Fatalf("Select(%q) returned %d samples, want 1", tc.wantName, len(got))
			}
			if got[0].Value != tc.wantValue {
				t.Errorf("value = %v, want %v", got[0].Value, tc.wantValue)
			}
			for k, v := range tc.wantLabels {
				if got[0].Labels[k] != v {
					t.Errorf("label %q = %q, want %q", k, got[0].Labels[k], v)
				}
			}
			if len(got[0].Labels) != len(tc.wantLabels) {
				t.Errorf("label count = %d, want %d (%v)", len(got[0].Labels), len(tc.wantLabels), got[0].Labels)
			}
		})
	}
}

func TestSpecialValues(t *testing.T) {
	mf := mustParse(t, strings.Join([]string{
		`plus_inf +Inf`,
		`minus_inf -Inf`,
		`not_a_number NaN`,
	}, "\n"))

	if v, ok := mf.Max("plus_inf", nil); !ok || !math.IsInf(v, 1) {
		t.Errorf("plus_inf = %v (ok=%v), want +Inf", v, ok)
	}
	if v, ok := mf.Max("minus_inf", nil); !ok || !math.IsInf(v, -1) {
		t.Errorf("minus_inf = %v (ok=%v), want -Inf", v, ok)
	}
	// A series whose only sample is NaN must report absent, not zero: the
	// difference decides whether a rule is allowed to fire.
	if v, ok := mf.Sum("not_a_number", nil); ok {
		t.Errorf("NaN-only series reported present with value %v, want absent", v)
	}
}

func TestAbsentMetricIsNotZero(t *testing.T) {
	mf := mustParse(t, `present_metric 0`)

	if v, ok := mf.Sum("present_metric", nil); !ok || v != 0 {
		t.Errorf("present zero: got (%v, %v), want (0, true)", v, ok)
	}
	if _, ok := mf.Sum("absent_metric", nil); ok {
		t.Error("absent metric reported as present")
	}
	if _, ok := mf.Max("absent_metric", nil); ok {
		t.Error("absent metric reported as present via Max")
	}
}

func TestLabelSelection(t *testing.T) {
	text := `
vllm:num_requests_waiting{model_name="a",engine="0"} 3
vllm:num_requests_waiting{model_name="a",engine="1"} 4
vllm:num_requests_waiting{model_name="b",engine="0"} 9
vllm:kv_cache_usage_perc{model_name="a",engine="0"} 0.4
vllm:kv_cache_usage_perc{model_name="a",engine="1"} 0.91
`
	mf := mustParse(t, text)

	// Summing across engines is right for a request count...
	if v, ok := mf.Sum("vllm:num_requests_waiting", Labels{"model_name": "a"}); !ok || v != 7 {
		t.Errorf("sum for model a = %v (ok=%v), want 7", v, ok)
	}
	// ...and wrong for a utilisation gauge, where the busiest engine is what
	// matters.
	if v, ok := mf.Max("vllm:kv_cache_usage_perc", Labels{"model_name": "a"}); !ok || v != 0.91 {
		t.Errorf("max KV usage for model a = %v (ok=%v), want 0.91", v, ok)
	}
	if v, ok := mf.Sum("vllm:num_requests_waiting", nil); !ok || v != 16 {
		t.Errorf("unfiltered sum = %v, want 16", v)
	}
	if _, ok := mf.Sum("vllm:num_requests_waiting", Labels{"model_name": "missing"}); ok {
		t.Error("selector matching nothing reported present")
	}
}

// histogram builds an exposition payload with the given cumulative buckets.
func histogram(family string, pairs [][2]string, sum, count string) string {
	var b strings.Builder
	b.WriteString("# TYPE " + family + " histogram\n")
	for _, p := range pairs {
		b.WriteString(family + `_bucket{le="` + p[0] + `"} ` + p[1] + "\n")
	}
	b.WriteString(family + "_sum " + sum + "\n")
	b.WriteString(family + "_count " + count + "\n")
	return b.String()
}

func TestQuantile(t *testing.T) {
	// 5 observations: 2 at or below 1s, 3 more at or below 2s.
	text := histogram("lat_seconds", [][2]string{
		{"1", "2"}, {"2", "5"}, {"+Inf", "5"},
	}, "6", "5")
	mf := mustParse(t, text)

	tests := []struct {
		q    float64
		want float64
	}{
		// rank 2.0 lands exactly on the le=1 boundary
		{q: 0.4, want: 1.0},
		// rank 2.5 is one sixth of the way through the (1,2] bucket
		{q: 0.5, want: 1 + (2-1)*((2.5-2.0)/3.0)},
		{q: 0.9, want: 1 + (2-1)*((4.5-2.0)/3.0)},
	}
	for _, tc := range tests {
		got, ok := mf.Quantile("lat_seconds", tc.q, nil)
		if !ok {
			t.Fatalf("q=%v: no value", tc.q)
		}
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("q=%v: got %v, want %v", tc.q, got, tc.want)
		}
	}

	if _, ok := mf.Quantile("lat_seconds", 0, nil); ok {
		t.Error("q=0 should be rejected")
	}
	if _, ok := mf.Quantile("lat_seconds", 1, nil); ok {
		t.Error("q=1 should be rejected")
	}
	if _, ok := mf.Quantile("missing_family", 0.95, nil); ok {
		t.Error("absent histogram reported a quantile")
	}
}

func TestQuantileInOpenEndedBucketReturnsLargestFiniteBoundary(t *testing.T) {
	// Every observation exceeds the largest finite boundary. There is no
	// information in the payload about how far beyond it they fall, so the
	// honest answer is the boundary itself.
	text := histogram("slow_seconds", [][2]string{
		{"1", "0"}, {"10", "0"}, {"+Inf", "4"},
	}, "999", "4")
	mf := mustParse(t, text)

	got, ok := mf.Quantile("slow_seconds", 0.95, nil)
	if !ok {
		t.Fatal("no value")
	}
	if got != 10 {
		t.Errorf("got %v, want 10 (largest finite boundary)", got)
	}
}

func TestQuantileWithNoObservations(t *testing.T) {
	text := histogram("idle_seconds", [][2]string{{"1", "0"}, {"+Inf", "0"}}, "0", "0")
	mf := mustParse(t, text)
	if _, ok := mf.Quantile("idle_seconds", 0.95, nil); ok {
		t.Error("empty histogram reported a quantile; an idle workload must not look fast")
	}
}

func TestQuantileAggregatesAcrossSeries(t *testing.T) {
	// Two engines, each with its own bucket series. Quantiles must be computed
	// on the merged distribution, not on whichever series happens to be first.
	text := `
# TYPE ttft_seconds histogram
ttft_seconds_bucket{engine="0",le="1"} 4
ttft_seconds_bucket{engine="0",le="+Inf"} 4
ttft_seconds_bucket{engine="1",le="1"} 0
ttft_seconds_bucket{engine="1",le="+Inf"} 4
ttft_seconds_count{engine="0"} 4
ttft_seconds_count{engine="1"} 4
`
	mf := mustParse(t, text)

	// Merged: le=1 -> 4, +Inf -> 8. The median falls inside the le=1 bucket.
	got, ok := mf.Quantile("ttft_seconds", 0.5, nil)
	if !ok {
		t.Fatal("no value")
	}
	if got != 1 {
		t.Errorf("merged median = %v, want 1", got)
	}
	if got, ok := mf.Quantile("ttft_seconds", 0.5, Labels{"engine": "1"}); !ok || got != 1 {
		// Engine 1 has everything above 1s, so the median clamps to the
		// largest finite boundary.
		t.Errorf("engine 1 median = %v (ok=%v), want 1", got, ok)
	}
}

func TestQuantileWithNonMonotonicBuckets(t *testing.T) {
	// A scrape taken mid-update can show a later bucket below an earlier one.
	// Clamping keeps the estimate sane instead of producing a negative
	// bucket population.
	text := histogram("racy_seconds", [][2]string{
		{"1", "10"}, {"2", "6"}, {"+Inf", "12"},
	}, "20", "12")
	mf := mustParse(t, text)

	got, ok := mf.Quantile("racy_seconds", 0.5, nil)
	if !ok {
		t.Fatal("no value")
	}
	if got < 0 || got > 2 || math.IsNaN(got) {
		t.Errorf("got %v, want a finite value within the bucket range", got)
	}
}

func TestTypeComments(t *testing.T) {
	mf := mustParse(t, "# HELP x docs\n# TYPE x counter\nx 1\n")
	if typ, ok := mf.Type("x"); !ok || typ != "counter" {
		t.Errorf("Type(x) = %q, %v; want counter, true", typ, ok)
	}
	if _, ok := mf.Type("y"); ok {
		t.Error("Type reported a family that was never declared")
	}
	if mf.LinesSkipped != 0 {
		t.Errorf("comment lines counted as skipped: %d", mf.LinesSkipped)
	}
}

func TestParseNeverPanics(t *testing.T) {
	inputs := []string{
		"", "   ", "\n\n\n", "{}", "}{", `a{="1"} 2`, `a{b=1} 2`, `a{b="1"`,
		"#", "# TYPE", "# TYPE only_two", strings.Repeat("x", 5000),
		"metric{a=\"\\\"} 1", "\x00\x01\x02",
	}
	for _, in := range inputs {
		mf, err := ParseString(in)
		if err != nil {
			t.Fatalf("input %q: unexpected error %v", in, err)
		}
		_, _ = mf.Sum("anything", nil)
		_, _ = mf.Quantile("anything", 0.5, nil)
	}
}

// vllmPayloadFixture is used by both the benchmark and the vLLM adapter tests.
func benchPayload() string {
	var b strings.Builder
	b.WriteString(`vllm:num_requests_running{model_name="m",engine="0"} 12` + "\n")
	b.WriteString(`vllm:num_requests_waiting{model_name="m",engine="0"} 5` + "\n")
	les := []string{"0.001", "0.005", "0.01", "0.02", "0.04", "0.06", "0.08", "0.1",
		"0.25", "0.5", "0.75", "1.0", "2.5", "5.0", "7.5", "10.0", "20.0", "40.0",
		"80.0", "160.0", "640.0", "2560.0", "+Inf"}
	for i, le := range les {
		b.WriteString(`vllm:time_to_first_token_seconds_bucket{model_name="m",engine="0",le="` +
			le + `"} ` + itoa(i*7) + "\n")
	}
	b.WriteString(`vllm:time_to_first_token_seconds_sum{model_name="m",engine="0"} 812.5` + "\n")
	b.WriteString(`vllm:time_to_first_token_seconds_count{model_name="m",engine="0"} 154` + "\n")
	return b.String()
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func BenchmarkParse(b *testing.B) {
	payload := benchPayload()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		mf, err := ParseString(payload)
		if err != nil {
			b.Fatal(err)
		}
		if _, ok := mf.Quantile("vllm:time_to_first_token_seconds", 0.95, nil); !ok {
			b.Fatal("no quantile")
		}
	}
}
