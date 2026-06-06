# BUG-049-003 — Twitter API alert references wrong metric name + missing operator runbook entries

| Field | Value |
|-------|-------|
| Parent spec | `specs/049-monitoring-stack/` |
| Discovered by | Sweep round 10 (`stochastic-quality-sweep` → `harden-to-doc` mapped from `harden` trigger) |
| Discovered at HEAD | `a697e0db800bfefa8d027d846b7a82f63a9d530b` |
| Severity | medium |
| Class | governance · alert-contract drift · operator-runbook gap |
| Status | open |

## Problem Statement

The harden trigger on spec 049 surfaced two real findings:

### F-049-H10-02: `alerts.yml` references a non-emitted metric name

Spec 056 (Twitter API connector) commit `6912eb5e5` (2026-06-04) appended a
`smackerel-connector-twitter` rule group to `config/prometheus/alerts.yml`.
The `TwitterAPIRetryStorm` rule's `expr:` field references
`smackerel_connector_twitter_api_retries` (no `_total` suffix):

```yaml
# config/prometheus/alerts.yml (line 181-184)
- alert: TwitterAPIRetryStorm
  expr: |
    sum by (endpoint, reason) (rate(smackerel_connector_twitter_api_retries{connector="twitter"}[5m])) > 0.2
  for: 10m
```

The live runtime in `internal/metrics/metrics.go` (line 96) actually emits
`smackerel_connector_twitter_api_retries_total` (with the standard Prometheus
counter `_total` suffix):

```go
// internal/metrics/metrics.go:96
var ConnectorTwitterAPIRetries = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "smackerel_connector_twitter_api_retries_total",
        Help: "Twitter API v2 retry attempts by endpoint and reason",
    },
    []string{"connector", "endpoint", "reason"},
)
```

`internal/deploy/monitoring_alerts_contract_test.go::TestMonitoringAlertsContract_LiveFile`
detects this drift:

```text
contract violation: alert "TwitterAPIRetryStorm" references metric
"smackerel_connector_twitter_api_retries" which is NOT emitted by the
live runtime (not found in internal/metrics/*.go or ml/app/metrics.py)
— either add the instrumentation in the runtime or remove/correct the
alert
```

The contract test is correct: in production, this `TwitterAPIRetryStorm` rule
would silently never fire because the metric name doesn't match anything
Prometheus is collecting. Operators would lose retry-storm visibility on
the Twitter connector.

The two header comment lines (line 160 and 162) also use the unsuffixed
form; they should match the actual emitted names.

### F-049-H10-03: Twitter alerts missing from `docs/Operations.md` Alert Runbook

The same spec 056 commit added two new alerts (`TwitterAPIRateLimitChronicExhaustion`,
`TwitterAPIRetryStorm`) without updating the operator-facing Alert Runbook
table in `docs/Operations.md` (line 1512). The contract test
`TestMonitoringDocsContract_LiveFile` detects this drift:

```text
contract violation: alert "TwitterAPIRateLimitChronicExhaustion" from
config/prometheus/alerts.yml is not mentioned in docs/Operations.md —
an on-call engineer who searches for the alert name MUST find the
runbook row; add the alert to the Alert Runbook table
```

Per spec 049 FR-049-005(e), every alert in `alerts.yml` MUST be named in
the Alert Runbook table so an on-call engineer can look it up. Both
Twitter alerts violate this requirement today.

## Why It Matters

1. **Silent alert failure (F-049-H10-02).** A miswritten metric name in a
   Prometheus rule does NOT raise a build-time error; the rule loads,
   evaluates, and silently returns no series. The retry-storm signal for
   the Twitter connector would never page in production. Spec 056's
   intent is broken by the typo.
2. **Operator runbook gap (F-049-H10-03).** An alert that fires without a
   runbook entry forces the on-call engineer to read the YAML rule file
   to figure out what to do. Spec 049's FR-049-005(e) and design
   "Metrics Access Boundary" section both explicitly state operator
   docs must list every alert.
3. **Cross-spec contract enforcement.** Spec 049's contract tests caught
   the drift exactly as they were designed to. The fix preserves the
   guarantee that monitoring stays in lockstep with the runtime.

## Scenarios (Gherkin)

### SCN-049-B005 — Every alert metric name resolves to an emitted runtime metric

```gherkin
Given the live config/prometheus/alerts.yml
And   the live internal/metrics/*.go + ml/app/metrics.py runtime metric set
When  TestMonitoringAlertsContract_LiveFile walks every alert's expr
Then  every metric referenced is present in the runtime-emitted set
And   the test passes with no "NOT emitted by the live runtime" violations.
```

### SCN-049-B006 — Every alert in alerts.yml is named in docs/Operations.md Alert Runbook

```gherkin
Given the live config/prometheus/alerts.yml
And   the live docs/Operations.md Alert Runbook section
When  TestMonitoringDocsContract_LiveFile cross-references every alert name
Then  each alert name appears at least once in docs/Operations.md
And   the test passes with no "not mentioned in docs/Operations.md" violations.
```

## Out Of Scope

- Splitting the Twitter alerts into their own subsection of the runbook.
  The existing table is the right surface; the two missing rows go there.
- Refactoring `internal/metrics/metrics.go`. The runtime metric names
  are already correct (Prometheus counter convention with `_total`); the
  alert rule is what needs to be aligned.
- Reviewing every other connector alert. This bug is scoped to the two
  failing Twitter alerts surfaced by the round 10 harden trigger.
- Touching spec 056's planning truth. Spec 056 already shipped; the fix
  lands in spec 049-owned files (`alerts.yml` is product surface; the
  Operations.md monitoring section is product surface) per the
  artifact-ownership-routing rule "fix where the broken state lives".

## Acceptance Criteria

1. `config/prometheus/alerts.yml` `TwitterAPIRetryStorm` rule's `expr:`
   field references `smackerel_connector_twitter_api_retries_total`
   (with the `_total` suffix matching the live runtime).
2. The two comment lines in `alerts.yml` (the rule-group header) match
   the actual emitted metric names.
3. `docs/Operations.md` Alert Runbook table contains a row for each of
   `TwitterAPIRateLimitChronicExhaustion` and `TwitterAPIRetryStorm`,
   naming severity, backing metric, and firing action.
4. `go test ./internal/deploy/ -run 'TestMonitoringAlertsContract_LiveFile'
   -count=1` PASSES.
5. `go test ./internal/deploy/ -run 'TestMonitoringDocsContract_LiveFile'
   -count=1` PASSES.
6. `./smackerel.sh test unit --go --go-run 'TestMonitoring'` PASSES
   end-to-end (all 5 monitoring contract files + adversarial sub-tests).
7. `bash .github/bubbles/scripts/artifact-lint.sh
   specs/049-monitoring-stack` still PASSES.
8. `BUBBLES_AGENT_NAME=bubbles.validate bash
   .github/bubbles/scripts/state-transition-guard.sh
   specs/049-monitoring-stack` PASSES.

## Product Principle Alignment

This bug enforces Principle 8 ("Trust Through Transparency") — every
alert must point to a real, observable metric so an operator can trust
the runbook. It also supports Principle 6 ("Invisible by Default, Felt
Not Heard") by ensuring alerts that DO fire are actionable (named in
the runbook with a firing action).

### Single-Capability Justification

This bug is a single data-only correction on two existing product
surfaces (one alert rule and one operator runbook table). It does NOT
introduce a new capability, a second provider/component/variant, or any
adapter/strategy/plugin pattern. Spec 049 (parent) already owns the
singular capability “static monitoring contract for the Smackerel
stack”; the Twitter alerts are amendments published into that one
capability. No multi-implementation foundation is warranted.
