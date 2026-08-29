// Package promtext parses the Prometheus text exposition format.
//
// It exists because inference runtimes label every series (vLLM tags each
// metric with model_name and engine, Triton with model and version, DCGM with
// gpu and UUID), and because the latencies that matter for inference — TTFT,
// end-to-end, queue time — are exposed as histograms rather than summaries.
// Looking metrics up by exact line prefix, which is the obvious first
// implementation, silently returns nothing against every one of those.
//
// The parser is deliberately small: it reads samples, not a full metric model.
// Callers select series with a label subset match and aggregate explicitly,
// because the correct aggregation differs per metric — waiting requests across
// engines should be summed, KV-cache utilisation should be maxed.
//
// Reference: https://prometheus.io/docs/instrumenting/exposition_formats/
package promtext

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Sample is one parsed series line: a metric name, its label set, and a value.
// The name is exactly as written, so histogram lines keep their _bucket, _sum
// and _count suffixes.
type Sample struct {
	Name   string
	Labels Labels
	Value  float64
}

// Labels is a series' label set. A nil or empty map means an unlabelled series.
type Labels map[string]string

// Matches reports whether every label in want is present in l with the same
// value. An empty want matches every series.
func (l Labels) Matches(want Labels) bool {
	for k, v := range want {
		if l[k] != v {
			return false
		}
	}
	return true
}

// MetricFamilies holds the samples from one scrape, indexed by metric name.
type MetricFamilies struct {
	samples map[string][]Sample
	types   map[string]string // family name (no suffix) -> TYPE, when declared
	// LinesSkipped counts lines that could not be parsed. A non-zero value
	// against a runtime that is supposed to be supported usually means the
	// exposition format changed, so collectors surface it as a metric rather
	// than swallowing it.
	LinesSkipped int
}

// Parse reads a Prometheus text exposition payload.
//
// Unparseable lines are counted in LinesSkipped and skipped rather than
// failing the whole scrape: a single malformed series from an unrelated
// exporter should not blind the collector to the rest of the payload. Parse
// returns an error only when the reader itself fails.
func Parse(r io.Reader) (*MetricFamilies, error) {
	mf := &MetricFamilies{
		samples: make(map[string][]Sample),
		types:   make(map[string]string),
	}

	sc := bufio.NewScanner(r)
	// Exposition lines are short, but histogram bucket lines with many labels
	// can be long; 1 MiB is well beyond anything a runtime emits per line.
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if name, typ, ok := parseTypeComment(line); ok {
				mf.types[name] = typ
			}
			continue
		}
		s, ok := parseSample(line)
		if !ok {
			mf.LinesSkipped++
			continue
		}
		mf.samples[s.Name] = append(mf.samples[s.Name], s)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("promtext: reading exposition payload: %w", err)
	}
	return mf, nil
}

// ParseString is a convenience wrapper around Parse.
func ParseString(text string) (*MetricFamilies, error) {
	return Parse(strings.NewReader(text))
}

// Type returns the declared type of a metric family ("counter", "gauge",
// "histogram", "summary") and whether a "# TYPE" line was present.
func (m *MetricFamilies) Type(family string) (string, bool) {
	t, ok := m.types[family]
	return t, ok
}

// Names returns every metric name seen, sorted. Useful for diagnostics that
// report which of an expected metric set a target actually exposes.
func (m *MetricFamilies) Names() []string {
	out := make([]string, 0, len(m.samples))
	for n := range m.samples {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Select returns every sample of the named metric whose labels match want.
func (m *MetricFamilies) Select(name string, want Labels) []Sample {
	all := m.samples[name]
	if len(want) == 0 {
		return all
	}
	out := make([]Sample, 0, len(all))
	for _, s := range all {
		if s.Labels.Matches(want) {
			out = append(out, s)
		}
	}
	return out
}

// Sum adds the values of all matching series. The second return is false when
// no series matched, which callers must distinguish from a genuine zero — an
// absent metric and an idle engine are different conditions and only one of
// them should drive a recommendation.
//
// NaN samples are ignored; a series that is entirely NaN reports not-found.
func (m *MetricFamilies) Sum(name string, want Labels) (float64, bool) {
	var total float64
	var found bool
	for _, s := range m.Select(name, want) {
		if math.IsNaN(s.Value) {
			continue
		}
		total += s.Value
		found = true
	}
	return total, found
}

// Max returns the largest value across matching series. Use it for
// utilisation-style gauges where summing across engines or devices would be
// meaningless.
func (m *MetricFamilies) Max(name string, want Labels) (float64, bool) {
	best := math.Inf(-1)
	var found bool
	for _, s := range m.Select(name, want) {
		if math.IsNaN(s.Value) {
			continue
		}
		if s.Value > best {
			best = s.Value
		}
		found = true
	}
	if !found {
		return 0, false
	}
	return best, true
}

// Quantile estimates the q-th quantile (0 < q < 1) of a histogram family from
// its cumulative buckets, aggregating every matching series first.
//
// This is the same linear-interpolation estimate Prometheus itself uses in
// histogram_quantile: the result is only as precise as the bucket boundaries,
// and a value that falls in the +Inf bucket cannot be interpolated at all — in
// that case the largest finite boundary is returned, which understates the true
// quantile. Callers that report the number to a human should say so.
//
// Returns false when the family is absent, has no observations, or has no
// finite buckets.
func (m *MetricFamilies) Quantile(family string, q float64, want Labels) (float64, bool) {
	if q <= 0 || q >= 1 || math.IsNaN(q) {
		return 0, false
	}
	buckets, ok := m.buckets(family, want)
	if !ok || len(buckets) == 0 {
		return 0, false
	}

	total := buckets[len(buckets)-1].count // the +Inf bucket holds the full count
	if total <= 0 {
		return 0, false
	}
	rank := q * total

	// Find the first bucket whose cumulative count reaches the rank.
	idx := sort.Search(len(buckets), func(i int) bool {
		return buckets[i].count >= rank
	})
	if idx == len(buckets) {
		idx = len(buckets) - 1
	}

	upper := buckets[idx].le
	if math.IsInf(upper, 1) {
		// The quantile lands in the open-ended bucket. The best honest answer
		// is the largest finite boundary; anything else is invention.
		for i := len(buckets) - 1; i >= 0; i-- {
			if !math.IsInf(buckets[i].le, 1) {
				return buckets[i].le, true
			}
		}
		return 0, false
	}

	lower := 0.0
	countBelow := 0.0
	if idx > 0 {
		lower = buckets[idx-1].le
		countBelow = buckets[idx-1].count
	}

	span := buckets[idx].count - countBelow
	if span <= 0 {
		return upper, true
	}
	return lower + (upper-lower)*((rank-countBelow)/span), true
}

// SummaryQuantile reads a pre-computed quantile from a summary metric — the
// series carrying a quantile label, as opposed to the cumulative buckets a
// histogram exposes.
//
// The quantile label is matched numerically rather than by string, because
// exporters spell the same quantile differently ("0.95", "0.950000") and a
// string comparison silently returns nothing against half of them. Matching is
// exact to within a small tolerance: a caller asking for p95 should not
// silently receive p99.
//
// Summaries cannot be aggregated across series, so when several match the
// first in payload order is used.
func (m *MetricFamilies) SummaryQuantile(family string, q float64, want Labels) (float64, bool) {
	const tolerance = 1e-6
	for _, s := range m.Select(family, want) {
		raw, ok := s.Labels["quantile"]
		if !ok {
			continue
		}
		got, err := parseValue(raw)
		if err != nil || math.Abs(got-q) > tolerance {
			continue
		}
		if math.IsNaN(s.Value) {
			// Prometheus summaries report NaN for a quantile with no
			// observations in the current window.
			continue
		}
		return s.Value, true
	}
	return 0, false
}

type bucket struct {
	le    float64
	count float64
}

// buckets aggregates <family>_bucket series into a single cumulative
// distribution, summing counts that share an `le` boundary.
func (m *MetricFamilies) buckets(family string, want Labels) ([]bucket, bool) {
	raw := m.Select(family+"_bucket", want)
	if len(raw) == 0 {
		return nil, false
	}

	byLE := make(map[float64]float64, len(raw))
	for _, s := range raw {
		leStr, ok := s.Labels["le"]
		if !ok {
			continue
		}
		le, err := parseValue(leStr)
		if err != nil || math.IsNaN(le) {
			continue
		}
		if math.IsNaN(s.Value) {
			continue
		}
		byLE[le] += s.Value
	}
	if len(byLE) == 0 {
		return nil, false
	}

	out := make([]bucket, 0, len(byLE))
	for le, c := range byLE {
		out = append(out, bucket{le: le, count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].le < out[j].le })

	// Buckets are cumulative and must be non-decreasing. A scrape taken while
	// the exporter is updating can violate that; clamp rather than produce a
	// negative bucket population and a nonsense quantile.
	for i := 1; i < len(out); i++ {
		if out[i].count < out[i-1].count {
			out[i].count = out[i-1].count
		}
	}
	return out, true
}

// parseTypeComment extracts the family name and type from a "# TYPE x counter"
// line.
func parseTypeComment(line string) (name, typ string, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[1] != "TYPE" {
		return "", "", false
	}
	return fields[2], strings.ToLower(fields[3]), true
}

// parseSample parses one series line:
//
//	name value
//	name{a="1",b="2"} value
//	name{a="1"} value 1699999999000
func parseSample(line string) (Sample, bool) {
	name, rest, labels, ok := splitNameAndLabels(line)
	if !ok {
		return Sample{}, false
	}

	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return Sample{}, false
	}
	// fields[1], when present, is the optional millisecond timestamp. It is
	// deliberately ignored: the collector timestamps its own scrapes, and
	// mixing exporter-supplied clocks into rate calculations across hosts is a
	// reliable way to produce negative intervals.
	v, err := parseValue(fields[0])
	if err != nil {
		return Sample{}, false
	}
	return Sample{Name: name, Labels: labels, Value: v}, true
}

// splitNameAndLabels separates the metric name and optional label set from the
// remainder of the line. It scans the label block character by character so
// that braces, commas and spaces inside quoted label values are handled.
func splitNameAndLabels(line string) (name, rest string, labels Labels, ok bool) {
	open := strings.IndexByte(line, '{')
	if open < 0 {
		// Unlabelled: "name value [ts]"
		sp := strings.IndexAny(line, " \t")
		if sp < 0 {
			return "", "", nil, false
		}
		name = strings.TrimSpace(line[:sp])
		if !validMetricName(name) {
			return "", "", nil, false
		}
		return name, line[sp+1:], nil, true
	}

	name = strings.TrimSpace(line[:open])
	if !validMetricName(name) {
		return "", "", nil, false
	}

	labels, end, ok := parseLabels(line[open:])
	if !ok {
		return "", "", nil, false
	}
	return name, line[open+end:], labels, true
}

// parseLabels parses a `{a="1",b="2"}` block starting at s[0] == '{'.
// It returns the labels and the index just past the closing brace.
func parseLabels(s string) (Labels, int, bool) {
	labels := make(Labels)
	i := 1 // skip '{'
	for i < len(s) {
		// Skip separators and whitespace.
		for i < len(s) && (s[i] == ',' || s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i < len(s) && s[i] == '}' {
			return labels, i + 1, true
		}

		keyStart := i
		for i < len(s) && s[i] != '=' && s[i] != '}' {
			i++
		}
		if i >= len(s) || s[i] != '=' {
			return nil, 0, false
		}
		key := strings.TrimSpace(s[keyStart:i])
		i++ // skip '='

		if i >= len(s) || s[i] != '"' {
			return nil, 0, false
		}
		i++ // skip opening quote

		var val strings.Builder
		closed := false
		for i < len(s) {
			c := s[i]
			if c == '\\' && i+1 < len(s) {
				// Exposition format escapes \\, \" and \n inside label values.
				switch s[i+1] {
				case 'n':
					val.WriteByte('\n')
				case '\\':
					val.WriteByte('\\')
				case '"':
					val.WriteByte('"')
				default:
					val.WriteByte(s[i+1])
				}
				i += 2
				continue
			}
			if c == '"' {
				closed = true
				i++
				break
			}
			val.WriteByte(c)
			i++
		}
		if !closed {
			return nil, 0, false
		}
		if key == "" {
			return nil, 0, false
		}
		labels[key] = val.String()
	}
	return nil, 0, false
}

// parseValue parses an exposition value, including the Inf and NaN spellings
// Prometheus permits.
func parseValue(s string) (float64, error) {
	switch s {
	case "+Inf", "Inf", "inf", "+inf":
		return math.Inf(1), nil
	case "-Inf", "-inf":
		return math.Inf(-1), nil
	case "NaN", "nan", "NAN":
		return math.NaN(), nil
	}
	return strconv.ParseFloat(s, 64)
}

// validMetricName applies the exposition format's name grammar, extended with
// ':' which vLLM uses as its family separator (vllm:num_requests_running).
func validMetricName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '_', c == ':':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
