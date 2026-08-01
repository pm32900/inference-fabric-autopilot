# Contributing to P95 Autopilot

Thank you for your interest. P95 Autopilot is an alpha-stage project and contributions
are welcome, with the understanding that the API, data model, and architecture are still
evolving.

## Before you contribute

Please open an issue before starting work on a non-trivial change. This avoids wasted
effort if the change conflicts with the current roadmap direction.

See [docs/ROADMAP.md](./docs/ROADMAP.md) for where the project is heading.

## Project constraints

These are hard constraints. Contributions that violate them will not be merged:

- **Read-only.** P95 Autopilot must never create, update, patch, or delete Kubernetes resources. It observes — it does not act.
- **No autonomous remediation.** The system produces recommendations for humans to act on. It does not apply changes to workloads automatically.
- **No sensitive payload collection.** Do not add code that reads prompt bodies, response bodies, request headers, or user identifiers.
- **Go only** for the control plane, CLI, and node agent. No additional language runtimes.
- **No new required external dependencies** without discussion. The project aims to stay deployable with minimal cluster footprint.

## Development setup

**Requirements:**
- Go 1.21+
- Docker (for building images and running kind)
- kind (for local Kubernetes)
- kubectl
- helm 3+

**Clone and build:**
```bash
git clone https://github.com/<your-username>/inference-fabric-autopilot.git
cd inference-fabric-autopilot
go build ./...
```

**Run tests:**
```bash
go test ./...
go vet ./...
```

**Run locally (no cluster needed):**
```bash
bash scripts/run-local.sh
```

The control plane starts on `http://localhost:8080` in simulated collector mode.
No Kubernetes cluster or database is required for local development.

## Making changes

1. Fork the repository and create a branch from `main`.
2. Name your branch descriptively: `feat/vllm-histogram-support`, `fix/ttft-parsing`, `docs/operations-runbook`.
3. Keep commits focused. One logical change per commit.
4. Run `go test ./...` and `go vet ./...` before pushing.
5. Open a pull request against `main` with a clear description of what changed and why.

## Pull request guidelines

- Describe the problem being solved, not just the implementation.
- Include test coverage for new logic. Table-driven tests are preferred.
- Do not add or remove comments unless that is the explicit purpose of the PR.
- Do not reformat unrelated code in the same PR.
- Keep PRs small and reviewable. Large PRs will be asked to be split.

## Adding a new recommender rule

Rules live in `internal/recommender/recommender.go`. Each rule must:

1. Have a clearly numbered comment (`// ── Rule N: ...`).
2. Be tested in `internal/recommender/recommender_test.go` with both a firing and a non-firing case.
3. Include a `RelatedMetric` field naming the Prometheus metric or Kubernetes field that triggered it.
4. Not fire for runtimes it does not apply to (guard with `snap.Runtime == "vllm"` or similar).

## Adding a new runtime adapter

Runtime-specific parsing lives in `internal/runtime/<runtime-name>/`. Follow the pattern
established in `internal/runtime/vllm/`:

- `metrics.go` — metric name constants and a snapshot struct
- `parser.go` — `Parse(prometheusText string) Snapshot` function
- `parser_test.go` — table-driven tests with fixture payloads

## Code style

- Follow standard Go idioms and `gofmt` formatting.
- No global state outside of `main()`.
- Exported functions must have a doc comment.
- Errors must be wrapped with `fmt.Errorf("context: %w", err)` — do not discard errors.
- Do not log and return an error — choose one.

## Reporting bugs

Use the GitHub issue tracker. Include:
- What you did
- What you expected
- What actually happened
- Go version, OS, Kubernetes version if relevant
- Relevant log output

## License

By contributing, you agree that your contributions will be licensed under the
[Apache 2.0 License](./LICENSE).