# Smackerel Testing Guide

This guide defines what Smackerel must validate today and what the runtime must validate once implementation lands.

## Current Bootstrap Validation

Until the runtime is committed, validation is limited to the Bubbles framework and project-owned docs/spec artifacts.

| Test type | Command | Required when |
|-----------|---------|---------------|
| Framework doctor | `bash .github/bubbles/scripts/cli.sh doctor` | Project-owned bootstrap docs change |
| Framework validate | `timeout 1200 bash .github/bubbles/scripts/cli.sh framework-validate` | Before claiming bootstrap health |
| Artifact lint | `bash .github/bubbles/scripts/artifact-lint.sh specs/<feature>` | Spec or bug artifacts change |
| Traceability guard | `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/<feature>` | Traceability-sensitive artifact content changes |
| Regression baseline guard | `timeout 600 bash .github/bubbles/scripts/regression-baseline-guard.sh specs/<feature> --verbose` | Managed docs or competitive baseline content changes |

## Required Runtime Test Matrix

When runtime code lands, the repo must expose all test categories through the single CLI.

| Test type | Category | Required command |
|-----------|----------|------------------|
| Go unit | `unit` | `./smackerel.sh test unit --go` |
| Python unit | `unit` | `./smackerel.sh test unit --python` |
| Integration | `integration` | `./smackerel.sh test integration` |
| End-to-end API | `e2e-api` | `./smackerel.sh test e2e` |
| End-to-end UI | `e2e-ui` | `./smackerel.sh test e2e` |
| Stress | `stress` | `./smackerel.sh test stress` |

If a web UI is committed, UI-specific tests must stay behind the same CLI rather than introducing ad-hoc browser or package-manager commands into project docs.

## Environment Isolation Rules

### Development State Is Sacred

The persistent development stack exists for manual work only.

- It uses named volumes.
- It must survive CLI restarts.
- It must never be the target for automated E2E, integration, chaos, or validation writes.

### Test State Must Be Disposable

The automated test environment must use ephemeral storage.

- PostgreSQL test data should use `tmpfs` or disposable volumes.
- JetStream or queue state used by tests should be disposable.
- Extracted artifact scratch data and temp uploads should be disposable.
- Tests should create uniquely identifiable synthetic fixtures.

### Validation And Chaos Must Be Isolated

Certification, validation, and chaos runs must use isolated runtime state.

- Use a separate Compose project name.
- Use disposable stores.
- Never tear down another active session's runtime implicitly.

## E2E Requirements

Smackerel must adopt the same live-stack standards as the stronger repos.

### Live Stack Only

- `integration`, `e2e-api`, and `e2e-ui` must hit the real running stack.
- Request interception in live categories is forbidden.
- If a test uses interception or canned responses, it must be reclassified out of live categories.

### E2E Uses The Test Stack Only

`./smackerel.sh test e2e` must boot or attach to the ephemeral test stack, never the persistent dev stack.

Required behavior:

- Start disposable test storage.
- Run migrations or schema setup against the test store.
- Seed only synthetic test data.
- Start the runtime against the test environment.
- Execute live-stack E2E coverage.
- Tear down or reset disposable state safely.

### Bug Fixes Need Adversarial Regressions

Every bug fix regression must include at least one case that would fail if the bug returned.

- Tautological fixtures are forbidden.
- Silent-pass bailout logic is forbidden.
- Missing required controls or redirects must fail loudly.

## Verification Standards

Smackerel inherits the Bubbles evidence rules:

- Pass/fail claims require executed commands.
- Test evidence must include raw command output, not summaries.
- Long-running commands must use explicit timeouts.

When runtime code is committed, update this file, the command registry, and copilot instructions in the same change set so the documented test surface matches reality.