# Scopes: BUG-075-002 Assistant renderer Node toolchain

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Supply Node inside the sanctioned E2E container

**Status:** Done
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

- [x] Root cause is confirmed in the canonical Go E2E container. → Evidence: [report.md](report.md) "RED: Node absent in sanctioned E2E container" — inside the sanctioned Go E2E container both renderer tests reach the `node web/pwa/lib/render_descriptor_v1_cli.js` step and FAIL with `node not on PATH ... exec: "node": executable file not found in $PATH` (`--- FAIL`, exit 1), confirming the wrapper's tool prerequisites were incomplete for the assistant package; [design.md](design.md) "Root Cause" localizes it to `scripts/runtime/go-e2e.sh` ensuring `envsubst` but not Node.
- [x] Node is supplied and verified inside the repository-managed container. → Evidence: [report.md](report.md) "Live E2E — containerized renderer (current session)" — both legs show `[go-e2e] node missing - installing nodejs inside the tooling container` → `[go-e2e] nodejs install OK` inside the repository-managed Debian Go tooling container; [report.md](report.md) "### Code Diff Evidence" shows `_ensure_node.sh` verifies `node` after `apt-get install --no-install-recommends nodejs` and returns nonzero on failure. No host Node/npm is installed or consulted.
- [x] Open-window renderer regression passes with body-first addendum behavior. → Evidence: [report.md](report.md) "Live E2E — containerized renderer (current session)" — Leg (a) `--- PASS: TestLegacyRetirementNoticeE2E_OpenWindowRendersAddendumWithoutBlockingBody (0.16s)` (isolated, `ISO_E2E_EXIT=0`) and Leg (b) the same test PASS (0.13s) in full-package order; the test asserts the body remains first with the notice appended after it (SCN-001).
- [x] Non-retired adversarial renderer regression passes without a notice node. → Evidence: [report.md](report.md) "Live E2E — containerized renderer (current session)" — Leg (a) `--- PASS: TestLegacyRetirementNoticeE2E_NonRetiredTurnOmitsNotice (0.32s)` and Leg (b) the same test PASS (0.26s); a live ordinary response gains no notice node (SCN-002).
- [x] Missing Node cannot silently pass: removing Node bootstrap or its invocation fails an adversarial source contract or the fatal renderer prerequisite. → Evidence: [report.md](report.md) "Contract Revert-Reverify — load-bearing node bootstrap invocation (current session)" — removing `ensure_node "go-e2e"` makes `TestAssistantE2EPrerequisitesContract_LiveSources` FAIL (`assistant_e2e_package_contract_test.go:126: go-e2e.sh must source _ensure_node.sh and call ensure_node`, exit 1); byte-exact `git checkout HEAD --` restore returns it GREEN (SCN-003). Both renderer tests retain a fatal `exec.LookPath("node")` prerequisite; [report.md](report.md) "## Guards & Quality Gates" RQG `--bugfix` confirms the adversarial signal (`RQG_BUGFIX_EXIT=0`).
- [x] No host install, direct host ecosystem command, skip, bailout, or sleep exists. → Evidence: [report.md](report.md) "### Code Diff Evidence" (Node installed only INSIDE the container via `apt-get` — the container's trusted package source — never a host `node`/`npm` command) + [report.md](report.md) "## Guards & Quality Gates" reality-scan (`REALITY_EXIT=0`, 0 violations) and regression-quality standard (`RQG_STD_EXIT=0`, 0 violations); the retained fatal `exec.LookPath("node")` keeps broken renderer execution a failure rather than a bypass — no `t.Skip`, bailout return, or sleep is present.
- [x] Change Boundary contains every changed file and no excluded surface changes. → Evidence: [report.md](report.md) "## Guards & Quality Gates" git-backed proof — `git show 8ac848e1 --numstat` lists exactly the allowed files (`scripts/runtime/_ensure_node.sh`, `scripts/runtime/go-e2e.sh`, `internal/deploy/assistant_e2e_package_contract_test.go`, `tests/e2e/assistant/legacy_retirement_notice_test.go`, `docs/Testing.md`, `docs/Development.md`); `git status --short` is packet-only. No excluded surface (production images, JS renderer behavior, assistant response contracts, deployment, release trains, secrets, host tooling) is changed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [report.md](report.md) "Live E2E — containerized renderer (current session)" — SCN-001 & SCN-002 are `TestLegacyRetirementNoticeE2E_OpenWindow.../NonRetired...` (live, GREEN in both legs); SCN-003 is the adversarial `TestAssistantE2EPrerequisitesContract_*` source contract (revert-reverify GREEN). [scenario-manifest.json](scenario-manifest.json) maps all 3 scenarios to concrete tests; [report.md](report.md) "## Guards & Quality Gates" traceability-guard `TRACE_EXIT=0` (3 scenarios → 6 rows, G068 fidelity 3/3).
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "Live E2E — containerized renderer (current session)" Leg (b) full assistant package — 62 PASS / 7 SKIP; every in-boundary renderer test + neighboring assistant flow GREEN. The only 2 failures are pre-existing foreign `buildvcs` failures in `intent_replay_test.go` (spec-069 intent-replay), dispositioned in [report.md](report.md) "## Discovered Issues (Gate G095)" DI-075-002-01 — outside this change boundary, working tree packet-only, not a product regression.
- [x] Check, lint, format, artifact, traceability, reality, and regression guards pass. → Evidence: [report.md](report.md) "## Guards & Quality Gates" — `CHECK_EXIT=0`, `FORMAT_EXIT=0`, `LINT_EXIT=0`, `ALINT_EXIT=0` (Artifact lint PASSED), `TRACE_EXIT=0` (PASSED, 0 warnings), `REALITY_EXIT=0` (0 violations, 6 files), `RQG_STD_EXIT=0` (0 violations), `RQG_BUGFIX_EXIT=0` (adversarial signal detected).
- [x] Validate-owned certification records the strongest evidence-supported state. → Evidence: [state.json](state.json) `certification.certifierAgent = bubbles.validate`, `certification.certificationReadiness = ready`, and `certification.certifiedCompletedPhases` records the full 8-phase bugfix-fastlane claim set; terminal `certification.status = done` + `certifiedAt` are stamped only by the validate-owned promote commit after the planning-truth commit (G088). [report.md](report.md) "### Validation Evidence" records the state-transition-guard PASS (`failedGateIds: []`) + artifact-lint exit 0.
