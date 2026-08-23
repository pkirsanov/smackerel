# Report: BUG-069-005 Required assistant E2E false-green

## Summary

The product-owned runtime repair is implemented on the authorized branch from
starting revision `f701a6165be3979a7fce74d8eec27f06b7c79682`. The assistant
now constructs a bearer-authenticated core-to-ML compiler transport, the ML
sidecar exposes the schema-bound compiler route, and test SST selects a
profile-gated external provider fixture. Compiled clarifications and writes
compose with the existing Postgres conversation row, confirm machine, location
and weather capabilities, list store, and annotation store.

The exact five required E2E tests now report five passes, zero failures, and
zero skips. The full assistant E2E package passes with nine unrelated
profile-dependent skips. The generic Bubbles Go skip-family guard remains
foreign framework work and certification remains validate-owned.

The follow-on deterministic Barcelona weather gap is also fixed. A resolved
compiled weather intent now consumes the existing location and weather
capabilities, returns provider-qualified source metadata, and never falls
through to the generic LLM executor. The protected Barcelona E2E and the full
assistant package pass on the resulting tree; source failures remain
capture-safe and ambiguous locations still require a persistent choice before
lookup.

**Claim Source:** executed for implementation and verification; interpreted
only for ownership/routing conclusions.
**Interpretation:** The executed tests directly prove product behavior. The
remaining framework and planning-format findings are routed because their files
are outside `bubbles.implement` ownership.

## Completion Statement

Scopes 1 and 2 product implementation and implementation-adjacent tests are
delivered and verified, including the resolved compiled-weather execution path.
Required tests were strengthened without renaming; public assistant v1 fields
were unchanged. No parent spec, release train, secret, deployment, knb, target,
or framework-managed file was modified.

The packet and certification intentionally remain `in_progress`: the upstream
regression-quality guard enhancement remains foreign work, canonical
all-package E2E has not been rerun on the eventual merged-main candidate, and
security/audit/validate-owned certification are not claimed by this phase.

**Claim Source:** executed for product implementation; interpreted for the
ownership boundary.
**Interpretation:** Product work is proven, but a `route_required` result is
necessary because foreign-owned findings remain open.

## Test Evidence

### Before Fix - Accepted bubbles.test Handoff

**Executed by this agent:** NO
**Claim Source:** not-run (accepted `bubbles.test` handoff)
**Revision:** `37ed541524fe0ed61747cac929c11143b146657e`
**Repo command:** The handoff states the Smackerel repo CLI was used; the exact
command line was not supplied and is not reconstructed here.
**Reported Exit Code:** 0

```text
TestAnnotationIntentE2E_SlotsComeFromCompiledIntent                         SKIP
TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce                  SKIP
TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn               SKIP
TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation                  SKIP
TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence       SKIP
5 executed
0 required behavior passed
5 skipped
package exit 0
```

**Reported cleanup:** no files changed and no resources remained after the
reproduction.

**Uncertainty Declaration:** Full raw terminal output, timing, and the exact
selector were not supplied. The packet therefore treats the counts and test
identities as accepted handoff context, not as execution performed by
`bubbles.bug`.

### Required-Test No-Skip Guard (2026-08-23)

**Executed by this agent:** YES (current session)
**Claim Source:** executed — every line in this section was produced by a
command run in this session against the working tree.
**Phase:** implement

The reproduction above is this packet's subject: five required tests reported
package exit 0 while zero required behaviors ran. Nothing in the repository
prevented a recurrence, because the only skip guard in the tree
(`tests/e2e/agent/no_skip_guard_test.go`) resolves its scan directory from
`runtime.Caller(0)` and reads that one directory non-recursively. It covers
`tests/e2e/agent/` and nothing else, so `tests/e2e/assistant/` — where all five
required tests live — had no guard at all.

A directory-wide assertion would be the wrong instrument. Measured on this tree:
`tests/e2e/assistant/` carries roughly 30 legitimate `t.Skip`/`t.Skipf` calls
across 21 files — live-stack-unavailable guards, environment-branch guards, and
`status=planned` scenario placeholders. In that directory a skip is the norm and
its absence is the exception. The new guard is therefore a required-test
denylist bound to the five `scenario-manifest.json` identities, and it also
fails when a named required test is absent from its file, because a guard that
goes quiet when its subject is deleted or renamed reproduces this same bug in a
new form.

#### Red stage — a seeded skip must fail the guard

A `t.Skip` marker was temporarily seeded into the required test body in
`tests/e2e/assistant/http_confirm_test.go`:

```text
$ go test ./tests/e2e/assistant/ -run 'TestRequiredAssistantE2ETestsNeverSkip' -v
=== RUN   TestRequiredAssistantE2ETestsNeverSkip
    required_no_skip_guard_test.go:216: BUG-069-005 SCN-BUG069005-006 violation: 1 problem(s) with the manifest-required assistant E2E tests:
          http_confirm_test.go:28 — t.Skip-family call inside required test TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (SCN-BUG069005-005)

        A required test must FAIL loudly when its behavior is absent, never skip, and must never vanish.
        To fix: either
          (a) make the test FAIL fast on the missing dependency (preferred), or
          (b) if the requirement genuinely changed, update scenario-manifest.json FIRST and then this denylist.
        DO NOT silently bail out — a skipped required test is the false green this guard exists to prevent.
--- FAIL: TestRequiredAssistantE2ETestsNeverSkip (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      0.006s
RED_EXIT=1
```

The guard names the file, the line, the required test, and the scenario id it
belongs to, so the failure is actionable without reading the guard source.

#### Mutation restored as its own step, then verified

`trap ... EXIT` does not fire reliably in this execution environment and has
already left a mutation live in this repository once. The restore is therefore
an explicit command followed by an explicit verification, never a trap:

```text
$ git checkout -- tests/e2e/assistant/http_confirm_test.go
CHECKOUT_EXIT=0

$ git status --porcelain
 M specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/report.md
 M specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/scopes.md
?? tests/e2e/assistant/required_no_skip_guard_test.go

$ grep -n 'ORCHESTRATOR RED PROBE' tests/e2e/assistant/http_confirm_test.go
GREP_RC=1 (1 = absent = restored)

$ sed -n '27,29p' tests/e2e/assistant/http_confirm_test.go
func TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce(t *testing.T) {
        stack := loadHTTPTurnLiveStack(t)
        waitHTTPTurnHealthy(t, stack, 30*time.Second)
```

The seeded marker is gone and the required test body is byte-identical to its
committed form. No mutation was left in the tree.

#### Green stage — guard and adversarial self-test

```text
$ go test ./tests/e2e/assistant/ -run 'TestRequiredAssistantE2ETestsNeverSkip' -v
=== RUN   TestRequiredAssistantE2ETestsNeverSkip
--- PASS: TestRequiredAssistantE2ETestsNeverSkip (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.006s
GO_EXIT=0

$ go test ./tests/e2e/assistant/ -run 'TestRequiredAssistantE2ETestsNeverSkip|TestRequiredNoSkipGuard_AdversarialFinding' -v
--- PASS: TestRequiredNoSkipGuard_AdversarialFinding (0.01s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/skip_inside_required_test_is_caught (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/skipnow_and_skipf_are_caught (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/clean_required_test_reports_nothing (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/skip_in_neighbouring_func_is_ignored (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/brace_in_string_literal_does_not_break_boundaries (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/renamed_or_deleted_required_test_is_reported_missing (0.00s)
    --- PASS: TestRequiredNoSkipGuard_AdversarialFinding/method_named_skip_is_not_a_match (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.014s
GREEN_EXIT=0
```

The run is not vacuous: the `ok <pkg> <elapsed>` line is present with no
`[no tests to run]` marker, and the seven named subtests are each reported
individually, so the detector was exercised rather than merely compiled.

#### Both build modes still compile

The guard deliberately carries no `//go:build e2e` tag so it runs even when the
gated suite is switched off — a guard that only runs when the suite runs cannot
observe the suite silently not running. That makes the tagged build the thing
most at risk from the new file, so both were checked:

```text
$ go vet -tags e2e ./tests/e2e/assistant/
VET_TAGGED_EXIT=0

$ go vet ./tests/e2e/assistant/
VET_UNTAGGED_EXIT=0
```

### Static Source Checks

The following read-only checks were executed in the current session. Their full
terminal output was observed; the lines below are findings summaries, not a
substitute for future runtime test evidence.

1. Compiler SST search:
   - `config/generated/test.env` contains
     `ASSISTANT_INTENT_COMPILER_ENABLED=false`.
   - all nine compiler keys are emitted and fail-loud loaded, so the false value
     is explicit rather than a missing-key fallback.
2. Production wiring search:
   - `cmd/core` has no `NewLLMCompiler` call and no
     `WithIntentCompiler` call;
   - one comment says retrieval activates once Spec 068 is wired.
3. ML route search:
   - `/assistant/intent/compile` appears in Go compiler comments/contracts;
   - no Python/ML route handler matched.
4. Required-test search:
   - each of the five exact tests has a behavior-dependent `t.Skipf` branch.
5. Facade/state search:
   - clarification returns plain body text and persists `PendingClarify`;
   - write gating returns capture fallback and no `ConfirmCard`;
   - the existing confirm machine separately supports durable proposal and
     exactly-once resolution.
6. Framework guard search:
   - `regression-quality-guard.sh` scans conditional returns and optional
     assertions but has no Go skip-family pattern;
   - Bubbles agent policy separately names skip scans, exposing the mechanical
     enforcement gap.

**Claim Source:** executed (read-only source searches) and interpreted (root
cause synthesis).

## Authority Findings

### Spec 068

- Compiler-before-route is mandatory for every non-operational NL turn.
- Clarify must not guess a route.
- Write and external-write actions require confirmation.
- HTTP E2E for SCN-068-A01/A02/A03/A04/A05/A06/A07/A09 is explicitly assigned
  to Spec 069.

### Spec 069

- SCN-069-A03 and SCN-069-A04 require real HTTP disambiguation and confirm
  round trips.
- Its scenario manifest links all five reproduced tests as required E2E API
  coverage.
- Parent state certifies all scopes done, which conflicts with the reproduced
  zero-behavior result. This packet records but does not edit that foreign
  certification surface.

### Spec 071

- IntentTrace assumes compiler execution and traces every compiled turn.
- It does not authorize compiler disablement or replace pending interaction
  state.

### Spec 076

- Disambiguation and confirm-card parity remain required across transports.
- Its only explicit post-release exceptions are annotation-map removal and
  native mobile accessibility infrastructure.
- It does not supersede the five Spec 069 tests.

## Root Cause Conclusion

**Classification:** genuine runtime incompleteness plus required-test
false-green; not intentional disablement and not stale traceability.

The smallest complete repair is the vertical composition documented in
[design.md](design.md): explicit compiler-enabled test SST, real ML provider
route, core compiler attachment, existing persistent disambiguation/confirm
machines, stable v1 HTTP controls, deterministic external-provider fixture, and
strict no-skip required E2E.

## Post-Fix Evidence

### Scope 1 Go And Config Unit GREEN

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && ./smackerel.sh test unit --go`
**Exit Code:** 0
**Output (relevant window from full output):**

```text
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core
ok      github.com/smackerel/smackerel/internal/agent/tools/microtools
ok      github.com/smackerel/smackerel/internal/agent/tools/weather
ok      github.com/smackerel/smackerel/internal/assistant
ok      github.com/smackerel/smackerel/internal/assistant/confirm
ok      github.com/smackerel/smackerel/internal/assistant/context
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter
ok      github.com/smackerel/smackerel/internal/assistant/intent
ok      github.com/smackerel/smackerel/internal/config
[go-unit] go test ./... finished OK
```

**Result:** PASS

### Scope 1 Python Compiler Route GREEN

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && ./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
[py-unit] pip install OK; starting unit-only pytest ml/tests
........................................................................ [ 10%]
........................................................................ [ 20%]
........................................................................ [ 30%]
........................................................................ [ 40%]
........................................................................ [ 50%]
........................................................................ [ 60%]
........................................................................ [ 70%]
........................................................................ [ 81%]
........................................................................ [ 91%]
...............................................................          [100%]
718 passed, 2 deselected in 12.72s
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS

### Live Compiler And Pending-State Integration GREEN

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
**Exit Code:** 0
**Output (complete named-test window):**

```text
go-integration: applying -run selector: ^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$
=== RUN   TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
--- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.07s)
=== RUN   TestCompilerInteractiveControlsPersistByUserAndTransport
--- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.227s
PASS: go-integration
[py-integration] starting live integration pytest
.                                                                        [100%]
1 passed in 0.41s
[py-integration] live integration pytest finished OK
PASS: python-integration
```

**Result:** PASS

### Exact Five Required GREEN

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^(TestAnnotationIntentE2E_SlotsComeFromCompiledIntent|TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation|TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn|TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence|TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce)$'`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.13s)
=== RUN   TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.07s)
=== RUN   TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.06s)
=== RUN   TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.04s)
=== RUN   TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.376s
PASS: go-e2e
```

**Result:** PASS - five passed, zero failed, zero skipped.

### Full Assistant E2E Package GREEN

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant`
**Exit Code:** 0
**Output (package verdict and observed skip search):**

```text
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.10s)
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.05s)
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.04s)
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.04s)
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.04s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      65.658s
PASS: go-e2e
--- SKIP: TestAssistantE2E_CaptureAcknowledgementIsCrossTransportIdentical_TP_074_17
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackDedupWithinWindow_TP_074_11
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackIsInviolable_TP_074_04
--- SKIP: TestAssistantHTTPE2E_CaptureProvenanceIsDistinct_TP_074_07
--- SKIP: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation
--- SKIP: TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice
--- SKIP: TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario
--- SKIP: TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams
--- SKIP: TestMicroToolsE2E_CalculatorRejectsUnsafeExpression
REQUIRED_FIVE_SKIP_CALLS=0
```

**Result:** PASS - full package green; nine unrelated optional/profile-dependent
tests skipped; none of the protected five contains or reported a skip.

## Implemented Test Traceability

The following concrete paths were implemented or strengthened and executed in
the current session:

- `internal/config/assistant_intent_compiler_test.go` - fail-loud cross-field
  coherence; Go unit lane passed.
- `internal/assistant/intent/http_transport_test.go` - authenticated route,
  schema, and response-size bounds; Go unit lane passed.
- `ml/tests/test_intent_compiler.py` - schema-bound route and provider drift;
  Python unit lane passed.
- `ml/tests/test_intent_compiler_provider_fixture.py` - external fixture
  compiled-slot contract; Python unit lane passed.
- `tests/integration/assistant/bug069005_runtime_canary_test.go` - live
  core-to-ML compiler plus durable pending state; focused integration passed.
- `tests/e2e/assistant/annotation_intent_test.go` - adversarial compiled slots,
  zero pre-confirm effect, three exact annotation events after accept.
- `tests/e2e/assistant/intent_clarify_test.go` - persistent Springfield choices
  and no weather result before selection.
- `tests/e2e/assistant/http_disambiguation_test.go` - selected server-owned
  candidate clears pending state and drives real weather lookup.
- `tests/e2e/assistant/intent_side_effect_test.go` - valid list write returns a
  non-capture ConfirmCard and persists nothing before accept.
- `tests/e2e/assistant/http_confirm_test.go` - one list effect after accept and
  no additional effect on replay.
- Upstream `bubbles/scripts/regression-quality-guard-selftest.sh` - foreign
  framework test remains routed to the Bubbles owner; not run or modified here.

**Claim Source:** executed for product tests; not-run for the foreign framework
selftest.

## Packet Gates

### Artifact Lint

**Executed:** YES (current session)
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && bash .github/bubbles/scripts/artifact-lint.sh specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green`
**Exit Code:** 0
**Output:**

```text
Required artifact exists: spec.md
Required artifact exists: design.md
Required artifact exists: uservalidation.md
Required artifact exists: state.json
Required artifact exists: scopes.md
Required artifact exists: report.md
No forbidden sidecar artifacts present
Found DoD section in scopes.md
scopes.md DoD contains checkbox items
All DoD bullet items use checkbox syntax in scopes.md
Found Checklist section in uservalidation.md
uservalidation checklist contains checkbox entries
Detected state.json status: in_progress
Detected state.json workflowMode: bugfix-fastlane
Top-level status matches certification.status
All checked DoD items in scopes.md have evidence blocks
No unfilled evidence template placeholders in scopes.md
No unfilled evidence template placeholders in report.md
No repo-CLI bypass detected in report.md command evidence
Artifact lint PASSED.
```

**Result:** PASS

### Traceability Guard

**Executed:** YES (current session)
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && bash .github/bubbles/scripts/traceability-guard.sh specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green`
**Exit Code:** 0
**Output:**

```text
--- Scenario Manifest Cross-Check (G057/G059) ---
scenario-manifest.json covers 6 scenario contract(s)
scenario-manifest.json linked test exists: tests/e2e/assistant/annotation_intent_test.go
scenario-manifest.json linked test exists: tests/e2e/assistant/intent_clarify_test.go
scenario-manifest.json linked test exists: tests/e2e/assistant/http_disambiguation_test.go
scenario-manifest.json linked test exists: tests/e2e/assistant/intent_side_effect_test.go
scenario-manifest.json linked test exists: tests/e2e/assistant/http_confirm_test.go
scenario-manifest.json records evidenceRefs
All linked tests from scenario-manifest.json exist
--- Gherkin -> DoD Content Fidelity (Gate G068) ---
DoD fidelity: 6 scenarios checked, 6 mapped to DoD, 0 unmapped
--- Traceability Summary ---
Scenarios checked: 6
Test rows checked: 16
Scenario-to-row mappings: 6
Concrete test file references: 6
Report evidence references: 6
DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
RESULT: PASSED (0 warnings)
```

**Result:** PASS

### Build Quality And Governance

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Commands:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`; `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh lint`; `./smackerel.sh format --check`; packet artifact lint; traceability guard; implementation reality scan; installed regression-quality guard.
**Exit Codes:** all 0
**Output (verdict window):**

```text
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
Web validation passed
78 files already formatted
Artifact lint PASSED.
RESULT: PASSED (0 warnings)
Files scanned:  2
Violations:     0
Warnings:       1
PASSED with 1 warning(s) - manual review advised
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 5
Files with adversarial signals: 5
```

**Result:** PASS with one routed planning-format warning from implementation
reality: scope prose names file families rather than concrete implementation
paths. Zero implementation violations were found.

### Cleanup

**Executed:** YES (current session)
**Phase:** implement
**Claim Source:** executed
**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh --env test down --volumes && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh --env test status`
**Exit Code:** 0
**Output:**

```text
Container smackerel-test-smackerel-core-1 Removed
Container smackerel-test-postgres-1 Removed
Container smackerel-test-intent-compiler-provider-1 Removed
Container smackerel-test-smackerel-ml-1 Removed
Container smackerel-test-nats-1 Removed
Volume smackerel-test-nats-data Removed
Volume smackerel-test-ollama-data Removed
Volume smackerel-test-postgres-data Removed
Network smackerel-test_default Removed
NAME      IMAGE     COMMAND   SERVICE   CREATED   STATUS    PORTS
curl: (7) Failed to connect to 127.0.0.1 port 45001 after 0 ms: Couldn't connect to server
```

**Result:** PASS - teardown was idempotent and no test service remained.

### Open Owned Findings

1. `TR-BUG-069-005-FRAMEWORK-001` remains open for the Bubbles owner: the
  generic guard must detect required Go `t.Skip`, `t.Skipf`, and `t.SkipNow`
  mechanically and prove it with upstream selftests.
2. `F-BUG069005-PLAN-PATHS` is routed to `bubbles.plan`: the implementation
  reality scanner found zero violations but warned that scope implementation
  paths are family prose rather than concrete file references.
3. Parent Spec 069 state/certification reconciliation remains validate-owned.

**Claim Source:** interpreted from executed guards and artifact ownership.
**Interpretation:** These findings do not invalidate the product GREEN, but
they prevent this implement phase from claiming terminal packet completion.

## Invocation Audit

No subagents were invoked. This top-level authorized `bubbles.implement`
dispatch implemented product-owned Scopes 1 and 2, strengthened the exact five
required tests, ran implementation-phase validation, and recorded evidence.
Independent `bubbles.test`, `bubbles.security`, `bubbles.regression`,
`bubbles.validate`, and `bubbles.audit` phases were not claimed. Certification
and parent-state reconciliation remain outside this agent's ownership.

**Claim Source:** executed for this invocation's tool and edit history.

## Final Product Test Phase - Combined Candidate (2026-07-21)

### Test Phase Provenance And Verdict

**Agent:** `bubbles.test`
**Claim Source:** executed
**Candidate worktree:** `~/smackerel-bug-spec069-deterministic-e2e-20260720`
**Branch:** `bug/spec069-deterministic-e2e-20260720`
**Exact starting HEAD:** `1493aa0100de8dbb19599857fdf83e08a3e8f1df`
**Initial worktree:** clean; branch divergence from its upstream was `+0 -0`.
**Hardware profile:** every repo-CLI test/check/lint/format command used
`SMACKEREL_HARDWARE_TIER=cpu`.

The final product test phase is **BLOCKED / route-required**, not certified.
The protected five are green, topic-momentum regressions are green, language
unit lanes and product guards are green, and disposable-state cleanup is clean.
Two new required-test findings prevent a completed test-phase claim:

1. Parent Spec 069 required test
   `TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation`
   skipped in both the full assistant package and canonical complete E2E run
   because capture-as-fallback fired. This test was not one of the prior
   21 classified suite items. Its source still contains a behavior-dependent
   `t.Skipf`, while the combined test profile now supplies a deterministic
   external compiler-provider fixture. Owner route: `bubbles.test` for the
   parent Spec 069 HTTP weather E2E/provider-fixture contract.
2. Both focused BUG-069-005 Go live-integration tests failed before reaching
   their compiler/persistence assertions: the endpoint returned HTTP `503`
   with `ErrorCause:assistant_http_not_ready`. The canary posts immediately;
   the adjacent Spec 069 bind test has an explicit readiness wait. Owner route:
   `bubbles.test` for `tests/integration/assistant/bug069005_runtime_canary_test.go`
   and the integration readiness contract.

Certification remains `in_progress`. No `regression`, `security`, `validate`,
or `audit` phase is claimed. No production source, parent packet, Bubbles
framework source, installed `.github/bubbles/**`, deployment, release-train,
secret, knb, or target-host surface was modified.

### Exact Five Protected Spec 069 E2E

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^(TestAnnotationIntentE2E_SlotsComeFromCompiledIntent|TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce|TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn|TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation|TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence)$'`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.11s)
=== RUN   TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.05s)
=== RUN   TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.04s)
=== RUN   TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.03s)
=== RUN   TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.03s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.302s
PASS: go-e2e
```

**Result:** PASS - exactly 5 passed, 0 failed, 0 skipped. The runner also
reported the already-classified opt-in Ollama agent profile outside the selected
assistant package; it did not skip any selected test.

### Full Assistant E2E Package

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant`
**Exit Code:** 0
**Top-level accounting:** 48 passed, 0 failed, 5 skipped.
**Output:**

```text
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.18s)
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.06s)
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.05s)
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.07s)
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.03s)
--- SKIP: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation (0.03s)
--- SKIP: TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice (0.00s)
--- SKIP: TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario (0.00s)
--- SKIP: TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams (0.50s)
--- SKIP: TestMicroToolsE2E_CalculatorRejectsUnsafeExpression (0.21s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      70.586s
PASS: go-e2e
```

Skip disposition:

- The two legacy retirement tests require the mutually exclusive Telegram
  webhook-mode profile and retain their existing profile disposition.
- The two microtool tests remain the already-classified Spec 065 surfaces
  superseded into Spec 076; they are not claimed passed.
- The weather compiler test is a new required skip. Parent Spec 069 lists it as
  an `e2e-api` Test Plan row for cross-spec scenario SCN-068-A01, so it cannot be
  dispositioned as optional or claimed passed.

**Result:** BLOCKING FINDING despite package exit 0 - all protected five pass,
but one adjacent parent-required assistant scenario skipped.

### Topic Momentum Combined-Tree Regression

#### Canonical PostgreSQL Integration

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration-light --go-run '^TestTopicLifecycleMomentumFromPersistedStars$'`
**Exit Code:** 0
**Output:**

```text
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
=== RUN   TestTopicLifecycleMomentumFromPersistedStars
topic_lifecycle_momentum_test.go:35: PASS: canonical relationship uniqueness rejects duplicate BELONGS_TO edges
INFO topic state transition topic=<fixture>-zero from=emerging to=dormant momentum=0.5
INFO topic state transition topic=<fixture>-multiple from=emerging to=active momentum=11.5
topic_lifecycle_momentum_test.go:44: zero-star persisted momentum=0.5000 state=dormant
topic_lifecycle_momentum_test.go:48: multiple-stars persisted momentum=11.5000 state=active
topic_lifecycle_momentum_test.go:50: PASS: canonical topics schema has no star_count column
topic_lifecycle_momentum_test.go:51: PASS: zero linked starred artifacts contribute 0.0 star momentum
topic_lifecycle_momentum_test.go:52: PASS: one linked unstarred artifact contributes only 0.5 connection momentum
topic_lifecycle_momentum_test.go:53: PASS: two distinct linked starred artifacts contribute exactly 10.0 star momentum
topic_lifecycle_momentum_test.go:55: PASS: an unrelated starred artifact contributes nothing to the tested topic
--- PASS: TestTopicLifecycleMomentumFromPersistedStars (0.07s)
PASS
ok      github.com/smackerel/smackerel/tests/integration        0.231s
```

**Result:** PASS - one selected test passed against disposable PostgreSQL and
canonical migrations; the duplicate-edge correction remained adversarial and
green.

#### Focused Scheduler And Lifecycle

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^(TestCalculateMomentum.*|TestTransitionState.*|TestDefaultMomentumConfig|TestNewLifecycle|TestTopicMomentumJob_LogsLifecycleQueryFailure)$' --verbose`
**Exit Code:** 0
**Output:**

```text
=== RUN   TestTopicMomentumJob_LogsLifecycleQueryFailure
--- PASS: TestTopicMomentumJob_LogsLifecycleQueryFailure (0.00s)
ok      github.com/smackerel/smackerel/internal/scheduler       0.049s
=== RUN   TestCalculateMomentum
--- PASS: TestCalculateMomentum (0.00s)
=== RUN   TestTransitionState
--- PASS: TestTransitionState (0.00s)
=== RUN   TestDefaultMomentumConfig
--- PASS: TestDefaultMomentumConfig (0.00s)
=== RUN   TestCalculateMomentum_StarsAndConnections
--- PASS: TestCalculateMomentum_StarsAndConnections (0.00s)
=== RUN   TestTransitionState_ArchivedResurfaces
--- PASS: TestTransitionState_ArchivedResurfaces (0.00s)
=== RUN   TestNewLifecycle
--- PASS: TestNewLifecycle (0.00s)
ok      github.com/smackerel/smackerel/internal/topics  0.008s
[go-unit] go test ./... finished OK
```

**Result:** PASS - every selected scheduler/lifecycle test passed; no selected
test skipped.

#### Targeted Topic Lifecycle E2E

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --shell-run test_topic_lifecycle.sh`
**Exit Code:** 0
**Output:**

```text
=== Topic Lifecycle E2E Tests ===
Waiting for services to be healthy (max 120s)...
Services healthy after 0s
Seeding topics...
Existing pricing topic owner: topic-lifecycle-existing-pricing
PASS: Adversarial pricing topic present without duplicate collision
Hot topic momentum: 20
Dormant topic momentum: 0.1
PASS: Topic lifecycle: states and momentum verified
=== Topic Lifecycle E2E tests passed ===
PASS: test_topic_lifecycle.sh
Total:  1
Passed: 1
Failed: 0
```

**Result:** PASS - 1 passed, 0 failed, 0 skipped; teardown removed the full
disposable stack.

### Canonical Complete E2E - One Run

**Executed:** YES (current session; exactly one canonical complete run)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e`
**Exit Code:** 0
**Top-level accounting:** 136 passed, 0 failed, 16 Go skips, plus 1
harness-level opt-in Ollama-suite skip.
**Package accounting:** 12 passed, 0 failed.
**Output:**

```text
ok      github.com/smackerel/smackerel/tests/e2e/agent  8.961s
ok      github.com/smackerel/smackerel/tests/e2e/assistant      66.493s
ok      github.com/smackerel/smackerel/tests/e2e/auth   0.527s
ok      github.com/smackerel/smackerel/tests/e2e/capture        0.006s
ok      github.com/smackerel/smackerel/tests/e2e/drive  11.214s
ok      github.com/smackerel/smackerel/tests/e2e/foundation     3.494s
ok      github.com/smackerel/smackerel/tests/e2e/legacy_retirement      0.859s
ok      github.com/smackerel/smackerel/tests/e2e/microtools     0.052s
ok      github.com/smackerel/smackerel/tests/e2e/openknowledge  0.078s
ok      github.com/smackerel/smackerel/tests/e2e/policy 0.009s
ok      github.com/smackerel/smackerel/tests/e2e/transports     0.074s
ok      github.com/smackerel/smackerel/tests/e2e/wiki   0.180s
PASS: go-e2e
Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
```

Complete skip reconciliation:

| Family | Count | Disposition |
|---|---:|---|
| OpenKnowledge `A01`-`A06` live invoke tests | 7 | Existing armed-profile/harness route: `AGENT_INVOKE_URL` not exposed; not claimed passed |
| Planned Spec 074 capture policy tests | 4 | Existing `status=planned` rows; not claimed passed |
| Protected BUG-069-005 tests | 0 skipped / 5 passed | Fixed blockers executed and passed |
| Legacy open/expired window tests | 2 | Existing mutually exclusive Telegram webhook profile; not claimed passed |
| Superseded microtool live-LLM tests | 2 | Existing Spec 065 -> Spec 076 disposition; not claimed passed |
| Ollama agent suite | 1 harness suite | Existing opt-in hardware/profile disposition; not claimed passed |
| Parent Spec 069 weather compiler HTTP E2E | 1 | **NEW REQUIRED SKIP - route required** |

The prior 21-item inventory is therefore reconciled honestly: 15 previously
classified Go skips remain, the 5 former BUG-069-005 blockers now pass, and the
existing Ollama harness profile remains omitted. The weather compiler test is a
new seventeenth current skip surface and is not covered by that prior
classification.

**Result:** BLOCKING FINDING despite command/package success. The canonical run
does not establish a zero-required-skip broader-E2E pass, so BUG-003-002's
broader-E2E DoD remains unchecked and its report is intentionally unchanged.

### Go And Python Unit Lanes

#### Go Unit

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go`
**Exit Code:** 0
**Accounting:** 140 `ok` packages, 6 packages with no test files, 0 failures,
0 skips.
**Output:**

```text
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core 1.260s
ok      github.com/smackerel/smackerel/internal/assistant/context       (cached)
ok      github.com/smackerel/smackerel/internal/assistant/intent        (cached)
ok      github.com/smackerel/smackerel/internal/config  34.157s
ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
ok      github.com/smackerel/smackerel/internal/topics  (cached)
ok      github.com/smackerel/smackerel/tests/e2e/assistant      (cached)
ok      github.com/smackerel/smackerel/tests/integration/assistant      (cached)
ok      github.com/smackerel/smackerel/tests/observability      (cached)
[go-unit] go test ./... finished OK
```

**Result:** PASS.

#### Python Unit

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --python`
**Exit Code:** 0
**Output:**

```text
[py-unit] pip install OK; starting unit-only pytest ml/tests
........................................................................ [ 10%]
........................................................................ [ 20%]
........................................................................ [ 30%]
........................................................................ [ 40%]
........................................................................ [ 50%]
........................................................................ [ 60%]
........................................................................ [ 70%]
........................................................................ [ 81%]
........................................................................ [ 91%]
...............................................................          [100%]
718 passed, 2 deselected in 13.05s
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS - 718 passed, 0 failed, 0 skipped, 2 explicitly deselected by
the unit-only profile.

### Focused Live Compiler And Persistence Integration

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
**Exit Code:** 1
**Output:**

```text
go-integration: applying -run selector: ^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$
=== RUN   TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
bug069005_runtime_canary_test.go:91: assistant status = 503, want 200; envelope={SchemaVersion:v1 Transport:web Status:unavailable ErrorCause:assistant_http_not_ready CaptureRoute:false FacadeInvoked:false}
--- FAIL: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.00s)
=== RUN   TestCompilerInteractiveControlsPersistByUserAndTransport
bug069005_runtime_canary_test.go:149: assistant status = 503, want 200; envelope={SchemaVersion:v1 Transport:web Status:unavailable ErrorCause:assistant_http_not_ready CaptureRoute:false FacadeInvoked:false}
--- FAIL: TestCompilerInteractiveControlsPersistByUserAndTransport (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/assistant      0.132s
FAIL: go-integration (exit=1)
[py-integration] pip install OK; starting live integration pytest
.                                                                        [100%]
1 passed in 0.54s
[py-integration] live integration pytest finished OK
PASS: python-integration
```

**Result:** FAIL - Go 0 passed / 2 failed / 0 skipped; Python 1 passed / 0
failed / 0 skipped. The command's cleanup removed all test resources.

### Check, Lint, And Format

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Commands (home paths normalized):**

- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`
- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh lint`
- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh format --check`

**Exit Codes:** 0, 0, 0
**Output:**

```text
config-validate: ~/smackerel-bug-spec069-deterministic-e2e-20260720/config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
OK: PWA manifest has required fields
OK: Chrome extension manifest has required fields (MV3)
OK: Firefox extension manifest has required fields (MV2 + gecko)
OK: web/pwa/app.js
OK: web/pwa/sw.js
OK: web/extension/background.js
OK: web/extension/popup/popup.js
OK: Extension versions match (1.0.0)
Web validation passed
78 files already formatted
```

**Result:** PASS - no warning or failure line was emitted by these three gates.

### Packet Guards And Test Fidelity

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Primary packet results:** artifact lint PASS; traceability PASS with 6/6
scenarios mapped and 0 warnings; implementation reality PASS over 34 files with
0 violations and 0 warnings; installed regression guard PASS in normal and
`--bugfix` modes over 5 files, with adversarial signals in all 5.
**Topic packet results:** artifact lint PASS with its two pre-existing deprecated
state-field advisories; traceability PASS with 3/3 scenarios and 0 warnings;
implementation reality PASS over 1 file with 0 violations and 0 warnings;
installed regression guard PASS in normal and `--bugfix` modes over 3 files,
with adversarial signals in all 3.

```text
Artifact lint PASSED.
RESULT: PASSED (0 warnings)
Files scanned:  34
Violations:     0
Warnings:       0
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 5
Files with adversarial signals: 5
PROTECTED_SKIP_FAMILY_SCAN_EXIT=1
PRIMARY_LIVE_MOCK_SCAN_EXIT=1
Artifact lint PASSED.
state.json uses deprecated field 'scopeProgress'
state.json uses deprecated field 'scopeLayout'
RESULT: PASSED (0 warnings)
Files scanned:  1
Violations:     0
Warnings:       0
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 3
Files with adversarial signals: 3
TOPIC_SKIP_MARKER_SCAN_EXIT=1
TOPIC_LIVE_MOCK_SCAN_EXIT=1
```

`grep` exit `1` in these marker/mock scans means zero matches. The foreign
Bubbles Go skip-family detector finding remains open and is not claimed fixed.
The installed guard's clean result plus the explicit zero-match product scan
prove only the current protected files; they do not prove upstream generic
detection capability.

### Final Test Resource Cleanup

**Executed:** YES (current session)
**Phase:** test
**Claim Source:** executed
**Command:** explicit idempotent `./smackerel.sh --env test down --volumes`,
followed by name-only container/network/volume and process checks.
**Exit Code:** 0
**Output:**

```text
FINAL SMACKEREL TEST RESOURCE CHECK
containers:
<none>
networks:
<none>
volumes:
<none>
processes:
<none>
resource verdict:
PASS: zero residual smackerel-test resources or processes
```

**Result:** PASS.

### Test-Owned DoD Disposition

- Supported for checking: exact-five TP-BUG069005-10; SCN-BUG069005-006
  healthy-run/fail-closed proof; accepted pre-fix semantic RED paired with this
  session's exact-five GREEN; packet artifact lint and traceability.
- Remains unchecked: TP-BUG069005-11's upstream generic skip-family rejection
  claim (foreign guard not fixed); TP-BUG069005-13 (parent required weather
  E2E skipped); scenario-specific/broader E2E completion; validate-owned
  certification.
- Scope 1/2 Build Quality remains unchecked because the focused required Go
  integration selector failed 2/2.
- BUG-003-002 receives no report or DoD edit: the user-authorized cross-packet
  update condition required a canonical suite with no new required skip, and
  that condition was not met.

### Test Invocation Audit

No subagent, regression, security, validation, audit, deployment, or release
operation was invoked. This top-level `bubbles.test` run executed the product
tests and product-owned guards listed above, recorded observed pass/fail/skip
results, and stopped certification on the two new required-test findings.
Because scoped required tests are not green, no commit or push is authorized by
the user's close-out rule.

**Claim Source:** executed for commands and observed output; interpreted only
for ownership routing and DoD disposition.

## Discovered Issues

| Date | Finding | Disposition | Reference |
|---|---|---|---|
| 2026-07-21 | Parent-required `TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation` called `t.Skipf` after deterministic-profile capture fallback in both assistant and canonical E2E runs | addressed: the deterministic fixture emits the structured Barcelona slot, the protected test fails instead of skipping, and the production facade executes the sourced weather path | `tests/e2e/assistant/intent_compiler_http_test.go`; `tests/e2e/intent-compiler-provider/provider.py`; [closeout evidence](#weather-implementation-and-test-closeout) |
| 2026-07-21 | Both BUG-069-005 focused Go integration canaries received HTTP 503 `assistant_http_not_ready` before compiler/persistence assertions; adjacent Spec 069 live bind coverage waits for readiness | addressed: both canaries use bounded facade-readiness polling and pass against the disposable stack | `tests/integration/assistant/bug069005_runtime_canary_test.go`; [closeout evidence](#weather-implementation-and-test-closeout) |
| 2026-07-21 | Canonical runner omitted the opt-in Ollama agent suite and retained 15 previously classified Go skips across OpenKnowledge, planned Spec 074, legacy-window, and superseded microtool profiles | classified non-applicable/planned/profile surfaces for this packet and never claimed passed; the new weather skip is separately blocking above | `specs/064-open-ended-knowledge-agent/`; `specs/074-capture-as-fallback-policy/`; `specs/065-generic-micro-tools/`; `specs/076-assistant-completion-rescope/`; [skip accounting](#canonical-complete-e2e---one-run) |
| 2026-07-21 | Installed Bubbles regression guard still lacks generic Go `t.Skip`/`t.Skipf`/`t.SkipNow` detection | existing foreign route remains open; Smackerel installed framework files were not edited and upstream repair is not claimed | `TR-BUG-069-005-FRAMEWORK-001` in `state.json`; `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/design.md#8-framework-guard-routing` |
| 2026-08-23 | `tests/e2e/assistant/` had NO skip guard of any kind. The sibling guard resolves its scan directory from `runtime.Caller(0)` and reads that one directory non-recursively, so it covers `tests/e2e/agent/` only. All five manifest-required assistant tests were therefore unguarded — the exact hole this packet's false green fell through | fixed-in-session: added a required-test denylist guard asserting the five manifest-required tests contain no `t.Skip`-family call in their function bodies AND that none of the five has been deleted or renamed. RED proved by seeding a marker into one required test (caught at `http_confirm_test.go:28`, exit 1); mutation restored via `git checkout --` and re-verified clean; GREEN re-run exit 0 with 2 tests + 7 subtests executed | `tests/e2e/assistant/required_no_skip_guard_test.go`; [Required-Test No-Skip Guard](#required-test-no-skip-guard-2026-08-23) |
| 2026-08-23 | The G095 scanner flags the word `skipping` at `report.md:977`. That sentence is preserved historical RED evidence — it records that a required weather E2E stopped bailing out and began failing loudly — so it is a scanner match on a real English word, not a deferral | status-adjusted (documentation-only): the historical sentence is retained VERBATIM because rewording preserved red evidence would itself be evidence tampering; disposition is recorded here instead per remediation option (b). The underlying substantive finding is split into the two rows adjacent to this one, both of which are present in this same table | `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/report.md#test-continuation-deterministic-weather-and-readiness-2026-07-21` |
| 2026-08-23 | The finding behind `TR-BUG-069-005-FRAMEWORK-001` has two halves that were previously tracked as one, which made its product disposition unreadable | routed (upstream half only): the PRODUCT half — assistant required tests being unguarded in this repo — is now closed in-repo by the new guard. The FRAMEWORK half — the installed regression guard's own blindness to Go skip tokens — is owned by the upstream framework repository and is not repairable from here, because `.github/bubbles/` is framework-managed and MUST NOT be edited downstream | `TR-BUG-069-005-FRAMEWORK-001` in `state.json`; `tests/e2e/assistant/required_no_skip_guard_test.go` |

## Test Continuation: Deterministic Weather And Readiness (2026-07-21)

### Continuation Findings And Verdict

**Agent:** `bubbles.test`
**Claim Source:** executed for commands; interpreted for source-path ownership
**Starting HEAD:** `1493aa0100de8dbb19599857fdf83e08a3e8f1df`

| Finding | Result | Disposition |
|---|---|---|
| `F-BUG069005-CONT-001` - deterministic Barcelona compiler output omitted the structured location slot | addressed in test-owned provider fixture; focused unit and live compiler canary pass | `tests/e2e/intent-compiler-provider/provider.py`; `ml/tests/test_intent_compiler_provider_fixture.py` |
| `F-BUG069005-CONT-002` - integration canaries raced asynchronous assistant facade binding | addressed with bounded benign-reset polling: 60 second deadline, 2 second interval, 10 second request timeout, fail-loud last status/envelope/error | `tests/integration/assistant/bug069005_runtime_canary_test.go` |
| `F-BUG069005-CONT-003` - a correctly compiled unambiguous Barcelona weather turn still reaches provenance capture fallback | addressed: resolved compiled weather now executes through the injected location/weather capabilities and returns provider-qualified source metadata | `internal/assistant/compiled_interactions.go`; `internal/assistant/facade.go`; `internal/assistant/compiled_weather_test.go` |

The continuation verdict at that point was **BLOCKED / route-required**. The provider fixture,
skip-family source guard, and two live canaries are green. The exact required
weather E2E now fails loudly instead of skipping, but still reports
`CaptureRoute=true`. Production changes were explicitly forbidden in this
invocation, so the protected-five/full-assistant sequence was stopped and no
commit or push was authorized. The later implementation and green closeout are
recorded separately below so this red evidence remains intact.

### Focused Barcelona Provider Unit

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --python --python-k 'test_barcelona_weather_fixture_carries_canonical_location_slot' --verbose`
**Exit Code:** 0
**Output:**

```text
[py-unit] starting pip install -e ./ml[dev]
Obtaining file:///workspace/ml
Installing build dependencies: started
Installing build dependencies: finished with status 'done'
Checking if build backend supports build_editable: started
Checking if build backend supports build_editable: finished with status 'done'
Successfully built smackerel-ml
[py-unit] pip install OK; starting unit-only pytest ml/tests
.                                                                        [100%]
1 passed, 720 deselected in 0.87s
[py-unit] pytest ml/tests finished OK
```

**Result:** PASS - 1 selected passed, 0 failed, 0 skipped; 720 tests were
explicitly deselected by the focused selector.

### Protected Weather Skip-Family Source Guard

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall$'`
**Exit Code:** 0
**Output:**

```text
[go-e2e] nodejs install OK
go-e2e: applying package selector: assistant
go-e2e: applying -run selector: ^TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall$
=== RUN   TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall
--- PASS: TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.036s
PASS: go-e2e
Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

**Result:** PASS - the focused AST assertion found the protected weather test
and found zero calls to `t.Skip`, `t.Skipf`, or `t.SkipNow`. The runner's
separate opt-in Ollama profile remained omitted and is not claimed passed.

### Exact Weather E2E - Fail-Loud Blocker

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation$'`
**Exit Code:** 1
**Output:**

```text
go-e2e: applying package selector: assistant
go-e2e: applying -run selector: ^TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation$
=== RUN   TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation
    intent_compiler_http_test.go:111: deterministic weather fixture routed to capture fallback; envelope={SchemaVersion:v1 Transport:web TransportMessageID:e2e-scope1c-068a01-20260721T025209.164 Status:saved_as_idea Body:saved as an idea — i'll surface it later. Sources:[] SourcesOverflowCount:0 ConfirmCard:<nil> DisambiguationPrompt:<nil> ErrorCause: CaptureRoute:true Trace:{AssistantTurnID:trace_20260721T025209.207058441_8 AgentTraceID:trace_20260721T025209.207058441_8 RequestID:5641854bed5a/z0bLCQDR8f-000007} FacadeInvoked:true EmittedAt:2026-07-21T02:52:09.167901452Z Notice:<nil>}
--- FAIL: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation (7.33s)
FAIL
FAIL    github.com/smackerel/smackerel/tests/e2e/assistant      7.364s
FAIL
FAIL: go-e2e (exit=1)
Skipping Ollama agent E2E (set SMACKEREL_TEST_OLLAMA=1 to enable tests/e2e/agent/happy_path_test.go)
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
```

**Result:** FAIL - exact accounting is 0 passed, 1 failed, 0 skipped. The old
false-green skip is removed; the required scenario now exposes the production
failure directly.

### Exact Two Live Integration Canaries

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Command (home path normalized):** `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
**Exit Code:** 0
**Output:**

```text
go-integration: applying -run selector: ^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$
=== RUN   TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
--- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler (0.09s)
=== RUN   TestCompilerInteractiveControlsPersistByUserAndTransport
--- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport (0.05s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/assistant      0.246s
PASS: go-integration
[py-integration] pip install OK; starting live integration pytest
.                                                                        [100%]
1 passed in 0.41s
[py-integration] live integration pytest finished OK
PASS: python-integration
```

**Result:** PASS - exact selected Go accounting is 2 passed, 0 failed, 0
skipped. The first canary also proves the live compiler returned
`action_class=external_lookup`, `scenario_hint=weather_query`, and
`slots.location.raw=Barcelona`. The Python integration lane added 1 pass and 0
skips.

### Production-Owner Root Cause After Falsification

**Claim Source:** interpreted from the executed provider/canary/weather results
and read-only production-source inspection.

The parent hypothesis was only partially correct. Adding the canonical
`{"location":{"raw":"Barcelona"}}` slot fixes the provider output, and the live
compiler canary proves that output survives the real provider -> ML sidecar ->
Go compiler path. The facade still does not execute the resolved weather turn:

1. `compiledLocationRaw` is consumed only by
   `proposeCompilerDisambiguation`, which handles the ambiguous clarification
   branch.
2. A normal `external_lookup` with a resolved location falls through to the
   generic scenario executor; no production path directly uses the compiled
   location slot to invoke the existing location/weather resolvers.
3. No weather `SourceAssembler` is registered. A non-empty weather response
   with an empty `Sources` slice reaches `provenance.Enforce`, which rewrites it
   to `StatusSavedAsIdea` plus `CaptureRoute=true`; the facade then normalizes
   that response to the exact observed capture acknowledgement.

The minimal product repair belongs to `bubbles.implement`: route a resolved,
compiled `weather_query` through the existing read-only location/weather
capabilities and construct the sourced response just as the selected-location
resume path already does, with focused production tests. This continuation did
not edit production source.

### Static Quality Checks

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Commands (home path normalized):**

- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh check`
- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh lint`
- `cd ~/smackerel-bug-spec069-deterministic-e2e-20260720 && SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh format --check`

**Exit Codes:** 0, 0, 0
**Output:**

```text
config-validate: ~/smackerel-bug-spec069-deterministic-e2e-20260720/config/generated/dev.env.tmp.<pid> OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
OK: PWA manifest has required fields
OK: Chrome extension manifest has required fields (MV3)
OK: Firefox extension manifest has required fields (MV2 + gecko)
Web validation passed
78 files already formatted
```

**Result:** PASS for check, lint, and format-check. These static passes do not
override the red required weather E2E.

### Validation Sequence Stop And Suite Accounting

- Focused provider unit: 1 passed, 0 failed, 0 skipped, 720 deselected.
- Exact protected weather source guard: 1 passed, 0 failed, 0 skipped.
- Exact weather behavior E2E: 0 passed, 1 failed, 0 skipped.
- Exact integration canaries: 2 passed, 0 failed, 0 skipped; Python integration
  lane 1 passed.
- Exact protected five: not rerun in this continuation because the preceding
  required weather gate failed. The pre-continuation evidence remains 5 passed,
  0 failed, 0 skipped and is not presented as a continuation rerun.
- Full assistant package: not rerun in this continuation because the exact
  protected weather failure would make the package red. The pre-continuation
  evidence remains 48 passed, 0 failed, 5 skipped and is not presented as a
  continuation rerun.
- Canonical all-package E2E was not rerun, as explicitly required.
- Packet guards and transition assertion are recorded after the final additive
  packet edit; no regression/security/audit/validate phase is claimed.

### Final Test Resource Cleanup

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Command:** repo-CLI `--env test down --volumes`, followed by direct
name-filtered Docker/process verification using Bash arrays and no output
pipelines.
**Exit Code:** 0
**Output:**

```text
FINAL TEST RESOURCE VERIFICATION
compose_project=smackerel-test
containers=<none>
networks=<none>
volumes=<none>
processes=<none>
container_count=0
network_count=0
volume_count=0
process_count=0
verdict=PASS
```

**Result:** PASS - zero residual test containers, networks, volumes, or test
processes.

### Continuation DoD And Ownership Delta

- No new DoD checkbox is checked by this continuation. TP-BUG069005-03 was
  already checked; its stale follow-on finding is corrected by the current
  2-pass canary evidence above.
- TP-BUG069005-13, scenario-specific E2E completion, broader E2E completion,
  Scope 1/2 Build Quality, and validate-owned certification remain unchecked.
- TP-BUG069005-11 remains unchecked because the separately owned upstream
  generic Bubbles Go skip-family guard is still open; the new product-owned AST
  assertion proves only this protected weather test.
- Scope and certification status remain `in_progress`. No state or
  certification artifact was edited.
- No commit or push was performed because the user-authorized close-out
  condition required weather, canaries, protected five, and the assistant
  package all to be green.

### Final Packet And Transition Assertions

**Executed:** YES (current continuation)
**Phase:** test
**Claim Source:** executed
**Commands (home paths normalized):** packet artifact lint; traceability guard;
implementation reality scan; installed regression-quality guard in normal and
`--bugfix` modes over the protected five plus weather file; assertion-only
state-transition guard without `--revert-on-fail`.
**Exit Codes:** 0, 0, 0, 0, 0, 1
**Output:**

```text
Artifact lint PASSED.
Scenarios checked: 6
Test rows checked: 15
Scenario-to-row mappings: 6
DoD fidelity scenarios: 6 (mapped: 6, unmapped: 0)
RESULT: PASSED (0 warnings)
Files scanned:  34
Violations:     0
Warnings:       0
PASSED: No source code reality violations detected
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 6
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 6
Files with adversarial signals: 6
DoD items total: 25 (checked: 18, unchecked: 7)
PASS: No DoD format manipulation detected — all DoD items use checkbox format
TRANSITION BLOCKED: 25 failure(s), 3 warning(s)
failedGateIds: [G056,G060,G061,G022,G053,G090]
failedChecks: [Check-4-completion,Check-5-all-done]
blockingCode: DELIVERY_COMPLETION_FAILED
failureCount: 25
exitStatus: 1
verdict: FAIL
```

**Result:** Packet artifact, traceability, implementation reality, and both
product regression guards pass. The transition assertion correctly blocks
promotion and made no state mutation. Its remaining failures include the seven
unchecked DoD items, no Done scopes, missing validate-owned certification and
phase records, the open foreign framework route, planning/evidence contract
gaps, and retro convergence metadata. This test continuation claims none of
those owners' closures.

## Weather Implementation And Test Closeout

**Executed:** YES (current continuation)
**Starting revision:** `1493aa0100de8dbb19599857fdf83e08a3e8f1df`
**Claim Source:** executed for commands and observed output; interpreted only
for ownership and release routing.

### Implemented Behavior

- Resolved compiled `weather_query` intents now consume the structured
  `slots.location.raw` value through the existing location resolver.
- Canonical resolved locations feed the existing weather resolver; ambiguous
  locations continue through the durable disambiguation path and never perform
  weather lookup before selection.
- Successful weather responses require a non-empty forecast, provider name,
  retrieval time, and external-provider source reference before provenance
  enforcement.
- Missing capabilities, provider failures, malformed resolved locations, and
  unsourced forecasts fail through the existing provenance/capture policy.
- The generic LLM executor is not invoked for handled compiled weather turns.

### Focused Unit Evidence

**Commands:**

- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --go --go-run '^TestFacadeResolvedCompiledWeather'`
- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test unit --python --python-k 'barcelona_weather_fixture'`

**Exit Codes:** 0, 0

```text
[go-unit] applying -run selector: ^TestFacadeResolvedCompiledWeather
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/internal/assistant       0.152s
[go-unit] go test ./... finished OK
[py-unit] pip install OK; starting unit-only pytest ml/tests
.                                                                        [100%]
1 passed, 720 deselected in 0.82s
[py-unit] pytest ml/tests finished OK
unit verdict=PASS
required skips=0
```

### Live Integration And Protected Weather Evidence

**Commands:**

- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test integration --go-run '^(TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler|TestCompilerInteractiveControlsPersistByUserAndTransport)$'`
- `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation$'`

**Exit Codes:** 0, 0

```text
=== RUN   TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
--- PASS: TestIntentCompilerCanary_LiveCoreConstructsAndAttachesCompiler
=== RUN   TestCompilerInteractiveControlsPersistByUserAndTransport
--- PASS: TestCompilerInteractiveControlsPersistByUserAndTransport
PASS: go-integration
[py-integration] live integration pytest finished OK
PASS: python-integration
=== RUN   TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation
--- PASS: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.097s
PASS: go-e2e
```

### Full Assistant Package Evidence

**Command:** `SMACKEREL_HARDWARE_TIER=cpu ./smackerel.sh test e2e --go-package assistant`
**Exit Code:** 0

```text
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
--- PASS: TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup
--- PASS: TestIntentCompilerE2E_WeatherCompilesBeforeRouteAndNormalizesLocation
--- PASS: TestIntentCompilerWeatherProtectedTestHasNoSkipFamilyCall
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
--- PASS: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects
--- PASS: TestAssistantTransportHintParity_WebAndMobileShareResponseShape
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      102.493s
PASS: go-e2e
```

Known Telegram-webhook, real-LLM microtool, planned capture-policy, and opt-in
Ollama profile omissions remained explicit in runner output and are not claimed
as passed by this packet.

### Static And Governance Evidence

**Commands:** repo `check`, `lint`, `format --check`; packet artifact lint,
traceability, implementation-reality, changed-test regression guard, and exact
protected-five regression guard.
**Exit Codes:** all 0

```text
Config is in sync with SST
env_file drift guard: OK
scenarios registered: 17, rejected: 0
scenario-lint: OK
All checks passed!
Web validation passed
78 files already formatted
Artifact lint PASSED.
RESULT: PASSED (0 warnings)
Violations: 0
Warnings: 0
REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
Files scanned: 5
Files with adversarial signals: 5
```

### Isolation And Remaining Route

The integration and E2E invocations used the disposable `smackerel-test`
project and their own teardown logs removed the test containers, volumes, and
network. A separate E2E invocation later started from the main checkout and is
not part of this packet or its cleanup claim.

Scopes 1 and 2 are implementation-complete. Scope 3 has current evidence for
the protected tests and full assistant package, but canonical all-package E2E
must run on the eventual merged-main candidate. Certification remains
`in_progress`; security, audit, and validate ownership are not claimed here.

## Implementation Delta

### Code Diff Evidence

**Phase:** plan (linkage repair)
**Recorded:** 2026-08-23 by bubbles.plan
**Claim Source:** executed — every sha, path, and count below was read from
`git` in this session. Nothing here is a re-run of the test suite; this section
records WHAT CHANGED, not what passed.

**Changed paths — machine-readable delta index**
**Recorded:** 2026-08-23 by bubbles.implement
**Claim Source:** executed — the 37 delivered paths were read from
`git diff --name-only f8e51b5e^1 f8e51b5e` (non-spec paths only) in this
session; the guard path was read from `git status --porcelain`.

This list is deliberately placed in the section BODY, above the first `####`
sub-heading. The delta extractor stops at the next heading of ANY level, so a
path recorded under a sub-heading is invisible to it. The list is also the only
DURABLE evidence source: the working-tree signal that currently satisfies the
delta check comes from `git status` and `git diff HEAD`, both of which go empty
the moment this packet is committed. Prose below the sub-headings explains these
same paths; this block is what a machine can read.

```text
cmd/core/main.go
cmd/core/wiring_assistant_actions.go
cmd/core/wiring_assistant_facade.go
config/smackerel.yaml
docker-compose.yml
internal/agent/tools/microtools/location_normalize.go
internal/agent/tools/microtools/location_normalize_openmeteo.go
internal/agent/tools/microtools/location_normalize_openmeteo_test.go
internal/agent/tools/weather/tool.go
internal/assistant/compiled_interactions.go
internal/assistant/compiled_weather_test.go
internal/assistant/context/store.go
internal/assistant/facade.go
internal/assistant/intent/http_transport.go
internal/assistant/intent/http_transport_test.go
internal/config/assistant.go
internal/config/assistant_intent_compiler.go
internal/config/assistant_intent_compiler_test.go
internal/config/assistant_test.go
internal/config/validate_test.go
ml/app/main.py
ml/app/routes/intent_compile.py
ml/tests/conftest.py
ml/tests/test_intent_compiler.py
ml/tests/test_intent_compiler_provider_fixture.py
ml/tests/test_main.py
scripts/commands/config.sh
tests/e2e/assistant/annotation_intent_test.go
tests/e2e/assistant/http_confirm_test.go
tests/e2e/assistant/http_disambiguation_test.go
tests/e2e/assistant/intent_clarify_test.go
tests/e2e/assistant/intent_compiler_http_test.go
tests/e2e/assistant/intent_side_effect_test.go
tests/e2e/assistant/required_compiler_state_helpers_test.go
tests/e2e/intent-compiler-provider/provider.py
tests/e2e/stub-providers/nginx.conf
tests/integration/assistant/bug069005_runtime_canary_test.go
tests/e2e/assistant/required_no_skip_guard_test.go
```

The final entry, `tests/e2e/assistant/required_no_skip_guard_test.go`, is new in
this session and is described under
[Required-Test No-Skip Guard](#required-test-no-skip-guard-2026-08-23).

#### Locating the commit of record

`git log --oneline --all --grep='BUG-069-005'` returns FOUR commits, in two
subject-identical pairs. That matters, because a reader who cites by subject can
easily cite a sha that is not in the shipped history. `git merge-base
--is-ancestor` was run against each to settle it:

```text
$ git log --oneline --all --grep='BUG-069-005'
92865443 merge(consolidate): bug/spec069 BUG-069-005 assistant intent-compiler fixes (in_progress — dedicated to late completion)
f8e51b5e merge(consolidate): bug/spec069 BUG-069-005 assistant intent-compiler fixes (in_progress — dedicated to late completion)
aeb1b966 plan: reconcile BUG-069-005 implementation paths
eeccb00a plan: reconcile BUG-069-005 implementation paths

$ for c in 92865443 f8e51b5e aeb1b966 eeccb00a; do git merge-base --is-ancestor $c HEAD && echo "$c ANCESTOR-OF-HEAD" || echo "$c not-ancestor"; done
92865443 not-ancestor
f8e51b5e ANCESTOR-OF-HEAD
aeb1b966 not-ancestor
eeccb00a ANCESTOR-OF-HEAD
```

The commit of record for the delivered tree is therefore `f8e51b5e`, and the
planning reconciliation that followed it is `eeccb00a`. `92865443` and
`aeb1b966` carry the same subjects but are NOT reachable from `HEAD`; they are
pre-rebase copies and must not be cited as the delivery.

```text
$ git show -s --format='%H%n%cI%n%s' f8e51b5e
f8e51b5ed237dcfa44ee7951fbfa38c272ddf934
2026-07-27T06:24:05+00:00
merge(consolidate): bug/spec069 BUG-069-005 assistant intent-compiler fixes (in_progress — dedicated to late completion)

$ git show -s --format='%H%n%cI%n%s' eeccb00a
eeccb00a448bbd15d8e03dddf0504012c3a55913
2026-07-21T01:22:39+00:00
plan: reconcile BUG-069-005 implementation paths
```

#### Delta size, split by artifact vs source

```text
$ git show --shortstat --format='' f8e51b5e
 45 files changed, 6036 insertions(+), 59 deletions(-)

$ git show --numstat --format='' f8e51b5e | awk -F'\t' 'NF==3 { if ($3 ~ /BUG-069-005-required-e2e-false-green/) { pf++; pa+=$1; pd+=$2 } else { sf++; sa+=$1; sd+=$2 } } END { printf "packet-artifact files=%d +%d -%d\nsource files=%d +%d -%d\n", pf, pa, pd, sf, sa, sd }'
packet-artifact files=8 +3304 -0
source files=37 +2732 -59
```

The split is recorded because the raw `45 files / 6036 insertions` headline
overstates the code delivery by more than half: 8 of those files and 3304 of
those insertions are this packet's own markdown and JSON artifacts. The runtime
delivery is **37 files, +2732 / -59**.

`eeccb00a` touches exactly one file and no source at all:

```text
$ git show --stat --format='' eeccb00a
 .../BUG-069-005-required-e2e-false-green/scopes.md | 149 ++++++++++++++++-----
 1 file changed, 115 insertions(+), 34 deletions(-)
```

#### Non-artifact runtime, source, and config paths changed by `f8e51b5e`

```text
$ git show --stat --format='' f8e51b5e
 cmd/core/main.go                                   |   10 +-
 cmd/core/wiring_assistant_actions.go               |  163 +++
 cmd/core/wiring_assistant_facade.go                |   49 +
 config/smackerel.yaml                              |   16 +-
 docker-compose.yml                                 |   37 +
 .../agent/tools/microtools/location_normalize.go   |   21 +
 .../microtools/location_normalize_openmeteo.go     |   12 +-
 .../location_normalize_openmeteo_test.go           |   35 +
 internal/agent/tools/weather/tool.go               |   38 +
 internal/assistant/compiled_interactions.go        |  534 ++++++++
 internal/assistant/compiled_weather_test.go        |  292 ++++
 internal/assistant/context/store.go                |   15 +-
 internal/assistant/facade.go                       |   58 +
 internal/assistant/intent/http_transport.go        |  117 ++
 internal/assistant/intent/http_transport_test.go   |  100 ++
 internal/config/assistant.go                       |   13 +-
 internal/config/assistant_intent_compiler.go       |   14 +-
 internal/config/assistant_intent_compiler_test.go  |   43 +
 internal/config/assistant_test.go                  |   35 +-
 internal/config/validate_test.go                   |    4 +-
 ml/app/main.py                                     |   31 +
 ml/app/routes/intent_compile.py                    |  175 +++
 ml/tests/conftest.py                               |   11 +
 ml/tests/test_intent_compiler.py                   |  122 ++
 ml/tests/test_intent_compiler_provider_fixture.py  |   42 +
 ml/tests/test_main.py                              |   20 +
 scripts/commands/config.sh                         |    8 +-
 tests/e2e/assistant/annotation_intent_test.go      |   44 +-
 tests/e2e/assistant/http_confirm_test.go           |   30 +-
 tests/e2e/assistant/http_disambiguation_test.go    |   20 +-
 tests/e2e/assistant/intent_clarify_test.go         |   11 +-
 tests/e2e/assistant/intent_compiler_http_test.go   |   77 +-
 tests/e2e/assistant/intent_side_effect_test.go     |   11 +-
 .../required_compiler_state_helpers_test.go        |  184 +++
 tests/e2e/intent-compiler-provider/provider.py     |  122 ++
 tests/e2e/stub-providers/nginx.conf                |    3 +
 .../assistant/bug069005_runtime_canary_test.go     |  274 ++++
```

The paths abbreviated by `git`'s own `.../` prefix elision resolve to
`internal/agent/tools/microtools/location_normalize.go`,
`internal/agent/tools/microtools/location_normalize_openmeteo.go`,
`internal/agent/tools/microtools/location_normalize_openmeteo_test.go`,
`tests/e2e/assistant/required_compiler_state_helpers_test.go`, and
`tests/integration/assistant/bug069005_runtime_canary_test.go`.

#### The five protected E2E files, individually

This packet exists because five manifest-required tests reported success while
asserting nothing. The per-file delta on exactly those five is therefore the
most load-bearing part of the diff, and is recorded separately rather than left
inside a 37-file list:

```text
$ git show --numstat --format='' f8e51b5e | grep -E 'tests/e2e/assistant/(annotation_intent|http_confirm|http_disambiguation|intent_clarify|intent_side_effect)_test\.go'
36      8       tests/e2e/assistant/annotation_intent_test.go
22      8       tests/e2e/assistant/http_confirm_test.go
15      5       tests/e2e/assistant/http_disambiguation_test.go
9       2       tests/e2e/assistant/intent_clarify_test.go
9       2       tests/e2e/assistant/intent_side_effect_test.go
```

Read as insertions/deletions: every one of the five was both ADDED to and
DELETED from. A file that only gained lines would be consistent with adding an
assertion beside a surviving bailout; the deletions are what show the
skip-family branches were removed rather than supplemented.

#### Tree state at the time of this recording

```text
$ git log -1 --oneline
c8ec870b (HEAD -> main, origin/main) validate(BUG-104-001): certify 6 of 8 phases; terminal status blocked

$ git status --porcelain
(no output)
```

The working tree was clean before this session's artifact writes, so the counts
above describe committed history and not an uncommitted local state. All paths
are repository-relative from `<repo-root>`.

## Regression Evidence (BUG-069-005)

Executed by `bubbles.regression` on 2026-08-23 (run window 01:59:29Z to
02:14:33Z UTC, 904s measured). This packet is titled *required-e2e-false-green*,
so the regression question is the packet's own subject: **do the tests covering
this fix actually fail when the fix is removed?** A test that cannot fail is the
defect this packet exists to remove, so a passing suite proves nothing on its
own.

Scope note: this phase is diagnostic. No source file, no DoD checkbox, no
certification field, and no `uservalidation.md` byte was modified.

### Mutation Check — The Load-Bearing Evidence

One mutation, chosen because it targets the packet's own named root cause: a
**compiler-disabled test config**. The fix that closes it is the
`F069-INTENT-COMPILER-DISABLED` coherence rule in
`internal/config/assistant.go`, which makes the false-green configuration
refuse to validate. Its guard is `TestIntentCompilerRequiredWhenAssistantHTTPEnabled`
in `internal/config/assistant_intent_compiler_test.go` — a pure unit test with
no build tag and no live-stack dependency, so the check is deterministic.

**Claim Source:** executed in this session via the repo CLI.

Baseline at clean HEAD (`7a0f2b2e`), working tree verified empty first:

```text
$ ./smackerel.sh test unit --go --go-run 'TestIntentCompilerRequiredWhenAssistantHTTPEnabled'
ok      github.com/smackerel/smackerel/internal/config  0.091s
[go-unit] go test ./... finished OK
=== BASELINE_EXIT=0 WALL_SECONDS=96 ===
```

The `0.091s` timing matters: `internal/config` reports an elapsed duration
rather than `[no tests to run]`, which is how a focused selector that matched
nothing would have presented. The selector really bound to a test.

Mutation applied — the coherence condition short-circuited so the rule can
never fire:

```text
$ git status --porcelain
 M internal/config/assistant.go

$ git diff -U1 internal/config/assistant.go
@@ -555,3 +555,3 @@ func (c *Config) validateAssistantConfig() error {
                c.Assistant.IntentCompiler.ProviderName != "" || ...ProviderURL != ""
-       if compilerConfigLoaded && !c.Assistant.IntentCompiler.Enabled {
+       if false && compilerConfigLoaded && !c.Assistant.IntentCompiler.Enabled {
                return fmt.Errorf("[F069-INTENT-COMPILER-DISABLED] enabled assistant transport requires ...")
```

Mutated run — the test **died**, naming the exact missing behaviour:

```text
--- FAIL: TestIntentCompilerRequiredWhenAssistantHTTPEnabled (0.00s)
    assistant_intent_compiler_test.go:37: enabled transport with compiler disabled error = <nil>, want F069-INTENT-COMPILER-DISABLED
FAIL
FAIL        github.com/smackerel/smackerel/internal/config  0.031s
=== MUTATED_PIPELINE_EXIT=1 ===
```

Restoration was performed as its own explicit step, then verified — not folded
into a shell trap, because the agent terminal is a persistent shell where an
`EXIT` trap does not fire on command completion and would have left the
mutation resident in the tree:

```text
$ git checkout -- internal/config/assistant.go
$ git status --porcelain
--- end porcelain (empty above = clean) ---
$ grep -n 'if compilerConfigLoaded && !c.Assistant.IntentCompiler.Enabled' internal/config/assistant.go
556:    if compilerConfigLoaded && !c.Assistant.IntentCompiler.Enabled {

$ ./smackerel.sh test unit --go --go-run 'TestIntentCompilerRequiredWhenAssistantHTTPEnabled'
POST_RESTORE_EXIT=0 WALL_SECONDS=64
ok          github.com/smackerel/smackerel/internal/config  0.117s
[go-unit] go test ./... finished OK
```

**Result: PASS to FAIL to PASS.** The guard binds its claim. Removing the fix
kills the test, which is the property the original false-green lacked.

### Skip-Family Scan Of The Five Required Tests

**Claim Source:** executed static scan in this session.

```text
$ grep -nE 't\.Skip|t\.Skipf|t\.SkipNow|testing\.Short' <each required file>
--- tests/e2e/assistant/annotation_intent_test.go ---     (no skip-family call)
--- tests/e2e/assistant/intent_clarify_test.go ---        (no skip-family call)
--- tests/e2e/assistant/http_disambiguation_test.go ---   (no skip-family call)
--- tests/e2e/assistant/intent_side_effect_test.go ---    (no skip-family call)
--- tests/e2e/assistant/http_confirm_test.go ---          (no skip-family call)
```

All five are free of the five original `t.Skipf` bailouts. Two bare `return`
statements do remain in that directory, at `intent_clarify_test.go:99` and
`intent_side_effect_test.go:132`, and both were read in full rather than
pattern-matched. Neither is a silent pass and neither belongs to a required
test: both sit in *negative-invariant* tests
(`TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup`,
`TestIntentCompilerE2E_WriteAndStateMutationNeverBypassConfirmGate`) and exit on
the antecedent being false, where the conditional invariant holds trivially.
That is sound, and distinct from returning on a failure condition.

The required list-write test now carries hard assertions where it previously
skipped, including a database round-trip proving non-persistence before
confirmation (`listCountBySourceQuery(...) != 0` is fatal).

### Honest Negative Finding — The Guard Does Not Look

`regression-quality-guard.sh` returns `0 violation(s), 0 warning(s)` on all five
required files. That result is **not** evidence of skip-freeness, and reporting
it as such would repeat this packet's own mistake in a new place. An adversarial
probe was run against a throwaway file outside the repository tree containing
nothing but two skip bailouts:

**Claim Source:** executed adversarial probe in this session; probe deleted, repository tree unaffected.

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh <temp>/adversarial_skip_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
exit=0

$ grep -cE 't\.Skip|SkipNow|Skipf' .github/bubbles/scripts/regression-quality-guard.sh
0
```

The guard passes a file that is *entirely* skip bailouts, and its source
contains zero skip-family tokens. Its clean verdict on the required tests is a
blind pass. Skip-freeness above was established independently by direct scan,
not by that guard.

The in-repository `tests/e2e/agent/no_skip_guard_test.go` does enforce a no-skip
contract, but it resolves its scan directory from its own file location and does
not recurse, so it covers `tests/e2e/agent/` only and never inspects
`tests/e2e/assistant/` — the directory this packet is about.

Net: the five required assistant tests are skip-free **by content today** and
**mechanically unguarded** against reintroduction. That is a durability gap, not
a live regression, and it substantiates the already-open
`TR-BUG-069-005-FRAMEWORK-001` rather than opening a new route.

### Cross-Spec Observation (read-only)

Establishable read-only: the file header and function doc comment in
`tests/e2e/assistant/intent_side_effect_test.go` still describe the removed
behaviour, stating that the strict assertion "is skipped to keep the test stable
under LLM nondeterminism". The code now calls `t.Fatalf` instead. The prose
contradicts the executable contract and describes exactly the false-green
posture this packet removed, so a future reader could reinstate the skip
believing it sanctioned. Documentation drift only; no behaviour is affected.

### Verdict

**REGRESSION_DETECTED — none in the code under test; one durability gap recorded.**

| Check | Result |
|---|---|
| Mutation binds the fix | PASS to FAIL to PASS, proven |
| Skip-family bailouts in required tests | none |
| Silent-pass early returns in required tests | none |
| Bailout guard actually detects Go skips | NO — blind pass, evidenced |
| No-skip guard covers the assistant directory | NO — scoped to the agent directory |
| Source tree modified on exit | none; porcelain empty |

#### Follow-Up Narrative

- `followUpOwner`: `bubbles.implement` in the framework repository, under the existing open `TR-BUG-069-005-FRAMEWORK-001`.
- `followUpAction`: extend the bailout guard so a Go skip-family call in a manifest-required test is rejected, with a hermetic adversarial selftest proving the guard can fail; the probe above is a ready reproduction.
- `followUpTarget`: the framework-owned guard script; this session made no edit under `.github/bubbles/`, which another session owns.

- `followUpOwner`: `bubbles.docs`.
- `followUpAction`: reconcile the stale skip narrative in the `intent_side_effect_test.go` header and function comment with the fail-closed code.
- `followUpTarget`: `tests/e2e/assistant/intent_side_effect_test.go` comment blocks only.

### Test Phase — Required E2E Execution (2026-08-23)

**Phase:** test
**Agent:** bubbles.test
**Executed:** YES (current session)
**Claim Source:** executed

Three commands were run, in this order. All exit codes are the real observed values.

#### Command 1 — the five manifest-required tests, named explicitly

**Command:** `./smackerel.sh test e2e --go-package assistant --go-run 'TestAnnotationIntentE2E_SlotsComeFromCompiledIntent|TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation|TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn|TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence|TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce'`
**Exit Code:** 0
**Accounting:** 5 top-level PASS, 0 FAIL, **0 SKIP**

```text
go-e2e: applying package selector: assistant
=== RUN   TestAnnotationIntentE2E_SlotsComeFromCompiledIntent
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.16s)
=== RUN   TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.09s)
=== RUN   TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.07s)
=== RUN   TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.06s)
=== RUN   TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.06s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      0.485s
PASS: go-e2e
REQUIRED_E2E_RC=0
```

Each of the five carries its own `--- PASS:` line. That is the load-bearing check for this
packet: an `ok` line alone would not distinguish a real pass from a selector that matched
nothing, and package exit 0 is exactly what the original false green produced while all five
skipped. Zero `--- SKIP:` lines were emitted.

#### Command 2 — the unfiltered assistant package, including the no-skip guards

**Command:** `./smackerel.sh test e2e --go-package assistant`
**Exit Code:** 0
**Accounting:** 52 top-level PASS, 0 FAIL, 12 SKIP

```text
--- PASS: TestAnnotationIntentE2E_SlotsComeFromCompiledIntent (0.16s)
--- PASS: TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation (0.06s)
--- PASS: TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn (0.07s)
--- PASS: TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence (0.06s)
--- PASS: TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce (0.10s)
--- PASS: TestRequiredAssistantE2ETestsNeverSkip (0.00s)
--- PASS: TestRequiredNoSkipGuard_AdversarialFinding (0.01s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e/assistant      20.372s
PASS: go-e2e
FULL_ASSISTANT_E2E_RC=0
```

The two guard tests that protect the required set both pass, including the adversarial guard
with all seven of its subtests.

**The 12 skips, named rather than summarised.** None is in the manifest-required set, and the
`TestRequiredAssistantE2ETestsNeverSkip` guard passing is the mechanical statement that the
required set contains no skip-family call:

```text
--- SKIP: TestAssistantE2E_CaptureAcknowledgementIsCrossTransportIdentical_TP_074_xx
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackDedupWithinWindow_TP_074_11
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackIsInviolable_TP_074_04
--- SKIP: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround
--- SKIP: TestAssistantHTTPE2E_CaptureProvenanceIsDistinct_TP_074_07
--- SKIP: TestAssistantHTTPE2E_CaptureRouteInvokesCaptureOnceAndAcknowledges
--- SKIP: TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape
--- SKIP: TestIntentCompilerE2E_MalformedJSONBlocksRoutingAndCaptures
--- SKIP: TestLegacyRetirementE2E_AliasWindowRoutesPlainEnglishWithNotice
--- SKIP: TestLegacyRetirementE2E_ExpiredSlashCommandDoesNotInvokeScenario
--- SKIP: TestMicroToolsE2E_ConvertsThreeCupsFlourToGrams
--- SKIP: TestMicroToolsE2E_CalculatorRejectsUnsafeExpression
```

Their recorded reasons are environment gating, not assertion weakening — the two
`LegacyRetirementE2E` skips report `TELEGRAM_WEBHOOK_PATH / TELEGRAM_WEBHOOK_SECRET not set`,
and the two `MicroToolsE2E` skips report the live LLM answering `the service is unavailable
right now`. These belong to Spec 072/074/075 surfaces, not to this packet's required five.

#### Command 3 — the canonical repository-wide suite

**Command:** `./smackerel.sh test e2e`
**Exit Code:** 0
**Accounting:** zero `FAIL` and zero `--- FAIL:` lines in the retained output

```text
ok      github.com/smackerel/smackerel/tests/e2e        0.577s
PASS: go-e2e-graph-disabled
PASS: go-e2e-corpus-enforce
REPO_WIDE_E2E_RC=0
```

This is the run the three `Broader E2E regression suite passes.` items name by command. It was
executed on `main` at `673bb6a0`, measured `0` commits behind `origin/main`, so the
"merged with current main" precondition attached to the Scope 3 wording holds.

#### Two honest qualifications on the evidence above

**First — the counts in Command 2 were corrected mid-session, and the first reading was wrong.**
The initial capture of that run was truncated by the harness, and counting `--- PASS:` lines in
the truncated file yielded `35 PASS / 5 SKIP` with four of the five required tests appearing
absent. That reading was discarded once the truncation marker was found; the numbers recorded
above come from the complete untruncated output. The apparent absence was a capture artifact,
not a test result. It is recorded here because a truncated capture that still ends in exit 0 is
the same class of trap this packet documents.

**Second — Command 3's retained output is the tail of the run, not the whole run.** The capture
buffer reports `Output exceeded terminal scrollback; beginning of output was lost`, so the main
`go-e2e` phase scrolled out and its per-test lines cannot be quoted from this session. What is
claimed for Command 3 is therefore exactly what was observed: the canonical command completed
with exit code 0 and no failure line in the retained portion. Per-test enumeration of the
assistant package is not inferred from Command 3 — it is taken from Command 2, which was run
separately for that purpose.

The `[no tests to run]` lines visible against most packages in Command 3's tail are the two
deliberately filtered phases (`-run TestE2E_GraphActivation_Disabled` and
`-run TestE2E_Spec108_CorpusEnforce`); non-matching packages correctly report no match. That is
selector behaviour, not an empty suite.

#### Verdict

| Required test | Verdict | Own `--- PASS:` line |
|---|---|---|
| `TestAnnotationIntentE2E_SlotsComeFromCompiledIntent` | PASS | yes |
| `TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation` | PASS | yes |
| `TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn` | PASS | yes |
| `TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence` | PASS | yes |
| `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` | PASS | yes |

5 required executed; 5 passed; 0 failed; 0 skipped; package exit 0.


