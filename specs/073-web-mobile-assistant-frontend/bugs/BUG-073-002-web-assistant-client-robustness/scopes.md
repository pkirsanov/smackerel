# Scopes: BUG-073-002

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

Single bugfix-fastlane scope. Delivered via `bubbles-workflow mode:
bubbles-fastlane` (parent-expanded — the active runtime lacks `runSubagent`).

## Scope 1 — Single-flight, time-bounded, busy-aware web assistant client

**Status:** Done
**Owner:** bubbles.workflow (parent-expanded bugfix-fastlane)

### Definition of Done

- [x] `inFlight` single-flight guard: `dispatchTurn` early-returns while a turn is running; the submit handler early-returns (without clearing the composer) when `inFlight`
      → Evidence: report.md `### Code Diff Evidence` + `## Test Evidence` (guard pattern `if\s*\(\s*inFlight\s*\)` matched >= 2)
- [x] `postTurn` builds an `AbortController`, passes `signal: controller.signal` to `fetch`, and aborts after `TURN_TIMEOUT_MS`, surfacing a retryable timeout error
      → Evidence: report.md `## Test Evidence` (AbortController + signal patterns matched)
- [x] `setComposerBusy` disables `assistant-send-btn` and toggles `aria-busy` around the in-flight turn
      → Evidence: report.md `## Test Evidence` (`.disabled = busy` matched)
- [x] Go source-contract guard `TestWebAssistantRobustnessGuard_BUG_073_002` asserts all three mechanisms are present; adversarial twin proves it detects their removal
      → Evidence: report.md `## Test Evidence` (both PASS; re-RED on a stripped sample)
- [x] `node --check web/pwa/assistant.js` clean; existing storage + codegen-drift guards still pass (no regression)
      → Evidence: report.md `### Validation Evidence`
- [x] `SCN-073-ROBUST-01..03` recorded in `scenario-manifest.json`
      → Evidence: `scenario-manifest.json`
- [x] Scenario-specific regression coverage for the fixed behavior — the source-contract guard persists the single-flight + timeout + busy invariants and fails if the wiring regresses (proven by the adversarial re-RED)
      → Evidence: report.md `## Test Evidence`
- [x] Broader regression suite passes — the full `web/pwa/tests` Go package runs green with the new guard included
      → Evidence: report.md `### Validation Evidence` (`ok ... web/pwa/tests`)

### Test Plan

| ID | Test | File | Type | Scenario |
|----|------|------|------|----------|
| T-073-ROBUST-01 | TestWebAssistantRobustnessGuard_BUG_073_002 | web/pwa/tests/assistant_robustness_guard_test.go | source-contract (regression) | SCN-073-ROBUST-01 |
| T-073-ROBUST-02 | TestWebAssistantRobustnessGuard_Adversarial_BUG_073_002 | web/pwa/tests/assistant_robustness_guard_test.go | adversarial | SCN-073-ROBUST-02 |
| T-073-ROBUST-03 | TestWebAssistantStorageGuard_TP_073_06 (preserved) | web/pwa/tests/assistant_storage_guard_test.go | regression (preserved) | SCN-073-ROBUST-03 |

### Non-Goals

- Mobile Dart client (audited clean).
- Browser interaction e2e (blocked by the auth-gated disposable stack).
