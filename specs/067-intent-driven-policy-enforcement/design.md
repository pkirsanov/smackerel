# Design: 067 Intent-Driven Policy Enforcement

Owner: `bubbles.design`  
Workflow mode: `product-to-planning`  
Status ceiling for this pass: `specs_hardened`  
Source requirements: [spec.md](spec.md)

## Design Brief

**Current State.** Spec 037 guards `internal/agent`, but the intent-driven rules now span scenario YAML, API paths, Telegram adapters, annotation parsing, config handling, ML sidecar code, and assistant transport code. Known risk surfaces include missing `principleAlignment`, prompt growth, request-side regex parsing, user-facing keyword maps, and silent config fallback forms.

**Target State.** Add required guard tests that fail CI with file, line, rule id, policy source, and required owner/action. Exceptions are explicit, ID-based, reviewable, expiration-bound, and ratcheted by a committed baseline. Thresholds come from SST and fail loud when missing.

**Patterns to Follow.** Implement guards as repo tests under `tests/integration/policy/` or nearby existing guard packages, run them through `./smackerel.sh`, parse YAML structurally, use AST/token-aware scanning where useful, and keep terminal output readable without color.

**Patterns to Avoid.** Do not use optional scripts, shell-only grep pipelines, bypass flags, hidden allowlists, hardcoded scenario IDs, or PR-comment-only exceptions. Do not embed policy thresholds as local constants.

**Resolved Decisions.** Guard IDs follow SCN-067-A01..A08. Scenario exceptions use `policyExceptions:`. Source exceptions use a structured annotation. A committed JSON baseline records accepted exception IDs and fails on unreviewed growth or stale entries.

**Open Questions.** `/bubbles.plan` must select the initial `policy.scenario_prompt_max_lines` after the spec 065 weather-prompt reduction.

## Purpose And Scope

This design converts the intent-driven mandate into executable governance. It does not implement the refactors that remove violations; it prevents those patterns from returning.

## Architecture Overview

```text
./smackerel.sh test unit/integration
  -> guard bootstrap loads SST policy keys
  -> scenario YAML guards
  -> Go/Python source guards
  -> config/no-defaults guards
  -> exception baseline guard
  -> terminal/CI report
```

## Capability Foundation

The reusable foundation is `PolicyGuard`:

```go
type Violation struct {
    RuleID string
    RuleName string
    Path string
    Line int
    Detail string
    PolicySource string
    Owner string
}

type Guard interface {
    ID() string
    Run(ctx context.Context, repo Root, cfg PolicyConfig) ([]Violation, error)
}
```

### Variation Axes

| Axis | Values | Enforcement |
|------|--------|-------------|
| Input surface | YAML, Go, Python, config, baseline | parser-specific guards |
| Failure type | violation, bootstrap error, exception delta | stable report schema |
| Exception location | YAML, source annotation, baseline | exception loader |
| Threshold source | SST numeric, fixed vocabulary | config validation |

## Concrete Implementations

| Rule | Guard | Detection Contract |
|------|-------|--------------------|
| G067-A01 | principle alignment | every scenario YAML has valid `principleAlignment` |
| G067-A02 | prompt cap | non-blank `system_prompt` lines fit SST cap |
| G067-A03 | API keyword routing | user-facing API code does not route from regex intent classifiers |
| G067-A04 | free-text keyword maps | Telegram/annotation code does not choose scenarios or interaction classes from maps |
| G067-A05 | Python no-defaults | runtime SST reads in `ml/app` do not use non-empty fallbacks |
| G067-A06 | Go no-defaults | runtime SST reads in `internal` do not fall back to literals |
| G067-A07 | exception ratchet | actual accepted exceptions match committed baseline |
| G067-A08 | SST threshold presence | policy thresholds are required and fail loud |

## Data Model

No database migration.

Baseline contract at `policy-exception-baseline.json` unless planning chooses a governance directory:

```json
{
  "schema_version": "v1",
  "policy": "specs/067-intent-driven-policy-enforcement",
  "exceptions": [
    {
      "id": "G067-A05-ml-main-embedding-model-20260531",
      "rule_id": "G067-A05",
      "path": "ml/app/main.py",
      "owner": "reviewer",
      "reason": "accepted only until owning remediation removes the fallback",
      "expires_on": "2026-06-30"
    }
  ]
}
```

## Exception Annotation Contracts

Scenario YAML:

```yaml
policyExceptions:
  - id: G067-A02-weather-query-v1-20260531
    rule: G067-A02
    owner: reviewer
    reason: migration window while spec 065 lands location_normalize
    expires_on: 2026-06-30
```

Source annotation:

```go
// smackerel:policy-exception id=G067-A03-example rule=G067-A03 owner=reviewer expires=2026-06-30 reason="diagnostic-only parser"
```

Missing metadata or expired exceptions are violations.

## API And Output Contracts

Stable JSON report shape for tests:

```json
{
  "status": "failed",
  "guards_run": 8,
  "violations": [
    {
      "rule_id": "G067-A03",
      "rule_name": "Forbidden keyword routing pattern",
      "path": "internal/api/domain_intent.go",
      "line": 12,
      "detail": "regex intent classifier in user-facing request path",
      "policy_source": "specs/067-intent-driven-policy-enforcement/spec.md"
    }
  ],
  "exceptions": {"baseline_count": 1, "current_count": 1, "delta_status": "unchanged"}
}
```

Plain text mirrors this with labelled rows.

## Configuration

Required SST keys:

| Key | Purpose |
|-----|---------|
| `policy.scenario_prompt_max_lines` | scenario prompt cap |
| `policy.policy_exception_baseline_path` | baseline file path |
| `policy.policy_exception_max_age_days` | exception age limit |
| `policy.intent_bypass_guard_enabled` | strict bool for compiler-bypass guard |

Missing or malformed keys fail bootstrap.

## Security And Compliance

- Guards do not print secret values.
- Exceptions are review metadata, not bypass flags.
- NO-DEFAULTS checks ignore only examples explicitly labelled forbidden.
- The guard suite is read-only.

## Observability And Failure Handling

CI check name: `intent-policy-guard`. Bootstrap errors fail as `bootstrap_error`; they do not degrade to pass.

## Testing And Validation Strategy

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| SCN-067-A01 | integration | `tests/integration/policy/principle_alignment_guard_test.go` | missing block fails with scenario id |
| SCN-067-A02 | integration | `tests/integration/policy/scenario_prompt_cap_guard_test.go` | over-cap prompt fails with counts |
| SCN-067-A03 | unit | `tests/integration/policy/keyword_routing_guard_test.go` | API regex classifier reported |
| SCN-067-A04 | unit | `tests/integration/policy/keyword_map_guard_test.go` | free-text keyword map reported |
| SCN-067-A05 | unit | `tests/integration/policy/no_defaults_python_guard_test.go` | Python fallback reported |
| SCN-067-A06 | unit | `tests/integration/policy/no_defaults_go_guard_test.go` | Go fallback reported |
| SCN-067-A07 | unit | `tests/integration/policy/policy_exception_guard_test.go` | exception growth fails |
| SCN-067-A08 | unit | `internal/config/policy_test.go` | missing policy cap fails loud |

Each guard test needs an adversarial fixture that would fail if the motivated violation returns.

## Alternatives And Tradeoffs

| Option | Decision | Rationale |
|--------|----------|-----------|
| Optional script | Rejected | Required tests are harder to skip |
| Auto-generated baseline as authority | Rejected | Hides exception growth |
| Raw grep only | Rejected | Higher false positives and weaker line evidence |
| No exceptions | Rejected | Some migrations need bounded, reviewable exceptions |

## Risks And Open Questions

| Risk | Mitigation |
|------|------------|
| False positives block diagnostic code | Structured expiring exceptions |
| Baseline becomes stale | Expiration and stale-entry failure |
| Prompt cap too low | Calibrate during planning |
