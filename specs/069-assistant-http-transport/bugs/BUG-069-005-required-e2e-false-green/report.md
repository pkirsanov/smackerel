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

**Claim Source:** executed for implementation and verification; interpreted
only for ownership/routing conclusions.
**Interpretation:** The executed tests directly prove product behavior. The
remaining framework and planning-format findings are routed because their files
are outside `bubbles.implement` ownership.

## Completion Statement

Scopes 1 and 2 product implementation and implementation-adjacent tests are
delivered and verified. Required tests were strengthened without renaming;
public assistant v1 fields were unchanged. No parent spec, release train,
secret, deployment, knb, target, or framework-managed file was modified.

The packet and certification intentionally remain `in_progress`: the upstream
regression-quality guard enhancement, planning-owned concrete path
reconciliation, independent test/security/regression phases, and validate-owned
certification are not claimed by this phase.

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
