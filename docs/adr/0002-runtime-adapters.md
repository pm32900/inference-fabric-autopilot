# 0002 — Runtime adapters are pure functions

**Status:** accepted

## Context

Every inference runtime names and shapes its metrics differently. vLLM labels
every series with `model_name` and exposes latency as histograms; Triton labels
by `model` and exposes cumulative microsecond counters with optional summaries;
DCGM labels by device. Rate conversion, counter-reset handling and staleness are
identical across all of them.

## Decision

An adapter is `Parse(payload, modelName) (Reading, error)` and nothing else. It
holds no state, does no I/O, and knows nothing about scheduling or storage.
Everything requiring memory across scrapes lives in the collector.

## Why

The alternative — an adapter that owns its own scrape loop and rate tracking —
means every new runtime reimplements counter-reset handling, and every one gets
it slightly differently. It also makes adapters untestable without HTTP and a
clock.

As written, an adapter test is a fixture and an assertion. That matters more than
it sounds: the vLLM integration in the first version of this project was tested
against a fixture invented from the metric *names*, passed its tests, and would
have returned zeros against a real server, because real vLLM labels every series
and the fixture did not. Fixtures built from the runtime's actual source, checked
by a pure function, are what catch that.

## Consequences

- Adapters report raw counters; the collector converts them to rates.
- An adapter cannot decide that a metric is stale or that a workload is
  unhealthy. It reports what was in the payload and what was missing.
- Adding a runtime is one interface and one registry line.
