package telemetry

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

// Metric is an optional measurement.
//
// The distinction between "not measured" and "measured as zero" is load-bearing
// here. A vLLM deployment that exposes no GPU-utilisation metric is not a GPU
// sitting at 0% idle, and a runtime that has served no requests yet has no p95
// latency rather than a p95 of zero. Collapsing the two — which a plain float64
// forces you to do — makes threshold rules fire on absent data, which is the
// fastest way to make a diagnostics tool untrustworthy.
//
// The zero value is an unmeasured Metric, so a Snapshot that a collector only
// partially fills is correct by default.
type Metric struct {
	Value float64
	OK    bool
}

// Observed returns a Metric carrying a measured value. NaN and infinities are
// treated as unmeasured: they are what a division by an empty counter produces,
// and they must never reach a comparison.
func Observed(v float64) Metric {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return Metric{}
	}
	return Metric{Value: v, OK: true}
}

// ObservedIf returns a measured Metric when ok, and an unmeasured one otherwise.
// It pairs with the (value, ok) results returned by the exposition parser.
func ObservedIf(v float64, ok bool) Metric {
	if !ok {
		return Metric{}
	}
	return Observed(v)
}

// Or returns the measured value, or def when the metric was not measured.
// Use it for display, never for rule evaluation.
func (m Metric) Or(def float64) float64 {
	if !m.OK {
		return def
	}
	return m.Value
}

// Above reports whether the metric was measured and exceeds threshold.
// An unmeasured metric is never above anything, which is what keeps rules from
// firing on missing data.
func (m Metric) Above(threshold float64) bool {
	return m.OK && m.Value > threshold
}

// Below reports whether the metric was measured and is under threshold.
func (m Metric) Below(threshold float64) bool {
	return m.OK && m.Value < threshold
}

// String renders the value for logs and CLI output, or "-" when unmeasured.
func (m Metric) String() string {
	if !m.OK {
		return "-"
	}
	return strconv.FormatFloat(m.Value, 'f', -1, 64)
}

// MarshalJSON emits the bare number, or null when the metric was not measured,
// so API consumers see the same distinction the rule engine does.
func (m Metric) MarshalJSON() ([]byte, error) {
	if !m.OK {
		return []byte("null"), nil
	}
	return json.Marshal(m.Value)
}

// UnmarshalJSON accepts a number or null.
func (m *Metric) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*m = Metric{}
		return nil
	}
	var v float64
	if err := json.Unmarshal(b, &v); err != nil {
		return fmt.Errorf("telemetry: decoding metric: %w", err)
	}
	*m = Observed(v)
	return nil
}
