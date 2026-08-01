# Security Policy

## Status

P95 Autopilot is **alpha-stage software** under active development.
It is not production-hardened and has not undergone a formal security audit.
Use it in non-production or controlled environments only.

## Read-only design guarantee

P95 Autopilot is architecturally read-only with respect to Kubernetes:

- It does **not** create, update, patch, or delete any Kubernetes resource.
- It does **not** exec into pods or run commands inside containers.
- It does **not** modify inference workload configuration.
- It does **not** collect prompt bodies, response bodies, request headers, or user identifiers.
- All Kubernetes API access is via a ClusterRole with only `get`,`list`, and `watch` verbs.

See [docs/SECURITY_MODEL.md](./docs/SECURITY_MODEL.md) for the full threat model and
[docs/RBAC_PERMISSIONS.md](./docs/RBAC_PERMISSIONS.md) for the exact RBAC definition.

## Data collection boundaries

P95 Autopilot collects only infrastructure-level metrics:

- Kubernetes workload metadata (pod name, namespace, runtime, replica count)
- Prometheus metrics from inference workload `/metrics` endpoints (latency, queue depth, token throughput, KV cache usage)
- No user-identifying data, no request payloads, no model inputs or outputs

See [docs/DATA_COLLECTION.md](./docs/DATA_COLLECTION.md) for the complete field-level inventory.

## Supported versions

No formal support policy exists yet. This project follows a rolling alpha model.
Security fixes will be applied to the latest commit on `main`.

| Version | Supported |
|---|---|
| `main` (latest) | ✅ Best effort |
| Any tagged release | ⚠️ No formal SLA |

## Reporting a vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

To report a security issue:

1. Email the maintainer directly. The contact address is in the GitHub profile associated with this repository.
2. Include a description of the vulnerability, steps to reproduce, and potential impact.
3. Allow up to 14 days for an initial response.

We will acknowledge receipt, assess the report, and coordinate disclosure.
For alpha-stage software, we aim to patch and release fixes quickly rather than follow a long embargo window.

## Known limitations and accepted risks

The following are known alpha-stage limitations, not vulnerabilities:

- **No mTLS between components.** The control plane HTTP API is unauthenticated by default. Deploy behind a Kubernetes Service with network-level access controls.
- **Default database password.** `config.yaml` ships with a placeholder DSN (`postgres://postgres:autopilot@...`). Change this before any deployment that exposes the DB.
- **No RBAC on the HTTP API.** The `/telemetry`, `/recommendations`, and `/workloads` endpoints do not require authentication. Restrict access via NetworkPolicy or ingress rules.
- **Prometheus scrape targets are not authenticated.** Metrics URLs in config are fetched over plain HTTP. Use in-cluster service names to avoid exposure.

## Security-relevant configuration

```yaml
# Set deployment_mode to airgapped to disable egress at the config layer
deployment_mode: airgapped

egress:
  enabled: false

# Privacy flags — all false by default and enforced at startup
privacy:
  collect_prompt_bodies: false
  collect_response_bodies: false
  collect_headers: false
  collect_user_identifiers: false

# RBAC posture — only valid value is read-only
rbac:
  mode: read-only