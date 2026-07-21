# Scopes: BUG-074-001 Canonical capture response

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Canonicalize successful no-ground capture

**Status:** Done
**Depends On:** none
**Owner:** `bubbles.implement`

### Gherkin Scenarios

```gherkin
Feature: Canonical successful capture response
  Scenario: Successful no-ground capture returns a normal acknowledgement
    Given open knowledge returns no grounded answer
    And fallback persistence succeeds
    When the facade returns the turn response
    Then status is saved as idea
    And capture route is true
    And error cause is empty
    And the body is the canonical acknowledgement

  Scenario: Capture persistence failure remains explicit
    Given fallback persistence fails
    When the facade returns the turn response
    Then status is unavailable with an error cause
```

### Implementation Plan

1. Add a pure canonical response helper and adversarial unit test.
2. Invoke it only after successful no-ground persistence.
3. Run focused live tests and full assistant package.

### Change Boundary

Allowed: `internal/assistant/facade.go`, focused facade tests, two existing E2E tests, and this packet.

### Implementation Files

- `internal/assistant/facade.go`
- `internal/assistant/facade_open_knowledge_no_ground_test.go`
- `internal/assistant/facade_high_band_test.go`
- `internal/assistant/facade_source_assembly_test.go`
- `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`
- `tests/e2e/assistant/http_capture_test.go`

### Test Plan

| Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|
| Canonicalization unit | `unit` | `internal/assistant/` | Clears stale error/control fields | `./smackerel.sh test unit --go --go-run 'CanonicalCapture'` | No |
| Capture persistence failure remains explicit | `unit` | `internal/assistant/facade_open_knowledge_no_ground_test.go` | `TestCanonicalizeSuccessfulCaptureResponse_LeavesExplicitFailureUnchanged` proves unavailable failure responses bypass successful-capture normalization | `./smackerel.sh test unit --go --go-run 'CanonicalizeSuccessfulCaptureResponse'` | No |
| Capture live regressions | `e2e-api` | `tests/e2e/assistant/` | No-ground and shared ACK shape | `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround|CaptureAcknowledgementMatchesTelegramShape'` | Yes |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Full package | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Neighboring assistant flows | `./smackerel.sh test e2e --go-package assistant` | Yes |

### Definition of Done

- [x] Root cause confirmed with both RED responses. → Evidence: [report.md](report.md) "Bug Reproduction — Before Fix" — current-session unit RED (`TestCanonicalizeSuccessfulCaptureResponse_ClearsUpstreamFailureShape` FAILs with `error_cause="provider_unavailable"` + refusal body `"I don't have a sourced answer for that."` surviving) plus the prior-session e2e RED (both responses: stale `body` and stale `error_cause`).
- [x] Successful capture returns canonical normal fields. → Evidence: [report.md](report.md) "Bug Reproduction — After Fix" (unit GREEN) + "Live E2E — capture scenario tests" (`status=saved_as_idea`, `capture_route=true`, `error_cause` empty, canonical "saved as an idea" body).
- [x] Failed capture remains unavailable and explicit. → Evidence: [report.md](report.md) "Test Evidence" — `TestCanonicalizeSuccessfulCaptureResponse_LeavesExplicitFailureUnchanged` PASS; the explicit unavailable failure is untouched and stayed GREEN even at RED (non-tautological).
- [x] Adversarial unit catches stale error/body/control fields. → Evidence: [report.md](report.md) RED→GREEN of `TestCanonicalizeSuccessfulCaptureResponse_ClearsUpstreamFailureShape` — it FAILs when the fix is neutralized (catches the stale `error_cause`/body/sources/confirm/disambig) and PASSes with the fix.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [report.md](report.md) "Live E2E — capture scenario tests" — `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` + `TestAssistantHTTPE2E_CaptureAcknowledgementMatchesTelegramShape` PASS on the live stack (`ok … 13.666s`, `E2E_CAPTURE_EXIT=0`).
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "Broader E2E regression — full assistant package" — full assistant package = 62 PASS; capture path + every neighboring assistant product flow GREEN. The only 2 failures are pre-existing, unrelated `buildvcs` (`go build … VCS status: exit 128`) environment failures in `intent_replay_test.go` — a different subsystem, outside this change boundary, not caused by this change (working tree is packet-only), owned by concurrent spec069 deterministic-e2e work (G051 class).
- [x] Change boundary and quality guards pass. → Evidence: [report.md](report.md) "Guards & Quality Gates" — change set limited to committed fix `8ac848e1` (`internal/assistant/facade.go` + `internal/assistant/facade_open_knowledge_no_ground_test.go`) plus this packet; `artifact-lint.sh` exit 0 and `state-transition-guard.sh` verdict PASS (`failedGateIds []`).
- [x] Validate-owned certification remains authoritative. → Evidence: [state.json](state.json) `certification.certifierAgent = bubbles.validate`, `certification.status = done`; the validate phase owns terminal certification (recorded in `execution.executionHistory`).
