# Scopes: BUG-073-003 — Cross-language canary CI toolchain gating

## Scope 1: Toolchain-gate the canary (skip on absence, fail-loud on drift) and keep it running in CI

**Status:** [ ] In Progress

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: BUG-073-003 — Cross-language canary is environment-gated, not silently disabled

  Scenario: Toolchain absent → canary skips (not fails)
    Given node and dart are NOT on PATH (the go-only CI unit container)
    When the cross-language renderer canary tests run
    Then they report "--- SKIP" with a reason naming the missing toolchain
    And the Go unit lane exits 0

  Scenario: Toolchain present but output drifts → canary still fails loud
    Given node and dart ARE on PATH
    And a golden descriptor is deliberately corrupted
    When the cross-language renderer canary runs
    Then the affected fixture subtest FAILS with a deep-equal/drift diagnostic (t.Fatalf)
    And no skip masks the drift

  Scenario: Skip decision is non-tautological
    Given a stubbed PATH probe
    When node is reported absent then dart is reported absent then both present
    Then decideRenderToolchain returns false+node-reason, false+dart-reason, and true+empty respectively

  Scenario: Canary still runs in CI with real toolchains
    Given the dedicated cross-language-canary CI job provisions node and Flutter/dart
    When the job runs the canary
    Then the 7 cross-language fixtures execute (not skip)
    And a contract test fails the build if that job is removed or unwired
```

### Implementation Plan

1. Add `decideRenderToolchain(lookPath)` pure function + `toolchainAvailable`/`toolchainSkipReason` package vars.
2. `TestMain` calls it with `exec.LookPath`, compiles dart AOT only when available.
3. Convert the three "not on PATH" `t.Fatalf`s to top-of-test `t.Skip(toolchainSkipReason)`.
4. Keep every present-but-broken `t.Fatalf` (compile error, empty/non-exec exe, deep-equal/schema) intact.
5. Add adversarial `TestDecideRenderToolchain_*` (both-present, node-absent, dart-absent, both-absent).
6. Add the `cross-language-canary` job to `.github/workflows/ci.yml` (Go + node + Flutter + `flutter pub get` + canary `go test`).
7. Add `internal/deploy/cross_language_canary_ci_contract_test.go` guarding the job wiring (+ adversarial mutation sub-tests).

### Test Plan

| Test Type | Category | Command | Live System |
|-----------|----------|---------|-------------|
| Reproduce (container, absent) | unit | `./smackerel.sh test unit --go --go-run '<canary regex>' --verbose` (expect FAIL pre-fix, SKIP post-fix) | No |
| Skip decision adversarial | unit | `./smackerel.sh test unit --go --go-run 'TestDecideRenderToolchain' --verbose` | No |
| Drift-detection preserved (host, present) | unit | corrupt a golden → host `go test -run CrossLanguageCanary ./tests/unit/clients/` → FAIL → restore | No |
| Canary passes with toolchains (host) | unit | host `go test -run '<canary regex>' ./tests/unit/clients/` → PASS | No |
| CI-wiring contract | unit | `./smackerel.sh test unit --go --go-run 'TestCrossLanguageCanaryCIJob' --verbose` | No |
| Full Go unit lane (regression) | unit | `./smackerel.sh test unit --go` (must be green) | No |
| Lint | n/a | `./smackerel.sh lint` | No |
| Format | n/a | `./smackerel.sh format --check` | No |
| Post-push CI | e2e (CI) | watch `CI` run for pushed SHA → `lint-and-test` GREEN, `cross-language-canary` runs the canary | Yes |

### Definition of Done — 3-Part Validation

- [x] Bug reproduced before fix (container FAIL with "not on PATH")
   - Evidence (report.md#evidence-1):
     ```
     render_descriptor_canary_test.go:125: node not on PATH; ... requires both node and dart: exec: "node": executable file not found in $PATH
     --- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
     render_descriptor_canary_test.go:367: dart not on PATH; ... requires dart: exec: "dart": executable file not found in $PATH
     --- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
     FAIL    github.com/smackerel/smackerel/tests/unit/clients
     (golang:1.25.10-bookworm probe: "node ABSENT" / "dart ABSENT" — matches CI run 27392353821)
     ```
- [x] R1 — Toolchain absent → canary SKIPS (container go unit lane green post-fix)
   - Evidence (report.md#evidence-3):
     ```
     --- SKIP: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
     --- SKIP: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
     ok      github.com/smackerel/smackerel/tests/unit/clients       0.012s
     EXIT_CODE=0
     ```
- [x] R4 — Skip decision adversarial tests pass and are non-tautological
   - Evidence (report.md#evidence-4):
     ```
     --- PASS: TestDecideRenderToolchain_BothPresent_Available (0.00s)
     --- PASS: TestDecideRenderToolchain_NodeAbsent_SkipsAndNamesNode (0.00s)
     --- PASS: TestDecideRenderToolchain_DartAbsent_SkipsAndNamesDart (0.00s)
     --- PASS: TestDecideRenderToolchain_BothAbsent_Skips (0.00s)
     ```
- [x] R2 — Drift detection preserved: corrupted golden still triggers `t.Fatalf` with toolchains present
   - Evidence (report.md#evidence-5):
     ```
     render_descriptor_canary_test.go:257: js renderer output deviates from golden
     --- FAIL: TestRenderDescriptorV1_CrossLanguageCanary/text_only (0.19s)
     DRIFT_RUN_EXIT=1
     (fixture restored via `git checkout`; git status porcelain clean)
     ```
- [x] Canary passes with toolchains present (host run, no drift)
   - Evidence (report.md#evidence-2):
     ```
     --- PASS: TestRenderDescriptorV1_CrossLanguageCanary (0.45s)  [7 fixture subtests PASS]
     --- PASS: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
     ok      github.com/smackerel/smackerel/tests/unit/clients       3.241s
     ```
- [x] R3 — `cross-language-canary` CI job added; CI-wiring contract test passes; adversarial mutation sub-tests pass
   - Evidence (report.md#evidence-6):
     ```
     --- PASS: TestCrossLanguageCanaryCIJob_LiveFile (0.00s)
     --- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingJob (0.00s)
     --- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingFlutter (0.00s)
     --- PASS: TestCrossLanguageCanaryCIJob_AdversarialMissingCanaryRun (0.00s)
     ok      github.com/smackerel/smackerel/internal/deploy
     ```
- [x] Full `./smackerel.sh test unit --go` lane is green
   - Evidence (report.md#evidence-7):
     ```
     [go-unit] starting go test ./...
     ok      github.com/smackerel/smackerel/tests/unit/clients       0.010s
     [go-unit] go test ./... finished OK   (no FAIL lines across the module)
     FULL_GO_UNIT_EXIT=0
     ```
- [x] `./smackerel.sh lint` and `./smackerel.sh format --check` clean for changed files
   - Evidence (report.md#evidence-7):
     ```
     $ ./smackerel.sh format --check  → 65 files already formatted ; FORMAT_EXIT=0
     $ ./smackerel.sh lint            → All checks passed! ; Web validation passed ; LINT_EXIT=0
     ```
- [x] artifact-lint passes for the bug folder
   - Evidence (report.md#evidence-7 / validate phase):
     ```
     $ bash .github/bubbles/scripts/artifact-lint.sh specs/073-web-mobile-assistant-frontend/bugs/BUG-073-003-canary-ci-toolchain-gating
     ✅ Top-level status matches certification.status
     ✅ All checked DoD items in scopes.md have evidence blocks
     Artifact lint PASSED.
     ARTIFACT_LINT_EXIT=0
     ```
- [ ] Post-push: `CI` `lint-and-test` GREEN and `cross-language-canary` executes the canary (≥10 lines)

