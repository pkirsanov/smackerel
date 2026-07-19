# Scopes: BUG-074-001 Canonical capture response

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Canonicalize successful no-ground capture

**Status:** In Progress
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

- [ ] Root cause confirmed with both RED responses.
- [ ] Successful capture returns canonical normal fields.
- [ ] Failed capture remains unavailable and explicit.
- [ ] Adversarial unit catches stale error/body/control fields.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Change boundary and quality guards pass.
- [ ] Validate-owned certification remains authoritative.
