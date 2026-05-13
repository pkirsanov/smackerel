# Feature: Monitoring Stack

## Status

In Progress - planning packet created

## Review Findings

- D-014: Monitoring stack readiness is not fully planned as a generic deployment contract.
- SEC-DEP-005: Metrics endpoints and dashboards need access-boundary clarity without embedding target-specific exposure rules.

## Outcome Contract

**Intent:** Provide a concrete, product-aligned monitoring plan for Smackerel deployments without embedding target-specific exposure decisions in the product repo.

**Success Signal:** Smackerel publishes Prometheus scrape configuration, dashboard and alert contracts, and access-boundary documentation showing which metrics are product-owned and which host-level exposure decisions belong to target adapters.

**Hard Constraints:**

- Product artifacts must not encode concrete host exposure decisions.
- Host singleton setup and external exposure remain target adapter responsibilities.
- Monitoring configuration must not introduce hardcoded hostnames or environment-specific paths.

**Failure Condition:** Operators lack a runnable monitoring configuration, alert set, dashboard inventory, or access boundary for metrics endpoints.

## Requirements

- **FR-049-001:** Define Prometheus scrape configuration for Smackerel core and ML sidecar metrics.
- **FR-049-002:** Define dashboards for service health, ingestion, queue depth, ML latency, error rate, and storage pressure.
- **FR-049-003:** Define alert rules for unavailable services, failed ingestion, NATS pressure, DB pressure, ML starvation, and backup failures.
- **FR-049-004:** Document metrics access boundary and target adapter responsibility for host exposure.
- **FR-049-005:** Add tests or static checks that metrics surfaces are not broadly published.

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-049-M01 Operator sees Smackerel metrics scraped
  Given the monitoring profile is enabled
  When Prometheus starts
  Then it scrapes Smackerel core and ML metrics endpoints
  And scrape targets use configured service names and ports

Scenario: SCN-049-M02 Alerts cover deployment failure modes
  Given the runtime stack is unhealthy
  When alert rules evaluate
  Then alerts fire for service unavailability, ingestion failure, queue pressure, database pressure, ML starvation, and backup failure

Scenario: SCN-049-M03 Metrics endpoints remain inside the operator boundary
  Given a target adapter chooses an exposure model
  When metrics endpoint exposure is inspected
  Then product metrics are not broad-bound by default
  And target exposure remains outside the product contract
```

## Product Principle Alignment

This spec supports Principle 6 by making operational state visible only when useful, and Principle 8 by attaching transparent metrics and alerts to failure modes.

## Non-Goals

- Installing host reverse proxies, VPN or network overlays, or firewall rules from Smackerel source.
- Choosing a managed observability vendor.
- Building a custom monitoring UI.
