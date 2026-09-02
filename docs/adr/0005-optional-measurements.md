# 0005 — A measurement is an optional value, not a float

**Status:** accepted

## Context

The natural Go representation of a telemetry snapshot is a struct of `float64`.
It is also wrong, and the failure is silent.

A runtime that does not export GPU utilisation is not a GPU sitting at 0% idle.
A server that has served no requests has no p95 latency, not a p95 of zero. With
a plain `float64` those are the same value, and every threshold rule reads them
as real measurements.

The concrete consequences in the earlier version of this project: the
low-GPU-utilisation rule fired on every workload without a DCGM endpoint; the
low-token-throughput rule fired on every idle workload; and a scrape that failed
wrote a zeroed snapshot that made a workload IFA could not reach look perfectly
healthy.

## Decision

Every measurement is a `telemetry.Metric`: a value plus a flag saying whether it
was measured. Comparisons on an unmeasured Metric are always false. It marshals
to JSON as `null`, and to SQL as `NULL`.

## Why

It makes the distinction impossible to lose by accident. `Above(threshold)`
cannot fire on missing data, because the type will not let it. A rule author does
not have to remember.

The zero value is "not measured", so a snapshot that a collector only partially
fills is correct by default rather than correct only if every field was
explicitly set.

## Consequences

- Rules read `m.Above(x)` rather than `m > x`, which is a small cost.
- API consumers see `null` and can distinguish a gap from a zero. The CLI prints
  `-` and says so under the table.
- History rows store `NULL`, so `AVG()` over a column skips unmeasured samples
  instead of averaging in zeros.
- A whole class of rule appears for free: if a value can be missing, IFA can say
  so, which is what `IFA-OBS-001` does.
