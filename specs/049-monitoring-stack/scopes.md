# Scopes: Monitoring Stack

Links: [spec.md](spec.md) | [design.md](design.md)

## Scope 1: Prometheus scrape and metrics access boundary

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-049-M01 Operator sees Smackerel metrics scraped
  Given the monitoring profile is enabled
  When Prometheus starts
  Then it scrapes Smackerel core and ML metrics endpoints
  And scrape targets use configured service names and ports

Scenario: SCN-049-M03 Metrics endpoints remain inside the operator boundary
  Given a target adapter chooses an exposure model
  When metrics endpoint exposure is inspected
  Then product metrics are not broad-bound by default
  And target exposure remains outside the product contract
```

### Implementation Plan

1. Define Prometheus scrape configuration for core and ML sidecar metrics.
2. Add static checks for target names, ports, and bind exposure.
3. Add docs explaining product metrics contracts vs target adapter host exposure.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-049-001 | config-static | monitoring config tests | SCN-049-M01 | Prometheus scrape config includes core and ML targets. |
| T-049-002 | integration | disposable monitoring stack | SCN-049-M01 | Prometheus can scrape both endpoints. |
| T-049-003 | config-static | compose/deploy tests | SCN-049-M03 | Metrics endpoints are not broadly published. |

### Definition of Done

- [ ] T-049-001 passes and proves scrape targets are configured.
- [ ] T-049-002 passes and proves metrics can be scraped in disposable runtime.
- [ ] T-049-003 passes and proves metrics exposure stays inside the operator boundary.

## Scope 2: Dashboards, alerts, and docs

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: SCN-049-M02 Alerts cover deployment failure modes
  Given the runtime stack is unhealthy
  When alert rules evaluate
  Then alerts fire for service unavailability, ingestion failure, queue pressure, database pressure, ML starvation, and backup failure
```

### Implementation Plan

1. Define dashboard inventory for health, ingestion, queues, ML, storage, and backup status.
2. Add alert rules for the failure modes named in the scenario.
3. Document operator response and ownership boundaries.

### Test Plan

| ID | Test Type | Location | Scenario | Assertion |
|----|-----------|----------|----------|-----------|
| T-049-004 | config-static | alert rule tests | SCN-049-M02 | Required alert rules exist and have actionable labels. |
| T-049-005 | docs-static | operations docs | SCN-049-M02 | Dashboard and alert ownership are documented. |
| T-049-006 | artifact | spec folder | all | Artifact lint passes for this feature. |

### Definition of Done

- [ ] T-049-004 passes and proves required alert rules exist.
- [ ] T-049-005 passes and docs describe dashboards, alerts, and ownership.
- [ ] T-049-006 passes and this planning packet remains lint-clean.
