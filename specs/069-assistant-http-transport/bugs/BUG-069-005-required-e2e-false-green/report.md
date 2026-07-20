# Report: BUG-069-005 Required assistant E2E false-green

## Summary

One consolidated bug packet documents a semantic RED hidden by a process-level
green result. The five exact Spec 069 manifest-required tests executed on
revision `37ed541524fe0ed61747cac929c11143b146657e`; all five skipped when
required compiler/interaction behavior was missing, so zero required behavior
passed while the package exited 0.

Static analysis in this session confirms compiler-disabled test config, absent
production compiler attachment, absent live ML compiler route, response/state
mismatches, and missing Go skip-family coverage in the generic regression
quality guard.

**Claim Source:** interpreted from authoritative artifacts, executed static
checks, and accepted `bubbles.test` handoff.

## Completion Statement

Bug ownership, authority reconciliation, root-cause classification, repair
design, scenario-first scopes, test plan, mechanical boundaries, and routing
are complete. No product code, test, config, parent artifact, framework-managed
file, deployment surface, or release-train file was modified. The bug and its
certification remain `in_progress`; no fix or post-fix test result is claimed.

**Claim Source:** executed for packet authoring; not-run for implementation and
post-fix verification.

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

No post-fix evidence exists in this packet-authoring phase.

**Claim Source:** not-run.

## Planned Test Traceability

The following concrete paths are the scenario-first test locations declared in
`scopes.md`. They are listed here so the packet has an explicit
scenario-to-test-to-evidence chain without pretending they have run after a
fix:

- `internal/config/assistant_intent_compiler_test.go` - not-run for this bug.
- `ml/tests/test_intent_compiler.py` - planned new provider-contract test;
  not present or run in this phase.
- `tests/integration/assistant/intent_compiler_canary_test.go` - not-run for
  this bug.
- `tests/e2e/assistant/annotation_intent_test.go` - accepted pre-fix skip only;
  post-fix not-run.
- `tests/e2e/assistant/intent_clarify_test.go` - accepted pre-fix skip only;
  post-fix not-run.
- `tests/integration/assistant/http_pending_state_test.go` - not-run for this
  bug.
- `tests/e2e/assistant/http_disambiguation_test.go` - accepted pre-fix skip
  only; post-fix not-run.
- `tests/e2e/assistant/intent_side_effect_test.go` - accepted pre-fix skip only;
  post-fix not-run.
- `tests/e2e/assistant/http_confirm_test.go` - accepted pre-fix skip only;
  post-fix not-run.
- `tests/e2e/assistant/http_live_stack_test.go` - not-run for this bug.
- Upstream `bubbles/scripts/regression-quality-guard-selftest.sh` - foreign
  framework test routed to the Bubbles owner; not-run in this product packet.

**Claim Source:** not-run. These references are planning links, not passing
evidence.

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

Broad product tests were intentionally not run in this packet-only phase.

## Invocation Audit

No subagents were invoked. The available tool surface did not expose a
`runSubagent` capability. This top-level `bubbles.bug` invocation performed
bug discovery, authority reconciliation, root-cause analysis, and packet
authoring only; implementation, test execution, and certification are routed to
their owning specialists.
