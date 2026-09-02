# Development

## Prerequisites

Go 1.24 or newer. That is the whole list for building, testing and running the
demo. Helm and Docker are needed only for the chart and image targets.

## The loop

```bash
make demo        # build and run against the simulated fleet — start here
make test        # all tests
make test-race   # the same under the race detector
make lint        # gofmt -s and go vet
make verify      # everything CI runs
make cover       # coverage.html
make bench       # parsing and rule-evaluation benchmarks
```

`make demo` is the fastest way to see a change in effect: it exercises the
parser, the collector, the rate tracker and the rule engine end to end.

## Layout

```
cmd/control-plane      wiring and lifecycle
cmd/ifa                CLI; talks only to the HTTP API

internal/promtext      Prometheus text-format parser
internal/runtime       the adapter contract
  ../vllm ../triton ../dcgm
internal/collector     scrape loop, rate conversion
internal/telemetry     measurement model and in-memory store
internal/recommender   rules and evaluation
internal/k8s           discovery via informers
internal/api           HTTP API
internal/metrics       IFA's own metrics
internal/storage/timescale
internal/demo          simulated fleet
```

Everything is under `internal/`. Nothing here is a stable library yet, and
promising an API surface before the tool has users is a promise you end up
keeping for no one.

## Adding a rule

One function in `internal/recommender/rules.go`, one line in `DefaultRules()`,
one constant for the code.

```go
func myRule() Rule {
    return Rule{
        Code:     CodeMyRule,          // permanent; documented in RECOMMENDATIONS.md
        Title:    "Short, specific",
        Severity: telemetry.SeverityWarning,
        Summary:  "One line for /api/v1/rules and the docs.",
        Runtimes: []telemetry.Runtime{telemetry.RuntimeVLLM}, // omit for all
        Supersedes: []Code{CodeSomethingGeneric},
        Eval: func(e *Eval) *Finding {
            t := e.T
            if !e.Sustained(func(s telemetry.Snapshot) bool {
                return s.SomeMetric.Above(t.SomeThreshold)
            }) {
                return nil
            }
            return &Finding{
                Window:      e.Span(),
                Explanation: "What was observed and why it matters.",
                Action:      "What a human would do next.",
                Evidence: []telemetry.Evidence{
                    evidence("some_metric", sourceMetric(e.Latest.Runtime, "some_metric"),
                        e.Latest.SomeMetric.Value, t.SomeThreshold, ">", "percent"),
                },
            }
        },
    }
}
```

What a good rule does:

- **Uses `Above` / `Below`, never `>` on a raw value.** An unmeasured metric must
  never satisfy a comparison. The `Metric` type enforces this; bypassing it is
  how a rule starts firing on missing data.
- **Combines signals where a single one is ambiguous.** "The queue is deep" is
  not actionable. If your rule would fire on both of two situations with opposite
  fixes, it needs a second signal.
- **Requires the condition to persist** unless it is genuinely instantaneous.
- **Carries evidence** — the observed value, the threshold, the comparison, and
  the source metric.
- **Suggests an action a human can take**, not a restatement of the trigger.
- **Supersedes the generic rule it explains**, so the operator reads one finding.

What to test: it fires at the boundary, it does not fire just below, it does not
fire when a required input is unmeasured, it does not fire on a single spike, and
the healthy control still produces nothing. `internal/recommender/recommender_test.go`
has the helpers.

Then document it in [RECOMMENDATIONS.md](RECOMMENDATIONS.md) — the catalogue is
part of the feature, not a follow-up.

## Adding a runtime

Implement `internal/runtime.Adapter` and add one line to the `adapters` map in
`internal/collector/collector.go`.

`Parse` is a pure function: no I/O, no clock, no state. Rate conversion,
counter-reset handling and staleness live in the collector, so you get them for
free and cannot get them subtly wrong.

**Build the fixture from the runtime's own metric definitions** — its source or
its reference documentation — not from the metric names. Include the labels it
actually emits. This is not a style preference: the original vLLM adapter in this
repository was tested against a fixture invented from the names, passed
everything, and returned nothing but zeros against a real server, because real
vLLM labels every series and the fixture did not.

Cover, at minimum: a complete payload, an empty payload (everything unmeasured,
nothing zero), a partial payload (missing metrics reported), a malformed payload
(counted and survived), and multiple models if the runtime supports them.

## Working on the demo

`internal/demo` serves vLLM-shaped exposition from a simulated engine. Each
scenario declares the rule code it exists to demonstrate, and
`TestDemoScenariosProduceTheirIntendedDiagnosis` asserts it still does — so a
scenario cannot quietly stop reproducing its failure mode and turn the demo into
a fleet of healthy workloads.

`demo.NewManualServer` gives a server with no background clock, so a test can
step through minutes of simulated time in milliseconds.

Keep exactly one healthy control scenario. A demo in which everything is on fire
only proves the thresholds are low.

## Conventions

- `gofmt -s`. CI fails on unformatted files.
- Comments explain *why*, not *what*. If a comment restates the line below it,
  delete one of them.
- Errors are wrapped with `%w` and enough context to locate the failure without
  a stack trace.
- Exported identifiers have doc comments; unexported ones have them when the
  reasoning is not obvious.
- No new dependencies without a reason in the pull request. The runtime
  dependency set is client-go, pgx and yaml, and it should stay small.

## Building the chart and image

```bash
make helm-lint
make docker
```

The chart's `config` block is validated against the Go config loader by a test,
so a field renamed in one and not the other fails the build rather than
producing a pod that crash-loops.
