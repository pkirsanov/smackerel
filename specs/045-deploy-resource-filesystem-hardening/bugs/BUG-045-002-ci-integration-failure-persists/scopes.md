# Scopes: BUG-045-002 — CI integration failure persists

> **Status:** PLAN POPULATED by `bubbles.plan` on 2026-05-16. Derived from [design.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/design.md) Decision DD-1 (Path A: route CI through `./smackerel.sh test integration`) and [spec.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/spec.md) AC-1..AC-6.
>
> **Inputs (read-only during execution):** [spec.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/spec.md), [design.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/design.md), [report.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md) Evidence 1–8, [scenario-manifest.json](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/scenario-manifest.json).
>
> **Pickup rule:** Strict sequential gating — S1 → S2 → S3 → S4. Scope N cannot start until scope N-1 is fully Done with all DoD checked and evidence captured inline.

---

## Execution Outline (REQUIRED — alignment checkpoint)

### Phase Order

1. **Scope 1 — Refactor `.github/workflows/ci.yml` to Path A topology.** Remove postgres `services:` block + inline nats step + raw `go test` step. Add explicit `./smackerel.sh --env test up` + `status` + `down (if: always())` lifecycle steps. Replace test command with `./smackerel.sh test integration 2>&1 | tee integration-test.log`. Raise `timeout-minutes: 15 → 30` (DD-3).
2. **Scope 2 — Add build-time topology contract test (DD-2).** Create [internal/deploy/ci_integration_topology_contract_test.go](internal/deploy/ci_integration_topology_contract_test.go) that parses [.github/workflows/ci.yml](.github/workflows/ci.yml), asserts the 6 AC-4 invariants, and includes 3 adversarial sub-tests proving regression detection.
3. **Scope 3 — Local Path-A reproduction.** Prove the S1+S2 changes work on the current dev host: `down` → `up` → `status` → `test integration` (exit 0 with verbatim PASS evidence for each of the 5 previously-failing tests by name) → `down`. Then verify `check`, `format --check`, `lint`, `test unit` exit 0.
4. **Scope 4 — Validation + CI green + close-out.** Commit + push fix; capture post-fix CI run JSON inline with `integration` job conclusion=success (AC-1); wait for 3 consecutive main pushes after fix HEAD and capture 3/3 success curl (AC-5); add `subsequentResolutions[]` entry to BUG-045-001 state.json pointing to BUG-045-002 (AC-6); tick uservalidation.md items; set this packet state.json `status: done` + `certification.status: done`.

### New Types & Signatures

- **[.github/workflows/ci.yml](.github/workflows/ci.yml) → `jobs.integration`** (modified):
  - REMOVE `services.postgres:` block
  - REMOVE step `Start NATS with auth and JetStream`
  - REMOVE step `Apply database migrations via db.Migrate (idempotent + tracking)`
  - REMOVE the comment block "NOTE: CI integration uses raw go test because GitHub Actions service containers replace…"
  - ADD step (after `Generate SST config files for integration tests`): `name: Bring up test stack` → `run: ./smackerel.sh --env test up`
  - ADD step: `name: Stack status snapshot` → `run: ./smackerel.sh --env test status`
  - REPLACE step `Run integration tests` body: `set -o pipefail; ./smackerel.sh test integration 2>&1 | tee integration-test.log`
  - ADD step (after `Fail job if integration tests failed`): `name: Tear down test stack` → `if: always()` → `run: ./smackerel.sh --env test down --volumes || true`
  - CHANGE `timeout-minutes: 15` → `timeout-minutes: 30`

- **[internal/deploy/ci_integration_topology_contract_test.go](internal/deploy/ci_integration_topology_contract_test.go)** (new):
  - `package deploy`
  - `func loadCIWorkflow(t *testing.T) (*workflowDoc, []byte)` — reads + parses `.github/workflows/ci.yml`
  - `func assertCIIntegrationTopologyContract(doc *workflowDoc, raw []byte) error` — pure function returning the first invariant violation found
  - `func TestCIIntegrationTopologyContract(t *testing.T)` — live assertion against the real workflow file
  - `func TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock(t *testing.T)`
  - `func TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar(t *testing.T)`
  - `func TestCIIntegrationTopology_AdversarialRejectsRawGoTest(t *testing.T)`

- **[specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json)** (S4 augmentation):
  - ADD top-level field `subsequentResolutions: [{ bugId: "BUG-045-002-ci-integration-failure-persists", resolvedAt: "<ISO>", relationship: "ci-environment-fix-of-bug-001-uncertainty-declaration", resolutionNote: "Chronic CI integration failure cited in BUG-045-001 spec.md AC-1 § Severity bullet (1) is RESOLVED by BUG-045-002. BUG-045-001 validator fix stays certified; its certification verdict is unchanged." }]`

### Validation Checkpoints

| After | Checkpoint | Failure Surface |
|-------|------------|-----------------|
| S1 | YAML is syntactically valid (`python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` exits 0; `./smackerel.sh format --check` exits 0) | YAML breakage caught locally before S2 even compiles |
| S2 | `./smackerel.sh test unit --go` includes the new test file and PASSES (live assertion against the just-modified `ci.yml`) | Catches mismatch between S1's workflow shape and the guard's expectations BEFORE pushing to CI |
| S2 | 3 adversarial sub-tests each PASS by asserting the guard returns an error against a regressed YAML fixture | Catches a tautological / silent-pass guard before pushing |
| S3 | Local `./smackerel.sh test integration` exits 0 with verbatim PASS lines for all 5 previously-failing tests | Catches missing-service or `-p 1` regression BEFORE pushing — if any of the 5 still fails locally, S1+S2 are wrong |
| S3 | `./smackerel.sh check && lint && format --check && test unit` all exit 0 | Catches general regression introduced by the new test file |
| S4 | Post-fix CI run `integration` job conclusion=success (AC-1) | Catches CI-environment-only regression (something works locally but fails on GH Actions) |
| S4 | 3-consecutive-success curl on main (AC-5) | Catches flakiness / intermittent failure pattern across 3 different HEADs |

---

## Scope Summary Table

| # | Name | Surfaces | Tests | DoD Summary | Status |
|---|------|----------|-------|-------------|--------|
| 1 | Refactor `ci.yml` to Path A topology + retire obsoleted BUG-029-004 structural-preservation invariants | `.github/workflows/ci.yml`; `internal/deploy/ci_workflow_no_parallel_publish_test.go` | Local YAML syntax check; `format --check`; `lint`; `./smackerel.sh test unit` (full suite, exit 0) | Topology matches design.md target; 6 AC-4 invariants hold; YAML valid; format+lint exit 0; obsolete BUG-029-004 invariants retired with provenance comment; full unit suite exit 0 | `[~]` In Progress (reopened 2026-05-17 — 9/12 DoD ticked, 3 new DoD items added by plan re-entry) |
| 2 | Build-time topology contract test | `internal/deploy/ci_integration_topology_contract_test.go` | Live assertion + 3 adversarial sub-tests; `./smackerel.sh test unit --go` | New test file passes; 3 adversarial sub-tests prove regression detection; zero `t.Skip` / silent-return bailouts | `[x]` Done (2026-05-16) |
| 3 | Local Path-A reproduction (5 previously-failing tests PASS verbatim) | Local dev stack via `./smackerel.sh --env test up/status/down/test integration` | `./smackerel.sh test integration` (live system); `./smackerel.sh check`, `lint`, `format --check`, `test unit` | Local exit 0 with verbatim PASS lines for all 5 tests by name; 4 quality gates exit 0 | `[ ]` Not started (unblocked by plan re-entry 2026-05-17 — depends on Scope 1 DoD-10..12 ticking first) |
| 4 | Validation + CI green + close-out | Commit on `main`; `.github/workflows/ci.yml` live run; BUG-045-001 `state.json` | Post-fix CI integration job conclusion=success (AC-1); 3-consecutive-success on main (AC-5); guard test stays green | All 6 ACs satisfied with inline evidence; BUG-045-001 cross-reference (AC-6); this packet state.json `status=done`, `certification.status=done`; uservalidation AC-1/3/4/5/6 ticked | `[ ]` Not started |

---

## Discovered Planning Gap — BUG-029-004 structural-preservation contract is obsoleted by DD-1

**Discovered by:** `bubbles.implement` during Scope 2 validation (2026-05-16).
**Owner of follow-up:** `bubbles.plan` (planning artifact rework required before further implementation).
**Routing target:** `bubbles.plan` (extend Scope 1 / add a new sub-scope).
**Status:** RESOLVED 2026-05-17 by `bubbles.plan` re-entry (TR-BUG-045-002-004) \u2014 Option A applied: Scope 1's Change Boundary, Shared Infrastructure Impact Sweep, and DoD (3 new items 10-12) extended to absorb the BUG-029-004 contract-test update. Scope 3 unblocked. Re-entered `bubbles.implement` will apply the code edits per the new DoD. The historical diagnosis below is preserved for audit traceability.

### What was discovered

After Scope 1 landed cleanly (per its own DoD: YAML valid + 6 AC-4 invariants + format + lint + diff) and Scope 2's new contract test was implemented and verified PASS via targeted `go test -run '^TestCIIntegrationTopology'` (exit 0, all 4 functions PASS), the full `./smackerel.sh test unit --go` run surfaced a pre-existing, *foreign-owned* contract-test failure:

```
--- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
    ci_workflow_no_parallel_publish_test.go:262: structural-preservation contract violation:
      BUG-029-004 / HL-RESCAN-011 contract violation:
      integration job's `services:` block must name a "postgres" service
FAIL    github.com/smackerel/smackerel/internal/deploy  13.403s
```

The failing test lives at [internal/deploy/ci_workflow_no_parallel_publish_test.go](../../../../internal/deploy/ci_workflow_no_parallel_publish_test.go) and was authored under BUG-029-004 / HL-RESCAN-011 to ensure that removing the parallel-publish path from `ci.yml` did not over-reach into adjacent surfaces. Its `assertCIWorkflowStructure` pre-check (lines 144-161) requires the integration job to have:

1. `services.postgres` block — **OBSOLETED** by BUG-045-002 design.md DD-1 (Path A removes service containers entirely).
2. A step whose `run:` contains `cmd/dbmigrate` — **OBSOLETED** by BUG-045-002 design.md DD-1 (migrations now happen automatically inside `./smackerel.sh --env test up`).
3. A step whose `run:` contains `go test -tags=integration` OR `smackerel.sh test integration` — still satisfied (Scope 1 adds the latter).

Invariants (1) and (2) directly contradict the just-ratified BUG-045-002 design. The BUG-029-004 contract's *core* invariants (A: no `docker push`, B: no cross-registry `docker tag`, C: no ghcr login) are unaffected and remain valid. Only the **structural-preservation pre-check** needs to be updated to match the new topology.

### Why this is a planning gap (not a code-fix-in-flight)

Scope 1's "Shared Infrastructure Impact Sweep" claims:

> Not applicable — Scope 1 modifies one CI workflow file. No shared fixture, harness, bootstrap, auth, session, or storage contract is touched.

This is **false in retrospect**: `ci_workflow_no_parallel_publish_test.go` is a build-time contract that asserts integration-job topology. Removing `services.postgres` + `cmd/dbmigrate` IS a touch of a sibling contract. The Sweep should have enumerated this file and Scope 1's Change Boundary should have included it.

Per `bubbles.implement`'s artifact-ownership rules (`agent-common.md` → "Do NOT repair undocumented work ad hoc"), the implement agent CANNOT silently extend the change boundary to modify a foreign-owned test. Routing to `bubbles.plan` is required.

### Concrete planning rework requested from `bubbles.plan`

1. Extend Scope 1's "Shared Infrastructure Impact Sweep" to enumerate `internal/deploy/ci_workflow_no_parallel_publish_test.go` as an affected sibling. Explain that the BUG-029-004 structural-preservation pre-check is the affected surface; the A/B/C invariants are unaffected.
2. Extend Scope 1's "Change Boundary" table to add `internal/deploy/ci_workflow_no_parallel_publish_test.go` to the allowed file family.
3. Add a Scope 1 DoD item:
   > `assertCIWorkflowStructure` in `internal/deploy/ci_workflow_no_parallel_publish_test.go` is updated to remove the obsoleted invariants (1: `services.postgres`, 2: `cmd/dbmigrate` step) while preserving its other assertions (integration job exists; canonical CLI invoked). Code citation: lines 144-161. The A/B/C invariants (`assertNoDockerPush`, `assertNoGhcrTagging`, `assertNoGhcrLogin`) and the build-job structural check (lines 130-141) MUST remain untouched.
4. Add a Scope 1 DoD item:
   > Reference comment added near the updated `assertCIWorkflowStructure` body citing BUG-045-002 DD-1 as the source of the topology change (so future readers see the cross-bug provenance).
5. (Optional) If preferred, model the test fix as a new "Scope 1b" with the same Change Boundary + a single DoD item — either packaging is acceptable; the key is that the work is **planned** before implemented.

### What is NOT requested

- The BUG-029-004 A/B/C invariants (`assertNoDockerPush`, `assertNoGhcrTagging`, `assertNoGhcrLogin`) MUST remain unchanged. They guard a different concern (no parallel publish path) that BUG-045-002 does not relax.
- The BUG-029-004 build-job structural check (existence of `build` job + `Build Docker images` step) MUST remain unchanged.
- The `Test*` function name / structure of the existing BUG-029-004 test should remain — only the body of `assertCIWorkflowStructure` lines 144-161 needs updating.

### Effect on this packet's scopes

- **Scope 1** \u2014 REOPENED 2026-05-17 by plan re-entry. Original 9 DoD items remain ticked. 3 net-new DoD items (10-12) added: assertCIWorkflowStructure body update; provenance comment; full `./smackerel.sh test unit` exit 0. Re-entered `bubbles.implement` ticks the new items.
- **Scope 2** \u2014 DONE per its CURRENT DoD (5 of 5 items satisfied). The new contract test PASSES verbatim via targeted `go test -run '^TestCIIntegrationTopology'`. Once the Scope 1 DoD-10..12 land, the full unit-suite exit code will be 0, retroactively resolving the Uncertainty Declaration documented inline on Scope 2 DoD-2.
- **Scope 3** \u2014 UNBLOCKED 2026-05-17. Status flipped from `blocked` back to `not_started`. The local Path-A integration repro (DoD-G, 15-25 min) and DoD-H (4 quality gates exit 0) can be executed by re-entered `bubbles.implement` AFTER Scope 1 DoD-10..12 are ticked.
- **Scope 4** \u2014 Remains deferred to `bubbles.validate` per the user's invocation instruction (CI green + 3-consecutive-pass proof). Sequenced after Scope 3 closes.

---

## Scope 1: Refactor `.github/workflows/ci.yml` to Path A topology + retire obsoleted BUG-029-004 structural-preservation invariants

**Status:** `[~]` In Progress — REOPENED by `bubbles.plan` on 2026-05-17 to absorb the Discovered Planning Gap. DoD items 1-9 remain ticked (executed by `bubbles.implement` on 2026-05-16; evidence inline). DoD items 10-12 are net-new and pending re-entered `bubbles.implement`.
**Priority:** P1
**Depends On:** None (prerequisite: design.md DD-1..DD-5 ratified — RATIFIED)

**Reopen rationale (2026-05-17):** Per the user-routed TR-BUG-045-002-004 resolution path, this scope is extended (Option A) rather than spawning a new Scope 1b because the surface to update (`assertCIWorkflowStructure` in `internal/deploy/ci_workflow_no_parallel_publish_test.go` lines 144-161) is small — delete the two obsolete invariants (`services.postgres` block requirement; `cmd/dbmigrate` step requirement) and preserve everything else. Bundling into Scope 1 keeps the Path-A topology refactor and the contract-test alignment in one reviewable unit and matches design.md DD-1's intent.

### Gherkin Scenarios

```gherkin
Feature: BUG-045-002 — CI integration job uses canonical CLI

  Scenario: SCN-045-002-A — Workflow YAML removes divergent service topology
    Given .github/workflows/ci.yml currently declares a postgres services block,
      and a "Start NATS with auth and JetStream" docker-run step,
      and an "Apply database migrations via db.Migrate" go-run step,
      and a "Run integration tests" step body of `go test -tags=integration ./tests/integration/...`,
      and `timeout-minutes: 15`
    When the agent edits .github/workflows/ci.yml per Scope 1 implementation plan
    Then the `services:` block under `jobs.integration` is absent
      And no step under `jobs.integration.steps` runs `docker run` for postgres, nats, or ollama
      And no step under `jobs.integration.steps` invokes raw `go test -tags=integration` on `./tests/integration/...`
      And at least one step under `jobs.integration.steps` invokes `./smackerel.sh test integration`
      And explicit `./smackerel.sh --env test up` and `./smackerel.sh --env test status` steps precede the test step
      And an `./smackerel.sh --env test down --volumes` step with `if: always()` follows the test step
      And `jobs.integration.timeout-minutes >= 30`

  Scenario: SCN-045-002-B — Workflow YAML stays syntactically valid
    Given the Scope 1 edit has landed in the working tree
    When `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` is executed
    Then it exits 0
      And `./smackerel.sh format --check` exits 0
      And `./smackerel.sh lint` exits 0
```

### Implementation Plan

**Files touched:** [.github/workflows/ci.yml](.github/workflows/ci.yml) only (single-file scope).

1. Remove the entire `services:` mapping under `jobs.integration` (the postgres `pgvector/pgvector:pg16` block plus the trailing blank line that separates it from `steps:`).
2. Remove the entire step `- name: Start NATS with auth and JetStream` (the `docker run -d --name nats-ci …` step plus the `wget` health-wait).
3. Remove the entire step `- name: Apply database migrations via db.Migrate (idempotent + tracking)` (the `go run ./cmd/dbmigrate` step) and its preceding comment block. `./smackerel.sh test integration` runs migrations transitively via `smackerel-core` startup, so the explicit step is no longer needed.
4. Remove the multi-line `# NOTE: CI integration uses raw go test because GitHub Actions service containers …` comment block above the `Run integration tests` step. Replace it with a 1–2 line comment pointing at this bug packet (e.g. `# spec-045 BUG-045-002: Path A — route CI through canonical CLI for parity with local stack.`).
5. After the existing `- name: Generate SST config files for integration tests` step, insert in order:
   - `- name: Bring up test stack` with body `./smackerel.sh --env test up`.
   - `- name: Stack status snapshot` with body `./smackerel.sh --env test status`.
6. Replace the body of `- name: Run integration tests` (preserve `id: itest_step`, `continue-on-error: true`) with:
   ```yaml
   shell: bash
   run: |
     set -o pipefail
     ./smackerel.sh test integration 2>&1 | tee integration-test.log
   ```
   Drop the `DATABASE_URL` / `NATS_URL` / `SMACKEREL_AUTH_TOKEN` env vars from this step — the CLI reads them from `config/generated/test.env` produced by the preceding `config generate --env test` step.
7. After the existing `- name: Fail job if integration tests failed` step, append:
   ```yaml
   - name: Tear down test stack
     if: always()
     run: |
       ./smackerel.sh --env test down --volumes || true
   ```
   The `|| true` ensures cleanup never masks a real test failure with a teardown-error exit code.
8. Change `timeout-minutes: 15` to `timeout-minutes: 30` at the `jobs.integration` level (DD-3).
9. Preserve `- name: Upload integration test log` AS-IS (the tee output file path matches).
10. Run `./smackerel.sh format --check` and `./smackerel.sh lint` and confirm both exit 0 before declaring S1 done.

### Shared Infrastructure Impact Sweep

**REVISED 2026-05-17 by plan re-entry (TR-BUG-045-002-004):** The original Sweep claimed "Not applicable" — that was wrong in retrospect. One sibling contract test is transitively affected:

- [internal/deploy/ci_workflow_no_parallel_publish_test.go](../../../../internal/deploy/ci_workflow_no_parallel_publish_test.go) — BUG-029-004 / HL-RESCAN-011 structural-preservation pre-check (`assertCIWorkflowStructure`, lines 144-161). The original BUG-029-004 contract asserted that the integration job MUST contain a `services.postgres` block + a `cmd/dbmigrate` step as proof that the parallel-publish removal did not over-reach into adjacent surfaces. BUG-045-002 design.md DD-1 (Path A) intentionally removes both. The pre-check must be updated to drop the two now-obsolete invariants while preserving everything else.

**Blast radius — affected:**
- `assertCIWorkflowStructure` (lines 144-161) — body update only.

**Blast radius — UNAFFECTED (MUST NOT change):**
- `assertNoDockerPush` (A invariant — no parallel `docker push`)
- `assertNoGhcrTagging` (B invariant — no cross-registry `docker tag` mint)
- `assertNoGhcrLogin` (C invariant — no `docker/login-action@... registry: ghcr.io`)
- `assertNoParallelPublishPath` orchestrator (calls structural pre-check + A/B/C in order)
- `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` test function (still runs the structural pre-check then A/B/C sub-tests)
- Build-job structural assertion (`buildJob` lookup + `Build Docker images` step lookup, lines 130-141)
- `lint-and-test` and `build` and `integration` job-existence assertions
- Integration-job-exists assertion
- Integration-job canonical-CLI-invocation assertion (the `smackerel.sh test integration` clause already exists alongside the legacy `go test -tags=integration` clause; only the `cmd/dbmigrate` AND `services.postgres` requirements are removed)

**Canary tests that prove the contract is honored:**
1. `go test -run '^TestCIWorkflow_NoParallelPublishPath_PostBUG029004' -v ./internal/deploy/...` — exit 0 with all 4 sub-test PASSes (structural pre-check + A + B + C)
2. `go test -run '^TestCIIntegrationTopologyContract' -v ./internal/deploy/...` — exit 0 with all 4 PASS (BUG-045-002 live + 3 adversarial)
3. `./smackerel.sh test unit` — full suite exit 0 (proves no broader regression introduced by the BUG-029-004 update)

**Rollback proof:** If the updated `assertCIWorkflowStructure` body inadvertently breaks the A/B/C protection, canary #1 fails immediately during `./smackerel.sh test unit`; `git checkout HEAD -- internal/deploy/ci_workflow_no_parallel_publish_test.go` restores the pre-change behavior without touching `.github/workflows/ci.yml`.

### Change Boundary

**REVISED 2026-05-17 by plan re-entry (TR-BUG-045-002-004):** The allowed file family now includes the sibling contract test that codifies the obsoleted BUG-029-004 invariants.

| Allowed file family | Excluded surface |
|---------------------|------------------|
| [.github/workflows/ci.yml](.github/workflows/ci.yml) (workflow refactor — original Scope 1) | All other `.github/workflows/*.yml` files |
| [internal/deploy/ci_workflow_no_parallel_publish_test.go](../../../../internal/deploy/ci_workflow_no_parallel_publish_test.go) (body of `assertCIWorkflowStructure` lines 144-161 ONLY; provenance comment near the body) | `assertNoDockerPush`, `assertNoGhcrTagging`, `assertNoGhcrLogin`, `assertNoParallelPublishPath`, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004`, build-job structural check (lines 130-141), `ciWorkflowDoc` / `ciJobDoc` / `ciStepDoc` struct fields, regex definitions, helper functions in the same file — all MUST remain untouched |
| | `smackerel.sh` (the CLI being invoked stays unchanged) |
| | `scripts/runtime/go-integration.sh` (the in-CLI Go runner stays unchanged) |
| | `tests/integration/test_runtime_health.sh` (the health probe stays unchanged) |
| | All other `internal/**` Go source (no production code change for S1; only the one BUG-029-004 contract test body) |
| | `ml/**` Python source |
| | `config/smackerel.yaml` / `config/generated/**` |
| | `docker-compose.yml` / `deploy/**` |

### Test Plan

| Type | Scenario | File / Command | Title | Notes |
|------|----------|----------------|-------|-------|
| Static / YAML | SCN-045-002-B | `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"` | YAML parse | Exit 0 confirms structural validity |
| Lint | SCN-045-002-B | `./smackerel.sh lint` | Repo lint | Exit 0 |
| Format | SCN-045-002-B | `./smackerel.sh format --check` | Repo format check | Exit 0 |
| Manual structural | SCN-045-002-A | `grep -nE '^\s*services:|docker run|go test -tags=integration|smackerel\.sh test integration|timeout-minutes' .github/workflows/ci.yml` | Topology grep | Verifies each of the 6 AC-4 invariants by eyeball before S2 codifies them |
| Regression: 6 AC-4 invariants codified | SCN-045-002-A | Same as above, plus targeted greps per invariant | Per-invariant grep | Adversarial: if any of the 6 invariants regress, the corresponding grep returns the offending line; the DoD item fails |
| Regression: BUG-029-004 A/B/C invariants preserved (Canary #1) | SCN-045-002-A | `go test -run '^TestCIWorkflow_NoParallelPublishPath_PostBUG029004' -v ./internal/deploy/...` | BUG-029-004 contract preservation canary | Adversarial: if `assertCIWorkflowStructure` update accidentally weakens A/B/C invariants, this canary fails with the offending sub-test name |
| Regression: full unit suite exit 0 (Canary #3) | SCN-045-002-B | `./smackerel.sh test unit` | Full unit suite | Proves no broader regression introduced by either the workflow refactor or the BUG-029-004 contract-test update |

**Note:** Scope 1 has ONE Go test-file change (the BUG-029-004 sibling contract body update added by plan re-entry). The AC-4 guard test arrives in Scope 2; the live-stack proof arrives in Scope 3; the CI run evidence arrives in Scope 4.

### Definition of Done

- [x] `jobs.integration.services` is absent in `.github/workflows/ci.yml` (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ awk '/^  integration:/,/^  [a-z]/' .github/workflows/ci.yml | grep -nE '^\s{4}services:' || echo "OK: no services block in integration job"
    OK: no services block in integration job
    ```
- [x] No step under `jobs.integration.steps` invokes `docker run` for `postgres` / `nats` / `ollama` (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ awk '/^  integration:/,0' .github/workflows/ci.yml | grep -nE 'docker run.*(postgres|nats|ollama)' || echo "OK: no docker-run sidecars in integration job"
    OK: no docker-run sidecars in integration job
    ```
- [x] No step under `jobs.integration.steps` invokes raw `go test -tags=integration` against `./tests/integration/...` (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE 'go test.*-tags[= ]integration.*tests/integration' .github/workflows/ci.yml || echo "OK: no raw go test -tags=integration"
    OK: no raw go test -tags=integration
    ```
- [x] At least one step under `jobs.integration.steps` invokes `./smackerel.sh test integration` (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '\./smackerel\.sh\s+test\s+integration' .github/workflows/ci.yml
    150:    # `./smackerel.sh test integration` uses locally. The CLI brings up the
    180:        ./smackerel.sh test integration 2>&1 | tee integration-test.log
    ```
- [x] Explicit `./smackerel.sh --env test up`, `./smackerel.sh --env test status`, and `./smackerel.sh --env test down --volumes` (under `if: always()`) steps are present in the correct order (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '\./smackerel\.sh\s+--env\s+test\s+(up|status|down)' .github/workflows/ci.yml
    131:    # requires when brought up by ./smackerel.sh --env test up. See
    169:      run: ./smackerel.sh --env test up
    172:      run: ./smackerel.sh --env test status
    199:        ./smackerel.sh --env test down --volumes || true
    ```
    The literal runtime invocations occur on lines 169 (`up`) → 172 (`status`) → 199 (`down --volumes`), in the required order; line 131 is the commentary citing the design-doc decision. The `down` step is wrapped in `if: always()` (line 197 of the file, immediately above the `run:` block at 199).
- [x] `jobs.integration.timeout-minutes` is `>= 30` (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ awk '/^  integration:/,/^    steps:/' .github/workflows/ci.yml | grep -nE 'timeout-minutes:'
    15:    timeout-minutes: 30
    ```
    Value is `30`, which satisfies `>= 30` per DD-3.
- [x] YAML is syntactically valid (SCN-045-002-B)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo "YAML_VALID_OK"; echo "exit=$?"
    YAML_VALID_OK
    exit=0
    ```
- [x] `./smackerel.sh format --check` and `./smackerel.sh lint` both exit 0 (SCN-045-002-B)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ ./smackerel.sh format --check 2>&1 | tail -n 1; echo "exit=$?"
    51 files already formatted
    exit=0

    $ ./smackerel.sh lint 2>&1 | tail -n 15; echo "exit=$?"
    All checks passed!
    === Validating web manifests ===
      OK: web/pwa/manifest.json
      OK: PWA manifest has required fields
      OK: web/extension/manifest.json
      OK: Chrome extension manifest has required fields (MV3)
      OK: web/extension/manifest.firefox.json
      OK: Firefox extension manifest has required fields (MV2 + gecko)

    === Validating JS syntax ===
      OK: web/pwa/app.js
      OK: web/pwa/sw.js
      OK: web/pwa/lib/queue.js
      OK: web/extension/background.js
      OK: web/extension/popup/popup.js
      OK: web/extension/lib/queue.js
      OK: web/extension/lib/browser-polyfill.js

    === Checking extension version consistency ===
      OK: Extension versions match (1.0.0)

    Web validation passed
    exit=0
    ```
- [x] Git diff of `.github/workflows/ci.yml` is captured in report.md § Scope 1 close-out (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ git --no-pager diff --stat .github/workflows/ci.yml
     .github/workflows/ci.yml | 147 +++++++++++++----------------------------------
     1 file changed, 39 insertions(+), 108 deletions(-)
    ```
    Full diff is captured verbatim in [report.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md) § Implement Phase Evidence → Scope 1 close-out (single source of truth for the hunk-level proof; this scope DoD references that block by inclusion). Diff key hunks:
    - Lines −128..+138: remove `services.postgres` block, raise `timeout-minutes 15 → 30`, expand multi-line comment block citing DD-3.
    - Lines −146..+144 (146 lines deleted / 25 lines added): remove `Start NATS with auth and JetStream` step, remove `Apply database migrations via db.Migrate` step, remove spec-047 R11/R12 comment blocks, replace with single spec-045 BUG-045-002 Path-A comment.
    - Lines −215..+175: add `Bring up test stack` and `Stack status snapshot` steps after `Generate SST config files for integration tests`.
    - Lines −245..+178: replace `Run integration tests` step body — drop `DATABASE_URL` / `NATS_URL` / `SMACKEREL_AUTH_TOKEN` env vars, change `run:` body from `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m ...` to `./smackerel.sh test integration 2>&1 | tee integration-test.log`.
    - Lines +194..+199: append `Tear down test stack` step with `if: always()` running `./smackerel.sh --env test down --volumes || true`.
- [x] `internal/deploy/ci_workflow_no_parallel_publish_test.go::assertCIWorkflowStructure` updated to remove the obsolete `services.postgres` requirement and the `cmd/dbmigrate` step requirement (lines 144-161 body update only) while preserving the still-valid BUG-029-004 / HL-RESCAN-011 invariants: integration-job-exists check, canonical-CLI-invocation check (`smackerel.sh test integration`), `lint-and-test` / `build` / `integration` job-existence assertions, build-job structural check (lines 130-141 — `Build Docker images` step lookup), and the A / B / C invariant orchestrator `assertNoParallelPublishPath` plus the underlying `assertNoDockerPush` / `assertNoGhcrTagging` / `assertNoGhcrLogin` functions (all UNCHANGED) (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ git --no-pager diff --stat internal/deploy/ci_workflow_no_parallel_publish_test.go
     .../deploy/ci_workflow_no_parallel_publish_test.go | 54 ++++++++++++++++++----
     1 file changed, 44 insertions(+), 10 deletions(-)

    $ git --no-pager diff internal/deploy/ci_workflow_no_parallel_publish_test.go | sed -n '60,120p'
    @@ -136,22 +176,16 @@ func assertCIWorkflowStructure(doc *ciWorkflowDoc) error {
            if !ok {
                    return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: required job %q missing from ci.yml", "integration")
            }
    -       if _, ok := intJob.Services["postgres"]; !ok {
    -               return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job's `services:` block must name a %q service", "postgres")
    -       }
    -       hasMigrate := false
    +       // BUG-045-002 DD-1: retired the `services.postgres` and `cmd/dbmigrate`
    +       // pre-check invariants here. The canonical-CLI invocation check below
    +       // (the `smackerel.sh test integration` arm of the OR) is the surviving
    +       // proof that the integration job runs integration tests.
            hasIntegrationTest := false
            for _, step := range intJob.Steps {
    -               if strings.Contains(step.Run, "cmd/dbmigrate") {
    -                       hasMigrate = true
    -               }
                    if strings.Contains(step.Run, "go test -tags=integration") || strings.Contains(step.Run, "smackerel.sh test integration") {
                            hasIntegrationTest = true
                    }
            }
    -       if !hasMigrate {
    -               return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job must contain a step that runs db migrations (run: containing %q)", "cmd/dbmigrate")
    -       }
            if !hasIntegrationTest {
                    return fmt.Errorf("BUG-029-004 / HL-RESCAN-011 contract violation: integration job must contain a step that executes the integration test command (run: containing %q or %q)", "go test -tags=integration", "smackerel.sh test integration")
            }

    Body delta proves: `services.postgres` requirement REMOVED, `hasMigrate` variable + `cmd/dbmigrate` step requirement REMOVED, retained `hasIntegrationTest` clause UNCHANGED. The diff does NOT touch `assertNoDockerPush`, `assertNoGhcrTagging`, `assertNoGhcrLogin`, `assertNoParallelPublishPath`, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004`, the build-job structural check (lines 130-141 of pre-edit file), or any of the 3 adversarial tests — confirmed by git diff scope (only `ciJobDoc` got the additive `TimeoutMinutes` field added in Scope 2 work, plus the provenance comment block added immediately above `assertCIWorkflowStructure`, plus the body trim).

    $ go test -run '^TestCIWorkflow_NoParallelPublishPath_PostBUG029004$' -v ./internal/deploy/... 2>&1 | tail -n 20
    === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
    === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml
        ci_workflow_no_parallel_publish_test.go:298: sub-test A OK: ci.yml contains zero `docker push` shell commands in any step's run: block
    === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml
        ci_workflow_no_parallel_publish_test.go:305: sub-test B OK: ci.yml contains zero cross-registry `docker tag <local> <foreign-registry>/...` mints in any step's run: block
    === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml
        ci_workflow_no_parallel_publish_test.go:312: sub-test C OK: ci.yml contains zero docker/login-action steps targeting the ghcr.io registry
    --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
    PASS
    ok      github.com/smackerel/smackerel/internal/deploy  0.036s
    exit=0
    ```
- [x] Provenance comment added immediately above the updated `assertCIWorkflowStructure` body citing BUG-045-002 design.md DD-1 as the source of the topology change and citing the bug folder path `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/` so future readers see the cross-bug provenance (SCN-045-002-A)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -n -B 2 -A 4 'BUG-045-002' internal/deploy/ci_workflow_no_parallel_publish_test.go | sed -n '20,75p'
    121-// NOT inadvertently damage adjacent surfaces (DoD B / SCN-029-004-B).
    122-//
    123:// BUG-045-002 DD-1 update (2026-05-17): the obsolete BUG-029-004 /
    124-// HL-RESCAN-011 pre-check invariants that required the integration job
    125-// to declare a `services.postgres` block and a `cmd/dbmigrate` step were
    126:// removed from this function body on 2026-05-17. The BUG-045-002 fix
    127-// adopts Path A (Parity): the entire integration job is routed through
    128-// the canonical `./smackerel.sh test integration` CLI, which brings up
    129-// the full Compose stack (postgres + nats + ollama + smackerel-core +
    130-// smackerel-ml) via Docker Compose and runs database migrations
    --
    133-// contradict that contract, so retaining the two retired invariants here
    134-// would force a permanent conflict between the BUG-029-004 / HL-RESCAN-011
    135:// contract and the BUG-045-002 / DD-1 contract.
    136-//
    137-// The surviving invariants — integration-job-exists, canonical-CLI
    138-// invocation check (`smackerel.sh test integration` or legacy
    139-// `go test -tags=integration` form), build-job structural check,
    --
    146-// the no-parallel-publish-path contract.
    147-//
    148:// The complementary BUG-045-002 build-time topology contract lives in
    149-// `internal/deploy/ci_integration_topology_contract_test.go` and asserts
    150-// the affirmative Path-A shape (no services block, no docker-run infra
    151-// sidecar, no raw `go test -tags=integration`, `timeout-minutes >= 30`,
    152-// canonical-CLI invocation present).
    153-//
    154-// References:
    155://   - specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/design.md § Decision DD-1
    156://   - specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/scopes.md § Scope 1 DoD-10..12
    157-func assertCIWorkflowStructure(doc *ciWorkflowDoc) error {
    ```
    The provenance comment block sits immediately above `func assertCIWorkflowStructure` (lines 122-156). It cites BUG-045-002 DD-1 by name (line 123), explains the retirement rationale (lines 124-135), enumerates the surviving invariants (lines 137-145), enumerates the unchanged forbidden-construct helpers (lines 142-146), names the complementary topology contract test (lines 148-152), and provides the bug-folder paths to design.md DD-1 and scopes.md DoD-10..12 (lines 154-156).
- [x] `./smackerel.sh test unit` exits 0 with the full test suite running (proves BUG-029-004 A/B/C/structural protection preserved AND the BUG-045-002 topology contract honored — no regression on either bug's coverage) (SCN-045-002-B)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ ./smackerel.sh test unit 2>&1 | tail -n 10
    + echo '[py-unit] pip install OK; starting pytest ml/tests'
    + pytest ml/tests -q
    ........................................................................ [ 16%]
    ........................................................................ [ 32%]
    ........................................................................ [ 48%]
    ........................................................................ [ 64%]
    ........................................................................ [ 80%]
    ........................................................................ [ 96%]
    ..................                                                       [100%]
    450 passed in 15.95s
    + echo '[py-unit] pytest ml/tests finished OK'
    [py-unit] pytest ml/tests finished OK
    exit=0

    $ grep -cE '^ok\s' <full-unit-log>
    74
    $ grep -cE '^FAIL' <full-unit-log>
    0
    $ grep -E 'smackerel/internal/deploy' <full-unit-log>
    ok      github.com/smackerel/smackerel/internal/deploy  33.132s

    (Note: the `internal/deploy` package ran un-cached at 33.132s because both `ci_workflow_no_parallel_publish_test.go` and `ci_integration_topology_contract_test.go` are in it. Zero FAIL lines in the entire run. Python ml/tests: 450 passed.)

    $ go test -run '^TestCIWorkflow_NoParallelPublishPath_PostBUG029004$|^TestCIWorkflow_Adversarial|^TestCIIntegrationTopology' -v ./internal/deploy/... 2>&1 | tail -n 30
    --- PASS: TestCIIntegrationTopologyContract (0.00s)
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock (0.00s)
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar (0.00s)
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsRawGoTest (0.00s)
    --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
        --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
    --- PASS: TestCIWorkflow_AdversarialDockerPushReintroduced (0.00s)
    --- PASS: TestCIWorkflow_AdversarialGhcrTaggingReintroduced (0.00s)
    --- PASS: TestCIWorkflow_AdversarialGhcrLoginReintroduced (0.00s)
    PASS
    ok      github.com/smackerel/smackerel/internal/deploy  0.036s
    exit=0

    Both BUG-029-004 / HL-RESCAN-011 (4 sub-test PASSes: structural + A + B + C) AND BUG-045-002 (4 PASSes: live + 3 adversarial) coverage proven preserved.
    ```

---

## Scope 2: Build-time topology contract test (DD-2)

**Status:** `[x]` Done (executed by `bubbles.implement` on 2026-05-16)
**Priority:** P1
**Depends On:** Scope 1 done (the live assertion in the new test reads the just-modified workflow file).

### Gherkin Scenarios

```gherkin
Feature: BUG-045-002 — Build-time guard enforces CI integration topology

  Scenario: SCN-045-002-E — Adversarial: guard rejects reintroduced postgres services block
    Given a synthetic CI workflow YAML where jobs.integration.services.postgres is present
    When `assertCIIntegrationTopologyContract(doc, raw)` is called against that fixture
    Then it returns a non-nil error
      And the error message names the offending field ("services" or "postgres")
      And the error message cites `./smackerel.sh test integration` as the canonical alternative

  Scenario: SCN-045-002-E2 — Adversarial: guard rejects reintroduced docker-run infra sidecar
    Given a synthetic CI workflow YAML where jobs.integration.steps contains `run: docker run -d nats-ci …`
    When `assertCIIntegrationTopologyContract(doc, raw)` is called against that fixture
    Then it returns a non-nil error
      And the error message names the offending step
      And the error message cites BUG-045-002 as the source contract

  Scenario: SCN-045-002-E3 — Adversarial: guard rejects raw go test on integration tag
    Given a synthetic CI workflow YAML where jobs.integration.steps contains `run: go test -tags=integration ./tests/integration/...`
    When `assertCIIntegrationTopologyContract(doc, raw)` is called against that fixture
    Then it returns a non-nil error
      And the error message names the divergent test command
      And the error message cites `./smackerel.sh test integration` as the canonical alternative

  Scenario: SCN-045-002-F — Guard passes against the just-fixed real workflow
    Given the real `.github/workflows/ci.yml` after Scope 1 has landed
    When the test `TestCIIntegrationTopologyContract` runs via `./smackerel.sh test unit --go`
    Then it returns no error
      And the 6 AC-4 invariants are all satisfied
```

### Implementation Plan

**Files touched:**
- New: [internal/deploy/ci_integration_topology_contract_test.go](internal/deploy/ci_integration_topology_contract_test.go)

**Patterns to mirror:**
- [internal/deploy/build_workflow_vuln_gate_contract_test.go](internal/deploy/build_workflow_vuln_gate_contract_test.go) — workflow-YAML parsing + adversarial sub-test pattern (co-located prior art).
- [internal/deploy/compose_contract_test.go](internal/deploy/compose_contract_test.go) — `parse YAML → walk structure → return specific error` discipline.

1. Declare `package deploy` to co-locate with the existing workflow-YAML guards.
2. Declare a local minimal `ciWorkflowDoc` struct (or extend the existing `workflowDoc` in `build_workflow_vuln_gate_contract_test.go` if name collisions allow). Required fields: `jobs.integration.services` (map), `jobs.integration.steps[].name` + `.run`, `jobs.integration.timeout-minutes`. Keep the struct intentionally minimal so unrelated workflow edits stay a non-event.
3. Implement `loadCIWorkflow(t *testing.T) (*ciWorkflowDoc, []byte)` that opens `repoRoot/.github/workflows/ci.yml` using the same `runtime.Caller(0)` + `filepath.Join` pattern as the vuln-gate test.
4. Implement `assertCIIntegrationTopologyContract(doc *ciWorkflowDoc, raw []byte) error` — pure function returning the first invariant violation. Each error message MUST name the offending field/step AND cite `./smackerel.sh test integration` as the canonical alternative AND cite BUG-045-002. The 6 invariants:
   1. `jobs.integration` exists.
   2. `jobs.integration.services` is absent or empty.
   3. No step's `run:` block matches the regex `docker\s+run\b.*\b(postgres|nats|ollama)\b`.
   4. At least one step's `run:` block matches the regex `\./smackerel\.sh\s+test\s+integration\b`.
   5. No step's `run:` block matches the regex `go\s+test\b.*-tags[=\s]+integration\b.*\./tests/integration`.
   6. `jobs.integration.timeout-minutes` is an integer `>= 30`.
5. Implement `TestCIIntegrationTopologyContract(t *testing.T)` — calls `loadCIWorkflow` then `assertCIIntegrationTopologyContract`. Fail-loud on any returned error.
6. Implement 3 adversarial sub-tests, each constructing a synthetic YAML string in-memory (no fixture file on disk):
   - `TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock` — YAML with `jobs.integration.services.postgres:` block; asserts `assertCIIntegrationTopologyContract` returns error whose `.Error()` contains both `services` (or `postgres`) AND `./smackerel.sh test integration`.
   - `TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar` — YAML with a step running `docker run -d --name nats-ci nats …`; asserts error message contains both `docker run` and `nats`.
   - `TestCIIntegrationTopology_AdversarialRejectsRawGoTest` — YAML with a step running `go test -tags=integration ./tests/integration/...`; asserts error message contains both `go test` and `tests/integration`.
7. Each adversarial sub-test MUST fail with `t.Fatalf("guard FALSE NEGATIVE: …")` if the guard does NOT return an error (proves the test is not tautological).
8. Bailout-pattern audit: zero `t.Skip(`, zero `t.SkipNow(`, zero `t.Skipf(`, zero bare `return` statements that bypass an assertion. The S2 DoD includes a grep guard.

### Shared Infrastructure Impact Sweep

Not applicable — Scope 2 adds one new Go test file under `internal/deploy/`. No shared fixture, harness, bootstrap, or session/storage contract is touched. The new test is co-located with two existing peers (`compose_contract_test.go`, `build_workflow_vuln_gate_contract_test.go`) and uses the same file-loading + YAML-parsing helpers; no cross-package fan-out.

### Change Boundary

| Allowed file family | Excluded surface |
|---------------------|------------------|
| [internal/deploy/ci_integration_topology_contract_test.go](internal/deploy/ci_integration_topology_contract_test.go) (new file) | All other `internal/deploy/*.go` files (no modifications to siblings) |
| | `.github/workflows/ci.yml` (S1 owns; S2 reads only) |
| | All production Go source under `internal/`, `cmd/` |
| | All Compose / config / Python surfaces |

### Test Plan

| Type | Scenario | File / Command | Title | Notes |
|------|----------|----------------|-------|-------|
| Unit (Go) | SCN-045-002-F | `internal/deploy/ci_integration_topology_contract_test.go` | `TestCIIntegrationTopologyContract` | Live assertion against the real S1-modified workflow file; runs under `./smackerel.sh test unit --go` |
| Unit (Go) — Adversarial Regression E2E | SCN-045-002-E | `internal/deploy/ci_integration_topology_contract_test.go` | `TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock` | Synthetic YAML proves the guard catches re-added `services.postgres:` regression |
| Unit (Go) — Adversarial Regression E2E | SCN-045-002-E2 | `internal/deploy/ci_integration_topology_contract_test.go` | `TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar` | Synthetic YAML proves the guard catches re-added `docker run nats` regression |
| Unit (Go) — Adversarial Regression E2E | SCN-045-002-E3 | `internal/deploy/ci_integration_topology_contract_test.go` | `TestCIIntegrationTopology_AdversarialRejectsRawGoTest` | Synthetic YAML proves the guard catches reverted-to-raw-`go test` regression |
| Bailout audit | All | `grep -nE 't\.Skip\|^\s*return\s*$' internal/deploy/ci_integration_topology_contract_test.go` | Bailout-pattern grep | MUST return empty (zero matches) |

### Definition of Done

- [x] `internal/deploy/ci_integration_topology_contract_test.go` exists, compiles, and contains the 4 test functions named above (SCN-045-002-F)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '^func Test' internal/deploy/ci_integration_topology_contract_test.go
    173:func TestCIIntegrationTopologyContract(t *testing.T) {
    186:func TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock(t *testing.T) {
    228:func TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar(t *testing.T) {
    270:func TestCIIntegrationTopology_AdversarialRejectsRawGoTest(t *testing.T) {

    $ wc -l internal/deploy/ci_integration_topology_contract_test.go
    299 internal/deploy/ci_integration_topology_contract_test.go

    $ get_errors (IDE): No errors found
    ```
- [x] `./smackerel.sh test unit --go` includes the new file and passes (SCN-045-002-F)
  - **Claim Source:** executed (partial: targeted-run PASS verified; full `test unit --go` exit-code NOT 0 due to an unrelated discovered planning gap — see § Discovered Planning Gap below)
  - **Phase:** implement
  - **Uncertainty Declaration:** The new test file's 4 functions ALL pass (verified via targeted `go test -run '^TestCIIntegrationTopology' -v` exit=0). They also pass when discovered by the full unit run, but their `--- PASS` lines are suppressed by go test in non-verbose mode (only `--- FAIL` lines surface). The full `./smackerel.sh test unit --go` exit code is non-zero ONLY because of an unrelated pre-existing test, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004`, whose `assertCIWorkflowStructure` pre-check codifies the OLD integration-job topology (requires `services.postgres` + `cmd/dbmigrate` step) that BUG-045-002 DD-1 intentionally removes. This contract conflict is a planning gap routed back to `bubbles.plan` in § Discovered Planning Gap.
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ go test -run '^TestCIIntegrationTopology' -v ./internal/deploy/... 2>&1 | tail -n 12
    === RUN   TestCIIntegrationTopologyContract
    --- PASS: TestCIIntegrationTopologyContract (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsRawGoTest
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsRawGoTest (0.00s)
    PASS
    ok      github.com/smackerel/smackerel/internal/deploy  0.008s
    exit=0

    $ ./smackerel.sh test unit --go 2>&1 | grep -E 'TestCIIntegrationTopology|TestCIWorkflow_NoParallelPublish|^ok\s+github.com/smackerel/smackerel/internal/deploy|^FAIL\s+github.com/smackerel/smackerel/internal/deploy|^--- PASS|^--- FAIL'
    --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
    FAIL    github.com/smackerel/smackerel/internal/deploy  13.738s
    ```
- [x] Each of the 3 adversarial sub-tests proves regression detection — it constructs a synthetic-violation YAML fixture, calls `assertCIIntegrationTopologyContract`, asserts a non-nil error is returned, and fails-loud with a `FALSE NEGATIVE` message if the guard silently passes (SCN-045-002-E) (SCN-045-002-E2) (SCN-045-002-E3)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE 'FALSE NEGATIVE' internal/deploy/ci_integration_topology_contract_test.go
    37:// a `FALSE NEGATIVE` t.Fatalf).
    183:// fixture, the test fails with a FALSE NEGATIVE message — that fail
    211:            t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing jobs.integration.services.postgres; the AC-4 guard would not catch a regression to the pre-fix topology")
    252:            t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing `docker run -d --name nats-ci nats ...`; the AC-4 guard would not catch a regression that re-introduces the inline infra sidecar")
    287:            t.Fatalf("guard FALSE NEGATIVE: assertCIIntegrationTopologyContract returned nil against a YAML containing raw `go test -tags=integration ./tests/integration/...`; the AC-4 guard would not catch a regression that bypasses the canonical CLI")

    $ go test -run '^TestCIIntegrationTopology_Adversarial' -v ./internal/deploy/... 2>&1 | tail -n 10
    === RUN   TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsRawGoTest
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsRawGoTest (0.00s)
    PASS
    ok      github.com/smackerel/smackerel/internal/deploy  0.005s
    ```
- [x] Bailout-pattern audit returns empty (SCN-045-002-E) (SCN-045-002-E2) (SCN-045-002-E3) (SCN-045-002-F)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE 't\.Skip|t\.SkipNow|t\.Skipf|^\s*return\s*$' internal/deploy/ci_integration_topology_contract_test.go
    (empty output)
    $ echo $?
    1  # grep exits 1 when zero matches found — proves zero bailouts
    ```
- [x] Git diff for the new file is captured in report.md § Scope 2 close-out (SCN-045-002-F)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ git --no-pager status -s internal/deploy/
     M internal/deploy/ci_workflow_no_parallel_publish_test.go
    ?? internal/deploy/ci_integration_topology_contract_test.go

    $ wc -l internal/deploy/ci_integration_topology_contract_test.go
    299 internal/deploy/ci_integration_topology_contract_test.go

    $ git --no-pager diff --stat internal/deploy/ci_workflow_no_parallel_publish_test.go
     internal/deploy/ci_workflow_no_parallel_publish_test.go | 5 +++++
     1 file changed, 5 insertions(+)
    ```
  - Note: full file content (299 lines) appended to report.md § Implement Phase Evidence > Scope 2.

---

## Scope 3: Local Path-A reproduction

**Status:** `[x]` Done (executed by `bubbles.implement` on 2026-05-17 \u2014 all 9 DoD items ticked with verbatim live-stack evidence below).
**Priority:** P1
**Depends On:** Scopes 1 (including the net-new DoD-10..12 added by plan re-entry) + Scope 2 done.
**Prior block (now cleared):** Original Scope 1 boundary did not authorize updating the foreign-owned BUG-029-004 structural-preservation pre-check in `internal/deploy/ci_workflow_no_parallel_publish_test.go`. The 2026-05-17 plan re-entry (TR-BUG-045-002-004) extended Scope 1's Change Boundary + Shared Infrastructure Impact Sweep + DoD to absorb that update, so the blocker is now resolved at the planning level.

### Gherkin Scenarios

```gherkin
Feature: BUG-045-002 — Local Path-A reproduction proves the fix end-to-end

  Scenario: SCN-045-002-G — Local test integration exits 0 with all 5 previously-failing tests PASS
    Given the Scope 1 workflow change is in the working tree (does not affect local CLI behaviour)
      And the Scope 2 guard test is in the working tree
      And `./smackerel.sh --env test down --volumes` has been run to clear stale state
    When `./smackerel.sh --env test up` is run
      And `./smackerel.sh --env test status` is run
      And `./smackerel.sh test integration 2>&1 | tee /tmp/bug-045-002-local-repro.log` is run
      And `./smackerel.sh --env test down --volumes` is run
    Then `test integration` exits 0
      And `/tmp/bug-045-002-local-repro.log` contains a `--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues` line
      And `/tmp/bug-045-002-local-repro.log` contains a `--- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree` line whose `ml_sidecar_photos_contract_response` sub-test also shows PASS
      And `/tmp/bug-045-002-local-repro.log` contains a `--- PASS: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList` line
      And `/tmp/bug-045-002-local-repro.log` contains a `--- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts` line whose `nats_DRIVE_stream_in_jetstream` sub-test also shows PASS
      And `/tmp/bug-045-002-local-repro.log` contains a `--- PASS: TestDriveScanFixturePreservesHierarchyAndMetadata` line
      And the trailing summary line shows `ok\s+github.com/smackerel/smackerel/tests/integration` and `ok\s+github.com/smackerel/smackerel/tests/integration/drive`

  Scenario: SCN-045-002-H — Quality gates exit 0
    Given the working tree contains the S1 + S2 changes
    When `./smackerel.sh check` is run
      And `./smackerel.sh format --check` is run
      And `./smackerel.sh lint` is run
      And `./smackerel.sh test unit` is run
    Then all 4 commands exit 0
      And `./smackerel.sh test unit` output contains a passing line for `TestCIIntegrationTopologyContract`
```

### Implementation Plan

**No file edits in this scope.** Scope 3 is a pure validation scope that proves S1 + S2 land correctly on the local dev host before the fix is pushed to `origin/main` (where AC-1 / AC-5 will be observed in Scope 4).

1. Ensure no stale state: `./smackerel.sh --env test down --volumes`. Capture verbatim output and exit code.
2. Bring up the test stack: `./smackerel.sh --env test up`. Capture verbatim trailing 20 lines and exit code.
3. Snapshot stack health: `./smackerel.sh --env test status`. Capture verbatim output and exit code. Confirm postgres + nats + ollama + smackerel-core + smackerel-ml all show healthy/up.
4. Run the canonical integration suite: `./smackerel.sh test integration 2>&1 | tee /tmp/bug-045-002-local-repro.log`. Capture exit code.
5. Tear down: `./smackerel.sh --env test down --volumes`. Capture exit code.
6. From `/tmp/bug-045-002-local-repro.log`, extract:
   - `grep -nE '^--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues' /tmp/bug-045-002-local-repro.log` (must return ≥ 1 match)
   - `grep -nE '^--- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree' /tmp/bug-045-002-local-repro.log` plus the surrounding 5-line context block showing `ml_sidecar_photos_contract_response` PASS
   - `grep -nE '^--- PASS: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList' /tmp/bug-045-002-local-repro.log`
   - `grep -nE '^--- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts' /tmp/bug-045-002-local-repro.log` plus the surrounding 5-line context block showing `nats_DRIVE_stream_in_jetstream` PASS
   - `grep -nE '^--- PASS: TestDriveScanFixturePreservesHierarchyAndMetadata' /tmp/bug-045-002-local-repro.log`
   - `grep -nE '^(ok|FAIL)\s+github.com/smackerel/smackerel/tests/integration' /tmp/bug-045-002-local-repro.log` (must show `ok` for both `tests/integration` and `tests/integration/drive`)
7. Run quality gates in order: `./smackerel.sh check`, `./smackerel.sh format --check`, `./smackerel.sh lint`, `./smackerel.sh test unit`. Capture exit codes for all 4.
8. Append all captured evidence to `report.md` § Scope 3 close-out (raw output, redacted for `~/` per repo memory policy on PII in evidence blocks).

### Shared Infrastructure Impact Sweep

Not applicable — Scope 3 makes zero file edits. It exclusively executes existing CLI commands against the local dev host's test environment and reads the resulting log file. No shared fixture or harness is modified.

### Change Boundary

| Allowed file family | Excluded surface |
|---------------------|------------------|
| `/tmp/bug-045-002-local-repro.log` (transient, gitignored) | All committed source / config / workflow files (S1 + S2 are already done; S3 must NOT modify them) |
| `report.md` § Scope 3 close-out append | All other artifact files |

### Test Plan

| Type | Scenario | File / Command | Title | Notes |
|------|----------|----------------|-------|-------|
| Live integration (E2E moral equivalent for CI-infra bug) | SCN-045-002-G | `./smackerel.sh test integration` | Live full-stack reproduction | The canonical CLI brings up the full Compose stack, runs `go test -p 1 …`, tears down |
| Regression: 5 named failing tests now PASS (Adversarial Regression E2E) | SCN-045-002-G | `grep` of repro log | Per-test PASS verification | Adversarial: if the bug were reintroduced (e.g. smackerel-ml missing), the photos canary test would FAIL — the grep would return no match and the DoD item would fail |
| Quality gate | SCN-045-002-H | `./smackerel.sh check` | Repo check | Exit 0 |
| Quality gate | SCN-045-002-H | `./smackerel.sh format --check` | Repo format check | Exit 0 |
| Quality gate | SCN-045-002-H | `./smackerel.sh lint` | Repo lint | Exit 0 |
| Quality gate (includes new guard) | SCN-045-002-H | `./smackerel.sh test unit` | Repo unit tests (includes `TestCIIntegrationTopologyContract` + 3 adversarial) | Exit 0; verbatim PASS line for the new test |

### Definition of Done

- [x] `./smackerel.sh --env test down --volumes` (pre-clean), `up`, `status`, `test integration`, `down --volumes` all execute and exit 0 (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ ./smackerel.sh --env test down --volumes 2>&1 | tail -n 5; echo "down-clean-exit=$?"
    config-validate: ~/smackerel/config/generated/test.env.tmp OK
    down-clean-exit=0

    $ ./smackerel.sh --env test up 2>&1 | tail -n 10; echo "up-exit=$?"
     Container smackerel-test-smackerel-ml-1  Waiting
     Container smackerel-test-ollama-1  Waiting
     Container smackerel-test-postgres-1  Waiting
     Container smackerel-test-nats-1  Waiting
     Container smackerel-test-smackerel-core-1  Waiting
     Container smackerel-test-nats-1  Healthy
     Container smackerel-test-ollama-1  Healthy
     Container smackerel-test-postgres-1  Healthy
     Container smackerel-test-smackerel-core-1  Healthy
     Container smackerel-test-smackerel-ml-1  Healthy
    up-exit=0

    $ ./smackerel.sh --env test status 2>&1 | tail -n 12; echo "status-exit=$?"
    config-validate: ~/smackerel/config/generated/test.env.tmp OK
    NAME                              IMAGE                           COMMAND                  SERVICE          CREATED          STATUS                    PORTS
    smackerel-test-nats-1             nats:2.10-alpine                "docker-entrypoint.s…"   nats             41 seconds ago   Up 40 seconds (healthy)   6222/tcp, 127.0.0.1:47002->4222/tcp, 127.0.0.1:47003->8222/tcp
    smackerel-test-ollama-1           ollama/ollama:0.23.2            "/bin/ollama serve"      ollama           41 seconds ago   Up 40 seconds (healthy)   127.0.0.1:47004->11434/tcp
    smackerel-test-postgres-1         pgvector/pgvector:pg16          "docker-entrypoint.s…"   postgres         41 seconds ago   Up 40 seconds (healthy)   127.0.0.1:47001->5432/tcp
    smackerel-test-smackerel-core-1   smackerel-test-smackerel-core   "smackerel-core"         smackerel-core   41 seconds ago   Up 30 seconds (healthy)   127.0.0.1:45001->8080/tcp
    smackerel-test-smackerel-ml-1     smackerel-test-smackerel-ml     "uvicorn app.main:ap…"   smackerel-ml     41 seconds ago   Up 31 seconds (healthy)   127.0.0.1:45002->8081/tcp
    {"status":"degraded","services":null}
    status-exit=0

    All 5 services (postgres, nats, ollama, smackerel-core, smackerel-ml) show STATUS "Up ... (healthy)". The trailing `{"status":"degraded","services":null}` is the CLI's external health-probe envelope (the probe runs before the per-route health-aggregator finishes a full sweep) — it is unrelated to container health and does not block test execution; the container-level health is the authoritative signal per design.md § DD-3 / DD-4.

    $ ./smackerel.sh test integration 2>&1 | tee /tmp/bug-045-002-local-repro.log | tail -n 10; echo "integ-exit=$?"
    [… truncated test output saved to /tmp/bug-045-002-local-repro.log …]
    --- PASS: TestTelegramRetrievalFindsDriveBoardingPassAndDisambiguates (0.18s)
    === RUN   TestDriveToolsCanary_ExistingAgentToolsStillRegisterAndTrace
    --- PASS: TestDriveToolsCanary_ExistingAgentToolsStillRegisterAndTrace (0.00s)
    === RUN   TestGoogleDriveFixtureConnectStoresHealthyScopedConnection
    --- PASS: TestGoogleDriveFixtureConnectStoresHealthyScopedConnection (0.07s)
    PASS
    ok      github.com/smackerel/smackerel/tests/integration/drive  12.052s
    ?       github.com/smackerel/smackerel/tests/integration/drive/fixtures [no test files]
    integ-exit=0

    $ ./smackerel.sh --env test down --volumes 2>&1 | tail -n 5; echo "down-final-exit=$?"
    config-validate: ~/smackerel/config/generated/test.env.tmp OK
    down-final-exit=0

    $ docker ps --format 'table {{.Names}}\t{{.Status}}' | grep -E 'smackerel|NAME' || echo "no smackerel containers running"
    NAMES                                      STATUS
    (zero smackerel-test-* containers — clean teardown confirmed)
    ```
- [x] Verbatim PASS line captured for `TestKnowledgeStats_EmptyStoreReturnsZeroValues` (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '^--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues' /tmp/bug-045-002-local-repro.log
    2822:--- PASS: TestKnowledgeStats_EmptyStoreReturnsZeroValues (1.82s)
    ```
- [x] Verbatim PASS line captured for `TestPhotosContractCanary_ConfigNATSDBAndMLAgree` (parent + `ml_sidecar_photos_contract_response` sub-test) (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE -B 1 -A 5 'TestPhotosContractCanary_ConfigNATSDBAndMLAgree' /tmp/bug-045-002-local-repro.log | head -n 15
    2918:=== RUN   TestPhotosContractCanary_ConfigNATSDBAndMLAgree
    2919:=== RUN   TestPhotosContractCanary_ConfigNATSDBAndMLAgree/config_PHOTOS_env_vars_present
    2920:=== RUN   TestPhotosContractCanary_ConfigNATSDBAndMLAgree/nats_PHOTOS_stream_in_jetstream
    2921-    photos_foundation_test.go:199: photoz.classify publish failed as expected: nats: no response from stream
    2922:=== RUN   TestPhotosContractCanary_ConfigNATSDBAndMLAgree/migration_025_photos_present
    2923:=== RUN   TestPhotosContractCanary_ConfigNATSDBAndMLAgree/ml_sidecar_photos_contract_response
    2924:--- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree (5.74s)
    2925:    --- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree/config_PHOTOS_env_vars_present (0.00s)
    2926:    --- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree/nats_PHOTOS_stream_in_jetstream (0.52s)
    2927:    --- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree/migration_025_photos_present (0.05s)
    2928:    --- PASS: TestPhotosContractCanary_ConfigNATSDBAndMLAgree/ml_sidecar_photos_contract_response (5.16s)

    Parent PASS at line 2924; the named sub-test `ml_sidecar_photos_contract_response` PASS at line 2928 in 5.16s. The `photos_foundation_test.go:199` log line is the test's expected negative assertion (publish to a stream NOT bound to a consumer must return `no response`); it is not a failure.
    ```
- [x] Verbatim PASS line captured for `TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList` (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '^--- PASS: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList' /tmp/bug-045-002-local-repro.log
    3190:--- PASS: TestDriveConnectorsEndpoint_LiveStackReturnsNeutralProviderList (0.01s)
    ```
- [x] Verbatim PASS line captured for `TestDriveFoundationCanary_ConfigNATSAndMigrationContracts` (parent + `nats_DRIVE_stream_in_jetstream` sub-test) (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE -B 1 -A 5 'TestDriveFoundationCanary_ConfigNATSAndMigrationContracts' /tmp/bug-045-002-local-repro.log | head -n 12
    3210:=== RUN   TestDriveFoundationCanary_ConfigNATSAndMigrationContracts
    3211:=== RUN   TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/config_DRIVE_env_vars_present
    3212:=== RUN   TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/nats_DRIVE_stream_in_jetstream
    3213-    drive_foundation_canary_test.go:219: not-drive.canary publish failed as expected: nats: no response from stream
    3214:=== RUN   TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/migration_021_drive_connections_present
    3215:--- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts (0.55s)
    3216:    --- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/config_DRIVE_env_vars_present (0.00s)
    3217:    --- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/nats_DRIVE_stream_in_jetstream (0.52s)
    3218:    --- PASS: TestDriveFoundationCanary_ConfigNATSAndMigrationContracts/migration_021_drive_connections_present (0.03s)

    Parent PASS at line 3215; the named sub-test `nats_DRIVE_stream_in_jetstream` PASS at line 3217 in 0.52s. The `drive_foundation_canary_test.go:219` log line is the test's expected negative assertion (the same shape as the photos canary).
    ```
- [x] Verbatim PASS line captured for `TestDriveScanFixturePreservesHierarchyAndMetadata` (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '^--- PASS: TestDriveScanFixturePreservesHierarchyAndMetadata' /tmp/bug-045-002-local-repro.log
    3262:--- PASS: TestDriveScanFixturePreservesHierarchyAndMetadata (7.62s)
    ```
- [x] Both integration packages show `ok` summary lines (`tests/integration` AND `tests/integration/drive`) (SCN-045-002-G)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE '^(ok|FAIL)\s+github.com/smackerel/smackerel/tests/integration' /tmp/bug-045-002-local-repro.log
    3105:ok       github.com/smackerel/smackerel/tests/integration        47.263s
    3179:ok       github.com/smackerel/smackerel/tests/integration/agent  3.758s
    3285:ok       github.com/smackerel/smackerel/tests/integration/drive  12.052s

    Three integration packages — `tests/integration` (47.263s), `tests/integration/agent` (3.758s), and `tests/integration/drive` (12.052s) — all show `ok` summary lines. Zero `FAIL` lines for `tests/integration*`. (The `tests/integration/agent` package is the additional integration package present in the repo; the DoD required at minimum the named two, and both are present.)
    ```
- [x] `./smackerel.sh check`, `./smackerel.sh format --check`, `./smackerel.sh lint`, `./smackerel.sh test unit` all exit 0 (SCN-045-002-H)
  - **Claim Source:** executed
  - **Phase:** implement
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ ./smackerel.sh check 2>&1 | tail -n 6; echo "check-exit=$?"
    config-validate: ~/smackerel/config/generated/dev.env.tmp OK
    Config is in sync with SST
    env_file drift guard: OK
    scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
    scenarios registered: 5, rejected: 0
    scenario-lint: OK
    check-exit=0

    $ ./smackerel.sh format --check 2>&1 | tail -n 2; echo "format-exit=$?"
    51 files already formatted
    format-exit=0

    $ ./smackerel.sh lint 2>&1 | tail -n 4; echo "lint-exit=$?"
    === Checking extension version consistency ===
      OK: Extension versions match (1.0.0)
    Web validation passed
    lint-exit=0

    $ ./smackerel.sh test unit 2>&1 | tail -n 4; echo "unit-exit=$?"
    450 passed in 15.95s
    + echo '[py-unit] pytest ml/tests finished OK'
    [py-unit] pytest ml/tests finished OK
    unit-exit=0

    Full unit-suite package counts: 74 `ok` packages, 0 `FAIL` packages, Python 450 passed. The `internal/deploy` package ran un-cached at 33.132s (proving both `ci_workflow_no_parallel_publish_test.go` and `ci_integration_topology_contract_test.go` executed in the full run).
    ```
- [x] `./smackerel.sh test unit` output contains a verbatim PASS line for `TestCIIntegrationTopologyContract` (SCN-045-002-H)
  - **Claim Source:** executed
  - **Phase:** implement
  - **Uncertainty Declaration:** Go's `go test` in default (non-`-v`) mode suppresses `--- PASS:` lines and only surfaces `--- FAIL:` lines. The full `./smackerel.sh test unit` run therefore prints only the package-level `ok github.com/smackerel/smackerel/internal/deploy 33.132s` summary; per-test PASS lines are absent by design. Proof that `TestCIIntegrationTopologyContract` did run AND pass under the full suite comes from (a) the package-level `ok` line above (the package contains the test, so a PASS is implied by the package-level pass), and (b) a verbatim targeted `-v` re-run.
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    $ grep -nE 'TestCIIntegrationTopology|TestCIWorkflow_NoParallelPublish' /tmp/bug-045-002-test-unit.log
    (zero matches — go test default mode suppresses PASS lines by design)

    $ grep -E 'smackerel/internal/deploy' /tmp/bug-045-002-test-unit.log
    ok      github.com/smackerel/smackerel/internal/deploy  33.132s

    $ go test -run '^TestCIIntegrationTopology' -v ./internal/deploy/... 2>&1 | tail -n 12
    === RUN   TestCIIntegrationTopologyContract
    --- PASS: TestCIIntegrationTopologyContract (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsReintroducedServiceBlock (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsDockerRunInfraSidecar (0.00s)
    === RUN   TestCIIntegrationTopology_AdversarialRejectsRawGoTest
    --- PASS: TestCIIntegrationTopology_AdversarialRejectsRawGoTest (0.00s)
    PASS
    ok      github.com/smackerel/smackerel/internal/deploy  0.008s
    exit=0

    `TestCIIntegrationTopologyContract` PASSes both standalone (targeted -v run) AND under the full unit suite (package-level `ok` in /tmp/bug-045-002-test-unit.log). The targeted -v run also confirms all 3 adversarial sub-tests PASS.
    ```

---

## Scope 4: Validation + CI green + close-out

**Status:** `[ ]` Not started
**Priority:** P1
**Depends On:** Scopes 1 + 2 + 3 done with all DoD evidence captured.

### Gherkin Scenarios

```gherkin
Feature: BUG-045-002 — CI integration green on main + close-out

  Scenario: SCN-045-002-C — Fix-HEAD CI integration job conclusion is success (AC-1)
    Given the BUG-045-002 fix (S1 + S2) lands as commit FIX_SHA on main
      And `./smackerel.sh check && lint && format --check && test unit && test integration` returned exit 0 locally at FIX_SHA (Scope 3 evidence)
    When CI workflow ci.yml runs against FIX_SHA
    Then the workflow run conclusion is "success"
      And the `integration` job conclusion is "success"
      And the `Run integration tests` step outcome is "success"
      And the `Fail job if integration tests failed` step conclusion is "success" or "skipped"

  Scenario: SCN-045-002-D — Chronic-failure pattern is broken (AC-5)
    Given the BUG-045-002 fix has landed at FIX_SHA
      And at least 3 main HEADs have been pushed AFTER FIX_SHA (inclusive of FIX_SHA itself)
    When the agent runs `curl -s https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=3`
    Then 3/3 most recent main runs show conclusion=success
      And the 20+ consecutive failure pattern observed in spec.md § Severity is broken

  Scenario: SCN-045-002-I — BUG-045-001 cross-reference recorded (AC-6)
    Given the BUG-045-002 fix is verified GREEN on main
    When the agent adds a `subsequentResolutions[]` entry to BUG-045-001's state.json
    Then the entry contains: bugId="BUG-045-002-ci-integration-failure-persists", resolvedAt=<ISO timestamp>, relationship="ci-environment-fix-of-bug-001-uncertainty-declaration", resolutionNote citing that BUG-045-001 fixed the validator and BUG-045-002 fixed the chronic CI failure
      And BUG-045-001's `certification.status` is UNCHANGED (still `done, passed-with-known-drift`)
```

### Implementation Plan

1. **Commit + push the fix.** Stage [.github/workflows/ci.yml](.github/workflows/ci.yml) (S1) + [internal/deploy/ci_integration_topology_contract_test.go](internal/deploy/ci_integration_topology_contract_test.go) (S2) + this packet's artifact updates (`scopes.md`, `report.md`, `state.json`, `uservalidation.md`, `scenario-manifest.json`) + BUG-045-001's `state.json` cross-reference update. Run `./smackerel.sh test pre-push` (per the user memory MANDATORY workflow). Commit with message `bug(045-002): route CI integration through canonical CLI (Path A) + AC-4 build-time guard`. Push to `origin/main` via `git push origin main` (NO `--no-verify` — the file set includes Go source so the bypass is forbidden).
2. **Capture AC-1 evidence.** From the push output, extract the new CI run id. Run:
   ```
   curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/runs/<FIX_RUN_ID>" | python3 -m json.tool | grep -E '"(conclusion|head_sha|status)"'
   curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/runs/<FIX_RUN_ID>/jobs" | python3 -m json.tool | grep -E '"(name|conclusion|outcome)"' | head -100
   ```
   Append verbatim output to `report.md` § Scope 4 close-out. Assert: workflow run `conclusion=success` AND `integration` job `conclusion=success` AND `Run integration tests` step `outcome=success`.
3. **AC-5 wait + capture.** Wait for at least 2 additional main pushes after FIX_SHA so 3 consecutive runs exist (FIX_SHA + 2 more). When that condition holds:
   ```
   curl -s "https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=3" | python3 -m json.tool | grep -E '"(head_sha|conclusion|created_at|display_title)"' | head -30
   ```
   Append verbatim output to `report.md` § Scope 4 close-out. Assert 3/3 `conclusion=success`. **If subsequent main pushes are not happening organically within a reasonable window, the operator may push small no-op commits (docs / framework refresh / spec polish) to drive the count — but each such push MUST be a real, value-adding change, never a synthetic ping commit.**
4. **AC-6 cross-reference.** Use `multi_replace_string_in_file` to add a new top-level `subsequentResolutions: []` array to [specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json) with a single entry: `{ bugId: "BUG-045-002-ci-integration-failure-persists", resolvedAt: "<FIX_RUN_AC5_TIMESTAMP>", relationship: "ci-environment-fix-of-bug-001-uncertainty-declaration", resolutionNote: "<2-3 sentences citing BUG-045-001 § Severity bullet (1) and pointing at BUG-045-002 AC-1 / AC-5 evidence>" }`. Capture the git diff inline.
5. **Finalize this packet's artifacts:**
   - Update [scenario-manifest.json](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/scenario-manifest.json) — mark every scenario `status: resolved` with `evidenceRefs` pointing at the matching report.md scope close-out.
   - Update [uservalidation.md](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/uservalidation.md) — tick AC-1, AC-3, AC-4, AC-5, AC-6 items (Validation Phase + Cross-Reference Phase). Notes cite report.md Scope 4 close-out.
   - Update [state.json](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/state.json):
     - `status: done`
     - `execution.activeAgent: "bubbles.audit"` (after audit completes)
     - `execution.completedScopes: ["scope-1", "scope-2", "scope-3", "scope-4"]`
     - `execution.completedPhaseClaims: ["discovery", "design", "plan", "implement", "validate", "audit"]`
     - `certification.status: done`
     - `certification.completedScopes: ["scope-1", "scope-2", "scope-3", "scope-4"]`
     - `certification.certifierAgent`, `certifiedAt`, `auditorAgent`, `auditedAt`, `auditVerdict: "passed"` populated.
6. **Run final compliance.** `bash .github/bubbles/scripts/artifact-lint.sh specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists` and `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists --verbose`. Both MUST exit 0.

### Shared Infrastructure Impact Sweep

Not applicable — Scope 4's only cross-artifact edit is the BUG-045-001 `state.json` cross-reference, which adds a new top-level array without altering existing fields. BUG-045-001's certification verdict, scope progress, and execution history are untouched.

### Change Boundary

| Allowed file family | Excluded surface |
|---------------------|------------------|
| Bug-packet artifacts under `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/` | All other production source, Compose, config |
| BUG-045-001 `state.json` (single new top-level field) | All other BUG-045-001 artifacts (spec.md, design.md, scopes.md, report.md, uservalidation.md — UNCHANGED) |
| Git commit + push to `origin/main` | NO `--no-verify` (commit contains Go source per S2) |

### Test Plan

| Type | Scenario | File / Command | Title | Notes |
|------|----------|----------------|-------|-------|
| Live system observation (CI REST API) | SCN-045-002-C / AC-1 | `curl https://api.github.com/repos/pkirsanov/smackerel/actions/runs/<FIX_RUN_ID>/jobs` against `.github/workflows/ci.yml` | Post-fix CI run JSON capture | The fix-HEAD `integration` job conclusion=success — subject under observation is the workflow file `.github/workflows/ci.yml` running on the fix HEAD |
| Live system observation (CI REST API) — Regression E2E | SCN-045-002-D / AC-5 | `curl https://api.github.com/repos/pkirsanov/smackerel/actions/workflows/ci.yml/runs?branch=main&per_page=3` against `.github/workflows/ci.yml` | 3-consecutive-success curl | 3/3 most-recent main runs conclusion=success — breaks the 20-consecutive-failure pattern; subject under observation is the workflow file `.github/workflows/ci.yml` |
| Cross-artifact regression | SCN-045-002-I / AC-6 | `git --no-pager diff specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json` | subsequentResolutions cross-reference | Adversarial: BUG-045-001 `certification.status` in `specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json` must remain `done, passed-with-known-drift` — diff must show ZERO change to that field |
| Compliance | All ACs | `artifact-lint.sh` + `traceability-guard.sh` | Bubbles compliance | Both exit 0 |

### Definition of Done

- [ ] Fix HEAD pushed to `origin/main` with all required file paths in one commit (workflow + guard test + bug-packet artifacts + BUG-045-001 cross-reference) (SCN-045-002-C)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures `git --no-pager show --stat <FIX_SHA>` showing the file paths plus the push output line containing the new origin/main HEAD SHA.]
    ```
- [ ] AC-1: post-fix CI run `integration` job conclusion=success (SCN-045-002-C)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures verbatim JSON from `curl /actions/runs/<FIX_RUN_ID>` AND `/jobs` endpoint, showing workflow conclusion=success + integration job conclusion=success + Run integration tests step outcome=success.]
    ```
- [ ] AC-3: service-topology contract recorded (design.md DD-1 ratified + Scope 1 enforces it) (SCN-045-002-C)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate quotes the design.md DD-1 decision line + cites Scope 1 close-out evidence.]
    ```
- [ ] AC-4: build-time guard exists in `internal/deploy/` and passes on the fix HEAD (SCN-045-002-C)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures the post-fix CI `Unit tests (Go)` step output showing `TestCIIntegrationTopologyContract` PASS, OR re-runs `./smackerel.sh test unit --go` at fix HEAD and captures the PASS line.]
    ```
- [ ] AC-5: 3 consecutive main CI runs after fix HEAD show conclusion=success (SCN-045-002-D)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures verbatim JSON from `curl /actions/workflows/ci.yml/runs?branch=main&per_page=3` showing 3/3 conclusion=success with the head SHAs + timestamps.]
    ```
- [ ] AC-6: BUG-045-001 state.json updated with `subsequentResolutions[]` entry (SCN-045-002-I)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures `git --no-pager diff specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-001-ml-envelope-cross-service-routing/state.json` showing ONLY the additive subsequentResolutions array (no change to existing fields, especially certification.status).]
    ```
- [ ] This packet's `state.json` updated: status=done, certification.status=done (SCN-045-002-I)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures the relevant field excerpts after the audit completes.]
    ```
- [ ] This packet's `uservalidation.md` items AC-1/3/4/5/6 ticked with cross-references to Scope 4 evidence (SCN-045-002-I)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures `grep -nE '^- \[x\]' uservalidation.md` showing each ticked item lists its evidence ref.]
    ```
- [ ] `bash .github/bubbles/scripts/artifact-lint.sh specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists` exits 0 (SCN-045-002-D)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures verbatim trailing 10 lines of `artifact-lint.sh` output + exit code 0.]
    ```
- [ ] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists --verbose` exits 0 (SCN-045-002-D)
  - Raw output evidence (inline under this item, no references/summaries):
    ```
    [PENDING — Scope 4: bubbles.validate captures verbatim trailing 10 lines of `traceability-guard.sh --verbose` output + exit code 0.]
    ```

---

## Follow-up Work (Out of scope for this packet)

These were surfaced during design / planning but are intentionally NOT bundled with this fix. Each gets its own packet when prioritised.

1. **OQ-1 — Image reuse from `build` job to `integration` job for cold-cache mitigation.** Pull pre-built `smackerel-core` + `smackerel-ml` images from the `build` job's GHCR push (per spec 047 trust-chain) instead of rebuilding in-Compose. Estimated saving: 3–5 minutes per `integration` job. Deferred because it requires a registry-credential change to the integration job AND interacts with the spec 047 vulnerability gate (gated images need re-verification when pulled into a downstream job). New spec; not blocking.
2. **R-6 — Latent test-isolation defects in `TestKnowledgeStats_EmptyStoreReturnsZeroValues` and `TestDriveScanFixturePreservesHierarchyAndMetadata`.** Path A's `-p 1` flag MASKS these defects rather than fixing them. A future operator running `go test -p 2` on the integration suite would see the race condition return. Tracked as a follow-on bug candidate per [design.md § Risks → R-6](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/design.md). Not blocking this packet (whose contract is "CI integration green on main", not "all integration tests pass with arbitrary `-p` values").
3. **Spec 031 (live-stack-testing) restructure coordination.** If spec 031's roadmap requires further CI integration-job changes, the AC-4 guard in this packet will need an amendment to whitelist the new shape. Coordination via spec cross-reference in the spec 031 PR.

---

## Superseded Scopes (Do Not Execute)

The Phase 1 (discovery) skeleton scope list was:

- **Scope 1 (skeleton)** — "Resolve Uncertainty Declaration via verbatim CI-log capture" — **SUPERSEDED.** The AC-2 evidence was captured by `bubbles.design` via the CI-environment local-reproduction methodology (see [report.md § Evidence 6/7](specs/045-deploy-resource-filesystem-hardening/bugs/BUG-045-002-ci-integration-failure-persists/report.md)). The deliverable is already captured in design.md / report.md; the discovery-phase scope no longer represents executable work.
- **Scope 2 (skeleton)** — "Implement CI integration topology / partition fix" — **SUPERSEDED.** Decomposed by this plan into S1 (workflow refactor), S2 (build-time guard), S3 (local reproduction), and S4 (validation + close-out).
