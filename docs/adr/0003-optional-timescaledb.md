# 0003 — TimescaleDB is optional and never on the critical path

**Status:** accepted

## Context

Long-term history is genuinely useful: "was this always this slow" is the second
question anyone asks. The obvious design makes the database the store, and the
rule engine reads from it.

## Decision

The rule engine reads only a bounded in-memory window. The database is a write-only
sink that never blocks and can be absent.

## Why

A diagnostics tool that stops diagnosing when its database is unreachable has
made an outage worse at exactly the moment it was supposed to help. Rules need
minutes of history, not months, so nothing they do requires durable storage.

Second, and just as important: requiring a database to see the project work is a
barrier to evaluating it. `make demo` needs no dependencies at all.

## Consequences

- A database outage costs history, not analysis. `ifa_database_dropped_total`
  says how much.
- The sink is bounded: one writer goroutine behind a fixed-size channel, drop
  and count on overflow. Unbounded goroutines were the original implementation
  and would exhaust memory whenever the database went away.
- Startup *does* fail when the database is enabled but unreachable. Enabling
  history and silently not writing it is worse than a clear failure at boot.
- IFA never runs migrations. `migrations/001_init.sql` is applied by an operator,
  so the running process needs no DDL rights on a database it only appends to.
