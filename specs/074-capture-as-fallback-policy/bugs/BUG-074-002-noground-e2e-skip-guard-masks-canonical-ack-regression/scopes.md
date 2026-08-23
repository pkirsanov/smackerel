# Scopes: BUG-074-002 No-ground E2E skip-guard masks the canonical-ack regression

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> **Filing state.** This packet was filed by `bubbles.bug` as a FILING +
> ROOT-CAUSE task. **No fix was implemented and none is claimed.** Every DoD item
> below is deliberately unchecked. The scope is routed to `bubbles.implement`;
> see `state.json` → `routing` and `transitionRequests`.

## Scope 1: Make the no-ground live E2E fail — not skip — on a canonical-ack regression

**Status:** Not Started
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope-Id:** SCOPE-BUG-074-002-01

### Change Boundary

Allowed: `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` and this
packet.

**Forbidden:** any change under `internal/`. This packet asserts no production
defect, so a production edit would be outside the evidence.

### Gherkin Scenarios

```gherkin
Feature: The no-ground capture E2E can fail on the regression it advertises

  Scenario: SCN-BUG-074-002-01 — An off-contract status FAILS the test
    Given a live open-knowledge no-ground turn reaches the facade
    And the returned envelope status is not "saved_as_idea"
    And the envelope carries no grounded sources
    When the regression test evaluates the envelope
    Then the test reports a failure
    And the test does not report a skip

  Scenario: SCN-BUG-074-002-02 — A genuinely grounded turn still passes
    Given a live open-knowledge turn that the model grounded
    And the envelope carries at least one source
    And capture route is false
    When the regression test evaluates the envelope
    Then the test passes without asserting the capture acknowledgement
    And the test does not report a skip

  Scenario: SCN-BUG-074-002-03 — The canonical acknowledgement is still enforced
    Given a live open-knowledge no-ground turn that was captured
    When the regression test evaluates the envelope
    Then capture route is true
    And confirm card is absent
    And disambiguation prompt is absent
    And the body carries the canonical saved-as-idea acknowledgement

  Scenario: SCN-BUG-074-002-04 — Only adapter unavailability may skip
    Given the assistant HTTP adapter never binds and answers 503 assistant_http_not_ready
    When the regression test runs
    Then the test may skip for infrastructure unavailability
    And no other condition in the test may skip
```

### Implementation Plan

1. **Resolve the open unknown first.** Run the existing test against the live
   disposable stack and record which branch the fabricated-city prompt actually
   takes today at HEAD. This is the fact `bug.md` declines to guess at, and it
   decides whether the primary fix (option a) is sufficient or the declared
   fallback (option b) is required. Record the raw envelope.
2. **Demonstrate the defect before changing it.** Show the current file
   reporting SKIP with exit 0 on an off-contract status. A described defect is
   not a reproduced one.
3. **Replace the guard at lines 98–100** with the two-branch outcome-space
   assertion from `design.md` → *Solution Approach*: grounded-with-sources is
   accepted on evidence; the full SCOPE-074-04B contract is asserted otherwise;
   any third shape fails.
4. **Tighten the line-71 adapter-readiness skip** to fire only on HTTP 503
   `assistant_http_not_ready`, and drop the pre-judging clause from its message.
5. **Correct the header comment** (lines 16–20) so it describes what the code now
   does. No surviving comment may promise a failure mode the code cannot produce.
6. **Prove the flip and the non-tautology**, then run the full assistant e2e
   package for neighbouring regressions.

### Reference Implementation

`tests/e2e/assistant/high_band_refusal_e2e_test.go` — delivered in the same
session as the DI-5 finding and deliberately built not to repeat this defect.
One `t.Skip` (line 90, adapter never bound), 18 `t.Errorf`/`t.Fatalf` contract
assertions, nondeterminism absorbed at the input and in the outcome space. Follow
its shape.

### Test Plan

| Row | Scenario | Test Type | Category | File | Description | Command | Live |
|---|---|---|---|---|---|---|---|
| TP-B074-002-01 | SCN-BUG-074-002-01 | Adversarial regression | `e2e-api` | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | An off-contract status with no sources FAILS; pre-fix the same condition SKIPs | `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround'` | Yes |
| TP-B074-002-02 | SCN-BUG-074-002-02 | Non-tautology control | `e2e-api` | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | A grounded-with-sources envelope still passes, proving the fix is not an always-fail | `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround'` | Yes |
| TP-B074-002-03 | SCN-BUG-074-002-03 | Contract regression | `e2e-api` | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | The four SCOPE-074-04B assertions still hold on the capture path | `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround'` | Yes |
| TP-B074-002-04 | SCN-BUG-074-002-04 | Bailout scan | `unit` | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | Static scan proves the only `t.Skip` in executable code is the 503 adapter guard | `grep -nE 't\.Skip\|t\.Errorf\|t\.Fatalf' tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` | No |
| TP-B074-002-05 | all | Neighbour regression | `e2e-api` | `tests/e2e/assistant/` | Full assistant e2e package shows no collateral regression | `./smackerel.sh test e2e --go-package assistant` | Yes |

### Definition of Done — Tiered Validation

**Core items**

- [ ] The open unknown is resolved: a live run records which branch the fabricated-city prompt takes at HEAD, with the raw envelope captured. → Evidence: [report.md](report.md)
- [ ] Defect reproduced before the fix: the current file demonstrably reports SKIP (exit 0, nothing reported) on an off-contract status. → Evidence: [report.md](report.md)
- [ ] Adversarial regression flips: the same off-contract condition reports FAIL after the fix. A regression that cannot be shown to flip the outcome does not satisfy this item. → Evidence: [report.md](report.md)
- [ ] Non-tautology proven: a legitimately grounded envelope still passes post-fix, so the fix is not an always-fail. → Evidence: [report.md](report.md)
- [ ] The four SCOPE-074-04B assertions (`capture_route`, nil `confirm_card`, nil `disambiguation_prompt`, canonical body) execute on every run that got HTTP 200 with a decodable envelope. → Evidence: [report.md](report.md)
- [ ] Bailout scan clean: the only `t.Skip` calls in executable code guard infrastructure availability — HTTP 503 `assistant_http_not_ready` (adapter bind timing) and `error_cause=provider_unavailable` (upstream failed before the grounding decision). Neither keys on the `saved_as_idea` status the canonical-ack assertions police. Every contract assertion uses `t.Errorf`/`t.Fatalf`. → Evidence: [report.md](report.md)
- [ ] Header comment (lines 16–20) matches the code; no comment promises a failure mode the code cannot produce. → Evidence: [report.md](report.md)
- [ ] Policy compliance: no failure-condition early exit remains, per `.github/copilot-instructions.md` line 331. → Evidence: [report.md](report.md)
- [ ] Change boundary honoured: the diff touches only `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` and this packet; zero `internal/` changes. → Evidence: [report.md](report.md)
- [ ] If option (b) was adopted instead of the primary, the falsifiable condition in `design.md` is shown to have been met by live evidence. → Evidence: [report.md](report.md)

**Build Quality Gate**

- [ ] Full assistant e2e package passes with no collateral regression; zero skipped required tests. → Evidence: [report.md](report.md)
- [ ] Build, lint, and format clean with zero warnings; `artifact-lint.sh` exit 0 and `state-transition-guard.sh` PASS. → Evidence: [report.md](report.md)
- [ ] Validate-owned certification remains authoritative; this packet does not self-certify. → Evidence: [state.json](state.json)
