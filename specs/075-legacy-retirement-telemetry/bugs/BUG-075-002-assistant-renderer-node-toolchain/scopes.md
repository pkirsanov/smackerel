# Scopes: BUG-075-002 Assistant renderer Node toolchain

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Supply Node inside the sanctioned E2E container

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.test`
**Scope Kind:** test-toolchain bugfix

### Gherkin Scenarios

```gherkin
Feature: Containerized assistant renderer E2E

  Scenario: Open-window notice renders as an addendum
    Given the disposable core returns a live retired-command response
    And Node is available inside the repository E2E container
    When the checked-in PWA renderer processes the response
    Then the body remains first and the notice appears after it

  Scenario: Non-retired turn does not gain a notice node
    Given the disposable core returns a live ordinary response
    And Node is available inside the repository E2E container
    When the checked-in PWA renderer processes the response
    Then the descriptor is unchanged by removing an absent notice key

  Scenario: Missing Node cannot silently pass
    Given the assistant E2E package requires the PWA renderer
    When the container bootstrap no longer provides Node
    Then the harness or renderer tests fail directly
```

### Implementation Plan

1. Add an idempotent Node prerequisite helper for the Go E2E container.
2. Invoke it from the repository E2E wrapper before Go tests start.
3. Add an adversarial source contract for helper and invocation removal.
4. Run both renderer tests, the full assistant package, impacted units, and governance guards.

### Change Boundary

Allowed: `scripts/runtime/_ensure_node.sh`, `scripts/runtime/go-e2e.sh`, focused harness contract tests, docs, and this packet.

Excluded: production images, JS renderer behavior, assistant response contracts, deployment, release trains, secrets, and host tooling.

### Implementation Files

- `scripts/runtime/_ensure_node.sh`
- `scripts/runtime/go-e2e.sh`
- `internal/deploy/assistant_e2e_package_contract_test.go`
- `tests/e2e/assistant/legacy_retirement_notice_test.go`
- `docs/Testing.md`
- `docs/Development.md`

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Node bootstrap contract | `unit` | repository harness contract tests | Detects missing helper, missing call, and non-fail-loud verification | `./smackerel.sh test unit --go --go-run 'AssistantE2E.*Node' --verbose` | No |
| Open-window renderer | `e2e-ui` | `tests/e2e/assistant/legacy_retirement_notice_test.go` | Live response renders body then notice | `./smackerel.sh test e2e --go-package assistant --go-run '^TestLegacyRetirementNoticeE2E_OpenWindow'` | Yes |
| Non-retired adversary | `e2e-ui` | `tests/e2e/assistant/legacy_retirement_notice_test.go` | Live ordinary response gains no notice node | `./smackerel.sh test e2e --go-package assistant --go-run '^TestLegacyRetirementNoticeE2E_NonRetired'` | Yes |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Executes all assistant scenarios inside the sanctioned container | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Confirms all neighboring assistant scenarios | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Static quality | `lint` | changed files | Check, lint, and format | `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` | No |

### Definition of Done

- [ ] Root cause is confirmed in the canonical Go E2E container.
- [ ] Node is supplied and verified inside the repository-managed container.
- [ ] Open-window renderer regression passes with body-first addendum behavior.
- [ ] Non-retired adversarial renderer regression passes without a notice node.
- [ ] Missing Node cannot silently pass: removing Node bootstrap or its invocation fails an adversarial source contract or the fatal renderer prerequisite.
- [ ] No host install, direct host ecosystem command, skip, bailout, or sleep exists.
- [ ] Change Boundary contains every changed file and no excluded surface changes.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Check, lint, format, artifact, traceability, reality, and regression guards pass.
- [ ] Validate-owned certification records the strongest evidence-supported state.
