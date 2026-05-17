# Recipe: Impact-Aware Validation + Trace Contracts

> *"Check the map before you drive the whole park."*

Use this when a repo wants faster narrow-first validation without weakening Bubbles' final completion gates.

## What This Adds

`testImpact` maps changed paths to components, canonical test categories, always-run checks, and full-suite triggers.

`traceContracts` maps important workflows to required trace/log evidence: spans, attributes, invariants, and red flags.

Both live in project-owned config, so framework upgrades do not overwrite them.

## Add the Project Config

Create or update `.github/bubbles-project.yaml`:

```yaml
testImpact:
  alwaysRun:
    - artifact-lint
    - state-transition-guard
  fullSuiteTriggers:
    - "proto/**"
    - "migrations/**"
  components:
    api:
      paths:
        - "backend/api/**"
        - "services/gateway/**"
      testCategories:
        - unit
        - integration
        - e2e-api
      alwaysRun:
        - contract-check
    web:
      paths:
        - "web/**"
        - "frontend/**"
      testCategories:
        - ui-unit
        - e2e-ui

traceContracts:
  workflows:
    booking.create:
      requiredSpans:
        - name: http.request
          attributes:
            - trace_id
            - booking.id
      requiredAttributes:
        - tenant.id
      requiredInvariants:
        - booking emitted exactly one confirmation event
      redFlags:
        error:
          - Missing trace_id
        warning:
          - slow span
```

## Generate a Validation Plan

```bash
bash .github/bubbles/scripts/test-impact-plan.sh \
  --changed-file-list changed-files.txt
```

Useful variants:

```bash
bash .github/bubbles/scripts/test-impact-plan.sh \
  --format json \
  --changed-from origin/main

bash .github/bubbles/scripts/test-impact-plan.sh \
  --require-config \
  --changed-file-list changed-files.txt
```

## Validate Trace Evidence

```bash
bash .github/bubbles/scripts/trace-contract-guard.sh \
  --workflow booking.create \
  --trace-output trace-output.log
```

Run without `--workflow` to check all configured contracts against the same evidence file.

## Workflow Rules

- `testImpact` answers what should run first, not what can be skipped.
- Full-suite triggers always win over narrow-first planning.
- Scenario-specific E2E, regression, stress, outcome-contract, and state-transition gates still apply.
- `traceContracts` validate actual trace/log output only. Code inspection is not trace evidence.
- Analyst-owned Success Signals stay business-observable and tech-agnostic. Design, test, and validate translate those signals into spans, attributes, and invariants.

## When It Helps

| Situation | Use |
|-----------|-----|
| Large repo, small change | Run the mapped narrow-first categories before the broader closeout suite |
| Shared schema or migration changed | Trigger full validation automatically |
| Runtime observability matters | Require trace/log evidence for the changed workflow |
| Flaky failure triage | Confirm whether the trace proves the expected invariant or reveals a red flag |

## Selftests

Framework maintainers can verify the scripts directly:

```bash
bash bubbles/scripts/test-impact-plan-selftest.sh
bash bubbles/scripts/trace-contract-guard-selftest.sh
```

`framework-validate.sh` runs both selftests in the Bubbles source repo.
