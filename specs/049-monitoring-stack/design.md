# Design: Monitoring Stack

## Current Truth

Smackerel has runtime health checks and metrics surfaces, but the readiness review found that scrape configuration, dashboards, alerts, and access-boundary docs need to be made concrete as generic deployment contracts.

## Proposed Design

### Prometheus and Metrics

- Add product-owned Prometheus scrape examples or a monitoring profile for core and ML metrics endpoints.
- Ensure targets use generated service names and ports rather than environment-specific hostnames.
- Add static checks preventing broad metrics binds.

### Dashboards and Alerts

- Define dashboards for API health, ingestion throughput, NATS pressure, ML latency, Postgres pressure, connector failures, and backup health.
- Define alert rules for actionable failure classes.

### Access Boundary

- Document that product metrics are exposed through product-owned metrics contracts and service names.
- Leave host singleton exposure details and external operator access to target adapters.

## Test Strategy

| Test ID | Type | Purpose |
|---------|------|---------|
| T-049-001 | config-static | Prometheus scrape config includes core and ML targets. |
| T-049-002 | integration | Prometheus can scrape the runtime metrics endpoints in a disposable stack. |
| T-049-003 | config-static | Metrics endpoints are not exposed on a broad public bind. |
| T-049-004 | docs-static | Docs explain product-vs-adapter monitoring ownership. |
| T-049-005 | artifact | Artifact lint passes for this feature. |

## Risk Controls

- Do not publish metrics endpoints broadly.
- Keep concrete hostnames, target addresses, and operator-network identifiers out of Smackerel artifacts.
- Alerts must be actionable and avoid pure status noise.
