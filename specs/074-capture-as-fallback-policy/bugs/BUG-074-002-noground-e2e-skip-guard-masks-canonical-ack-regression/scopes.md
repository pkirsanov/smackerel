# Scopes: BUG-074-002 No-ground E2E skip-guard masks the canonical-ack regression

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> **Execution state (updated 2026-08-23, `bubbles.test`).** Filed 2026-08-18 by
> `bubbles.bug` as a FILING + ROOT-CAUSE task, which implemented nothing. The
> implementation landed 2026-08-23 at commit `343d6076` under `bubbles.implement`;
> see `report.md` → *Implementation Delta*. The test phase then ran and checked
> **8 of 19** DoD items; see `report.md` →
> *Test Phase Per-DoD Evidence 2026-08-23*. The remaining **11** are unchecked for
> two distinct reasons, annotated individually below: five need a live envelope
> this hardware tier cannot produce (every run returns `provider_unavailable`, so
> branches 1, 2 and 4 of the shipped switch are untraversed), and the rest are
> blocked by DI-7 (the `test e2e` lane is refused by `disk-preflight`) or by DI-8
> (two planning artifacts describe behaviour the shipped test deliberately does
> not have). Ownership stays with `bubbles.test` for the suite re-run and
> `bubbles.plan` for DI-8; see `state.json` → `routing`.

## Scope 1: Make the no-ground live E2E fail — not skip — on a canonical-ack regression

**Status:** In Progress
**Depends On:** none
**Owner:** `bubbles.test`
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
| TP-B074-002-06 | SCN-BUG-074-002-01, SCN-BUG-074-002-03 | Regression E2E | `e2e-api` | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` :: `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` | Regression: the persistent live-stack E2E that permanently protects the SKIP→FAIL flip and the SCOPE-074-04B canonical-ack contract against reintroduction of a status-keyed escape hatch | `./smackerel.sh test e2e --go-package assistant --go-run 'CaptureFallbackOpenKnowledgeNoGround'` | Yes |

### Definition of Done — Tiered Validation

**Core items**

- [x] The open unknown is resolved: a live run records which branch the fabricated-city prompt takes at HEAD, with the raw envelope captured. → Evidence: [report.md](report.md#live-execution-of-the-adversarial-flip-2026-08-23) "Live Execution — The Adversarial Flip, Demonstrated" — raw envelope captured verbatim: `status="unavailable" error_cause="provider_unavailable" capture_route=false sources=0`. At HEAD on this tier the prompt takes **branch 3** (typed upstream failure), so the grounding decision is never reached. That answers which branch, and it also means DI-3 (is `saved_as_idea` reachable?) stays open — this item asks only for the branch and the envelope, and both are recorded.
- [x] Defect reproduced before the fix: the current file demonstrably reports SKIP (exit 0, nothing reported) on an off-contract status. → Evidence: [report.md](report.md#live-execution-of-the-adversarial-flip-2026-08-23) "Live Execution" run 1 — the pre-fix file on the live stack: `--- SKIP: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.19s)`, package exit 0, on an envelope whose status was `unavailable` and not `saved_as_idea`. Nothing about the canonical-ack contract was established and nothing said so.
- [x] Adversarial regression flips: the same off-contract condition reports FAIL after the fix. A regression that cannot be shown to flip the outcome does not satisfy this item. → Evidence: [report.md](report.md#live-execution-of-the-adversarial-flip-2026-08-23) "Live Execution" runs 1 and 2 — same stack, same prompt, same envelope: `--- SKIP ... (0.19s)` exit 0 before, `--- FAIL ... (12.40s)` `E2E_RC=1` after. **Scope of the claim:** run 2 is the fixed file before the typed `provider_unavailable` branch was added; on the shipped file that same envelope reports a typed SKIP (run 3). What flipped, and what this item asserts, is that the status-keyed escape hatch is gone. The shipped artifact's own FAIL paths (branch 2, `default:`) are untraversed — see [Test Phase Per-DoD Evidence](report.md#test-phase-per-dod-evidence-2026-08-23) → Uncertainty Declaration.
- [ ] Non-tautology proven: a legitimately grounded envelope still passes post-fix, so the fix is not an always-fail. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED.** Branch 4 was never traversed: every live run on this tier returned `sources=0`. Run 3 shows the shipped test exiting 0, which proves it is not an unconditional always-fail, but that is weaker than the grounded-envelope proof this item names and substituting it would be an over-claim.
- [ ] The four SCOPE-074-04B assertions (`capture_route`, nil `confirm_card`, nil `disambiguation_prompt`, canonical body) execute on every run that got HTTP 200 with a decodable envelope. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED, for two reasons.** Live: branch 1 was never reached, so the four assertions have executed zero times against a live envelope. Design: as written this item describes behaviour the shipped switch deliberately does not have — the four assertions run inside branch 1 only, because asserting the capture contract on an upstream-outage envelope would claim a violation nobody observed. The wording predates the five-branch design; recorded as DI-8 and owned by `bubbles.plan`.
- [ ] Bailout scan clean: the only `t.Skip` calls in executable code guard infrastructure availability — HTTP 503 `assistant_http_not_ready` (adapter bind timing) and `error_cause=provider_unavailable` (upstream failed before the grounding decision). Neither keys on the `saved_as_idea` status the canonical-ack assertions police. Every contract assertion uses `t.Errorf`/`t.Fatalf`. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Static verification executed this phase" — `grep -cE 't\.Skip' …` = **2** (lines 119 and 216 post-correction; formerly 112 and 209), `grep -cE 't\.Errorf|t\.Fatalf' …` = **19**. Line 119 keys on HTTP 503 `assistant_http_not_ready`; line 216 keys on `error_cause=provider_unavailable` and asserts `status` and `capture_route` before skipping. Neither reads `saved_as_idea`.
- [x] Header comment (lines 16–20) matches the code; no comment promises a failure mode the code cannot produce. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Correction made by this phase" — it did **not** match when this phase checked it: lines 16-18 claimed the classify and assert field sets were globally disjoint, but branch 1 selects on `status` and branch 4 on `sources`, both of which are also asserted elsewhere. Corrected to state the per-branch property the code actually has (no branch asserts its own selector), verified against all five `case` selectors and every assertion line in the per-branch table. The corrected block spans lines 16-30. `gofmt -l` empty, `go vet -tags e2e ./tests/e2e/assistant/` exit 0.
- [x] Policy compliance: no failure-condition early exit remains, per `.github/copilot-instructions.md` line 331. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Static verification executed this phase" — `grep -nE '^\s*return\s*$|t\.SkipNow|\.only\(|t\.Skip\(\)' …` returned **exit 1 (zero matches)**: no bare `return`, no `t.SkipNow`, no `.only(`, no argument-less `t.Skip()`. Line 331 reads "Required tests MUST NOT use bailout returns … or equivalent failure-condition early exits"; the two surviving skips key on infrastructure availability, not on a contract outcome.
- [x] Change boundary honoured: the diff touches only `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` and this packet; zero `internal/` changes. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Static verification executed this phase" — `git diff --name-only 343d6076~1..HEAD | grep -v '^specs/'` returns exactly one path, `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`. Across all five packet commits (`343d6076`, `352599e1`, `30d31da1`, `fa9a1582`, `9c256fd9`) every other file is inside this packet directory. Zero `internal/` paths. This phase's own edit is to the same single file.
- [x] If option (b) was adopted instead of the primary, the falsifiable condition in `design.md` is shown to have been met by live evidence. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Static verification executed this phase" — **vacuously satisfied: option (b) was not adopted.** `design.md` → *Alternative Considered — option (b)* keeps the deterministic stub as a declared fallback only, adoptable "only if a live run demonstrates the branch genuinely oscillates". No such oscillation was observed, and the shipped test carries no stub: `grep -nE 'httptest|stub|fake|mock|inject' …` exit 1 (zero matches), while `loadHTTPTurnLiveStack` (line 81) and `postAssistantTurn` (line 109) confirm the live path option (a) specifies. Nothing to demonstrate.
- [ ] `SCN-BUG-074-002-01` holds on the live stack: an envelope whose status is not `saved_as_idea` and which carries no grounded sources makes the test report a failure, and the same run reports no skip. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED: falsified as written by this packet's own run 3.** That run's envelope was exactly the shape the scenario names — `status="unavailable"` (not `saved_as_idea`), `sources=0` — and the shipped test reported SKIP through the typed `provider_unavailable` branch, not FAIL. The carve-out is defensible and typed, but the Gherkin has not been updated to admit it, and a test phase does not check an item its own evidence contradicts. Recorded as DI-8 and owned by `bubbles.plan`.
- [ ] `SCN-BUG-074-002-02` holds on the live stack: a turn the model genuinely grounded — at least one source, capture route false — passes without asserting the capture acknowledgement, and reports no skip. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED.** Branch 4 was never traversed; this hardware tier does not serve the model, so no grounded envelope was produced and the grounded-turn control has not run against a live stack.
- [ ] `SCN-BUG-074-002-03` holds on the live stack: on a captured no-ground turn the test asserts capture route true, confirm card absent, disambiguation prompt absent, and the canonical saved-as-idea acknowledgement present in the body. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED.** Branch 1 was never traversed. Every live run returned `error_cause="provider_unavailable"`, so the turn never reached the grounding decision and no capture envelope was produced. The four canonical-ack assertions remain unexecuted against a live stack.
- [ ] `SCN-BUG-074-002-04` holds as shipped, and shipped reality is TWO infrastructure-keyed skips rather than the single one the scenario's title and Given describe: HTTP 503 `assistant_http_not_ready` (the adapter never bound) and `error_cause=provider_unavailable` (upstream failed before the grounding decision was reached). Both key on infrastructure availability and neither keys on the `saved_as_idea` status the canonical-ack assertions police, so the scenario's Then clause — skip permitted only for infrastructure unavailability — is satisfied. This item records the second skip faithfully instead of narrowing the scenario to match delivery; the title/Given divergence is planning-owned and is recorded here so it stays visible. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) "Static verification executed this phase" — census confirms exactly two `t.Skip` sites as shipped: line 119 (HTTP 503 `assistant_http_not_ready`) and line 216 (`error_cause=provider_unavailable`, preceded by `status` and `capture_route` assertions). Both key on infrastructure availability; neither reads `saved_as_idea`, so the scenario's Then clause holds as this item states it.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior exist and pass: `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` in `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` permanently covers the SKIP→FAIL flip (`SCN-BUG-074-002-01`), the grounded-turn control (`SCN-BUG-074-002-02`) and the canonical-ack contract (`SCN-BUG-074-002-03`) per Test Plan row TP-B074-002-06. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED.** The test exists and contains a branch for each of the three scenarios, verified statically. "And pass" is unproven for `SCN-BUG-074-002-02` and `SCN-BUG-074-002-03`, which is the same untraversed-branch gap as the two items above.
- [ ] Broader E2E regression suite passes: the full `tests/e2e/assistant/` package runs green with zero skipped required tests, proving no neighbouring assistant regression was introduced (TP-B074-002-05). → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED: the suite did not run.** `./smackerel.sh test e2e --go-package assistant` was refused by `disk-preflight` at exit 1 (C: 39 GB free against a 40 GB requirement) before any container started. Not exit 73, so this is the disk guard rather than the suite lock. Neither the documented `DISK_PREFLIGHT_OVERRIDE=1` bypass nor a shared-host Docker prune was used. Blocker recorded as DI-7.

**Build Quality Gate**

- [ ] Full assistant e2e package passes with no collateral regression; zero skipped required tests. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED.** Same blocker as the broader-suite item above: `disk-preflight` refused the lane at exit 1 and no test executed. DI-7.
- [ ] Build, lint, and format clean with zero warnings; `artifact-lint.sh` exit 0 and `state-transition-guard.sh` PASS. → Evidence: [report.md](report.md#test-phase-per-dod-evidence-2026-08-23) — **UNCHECKED: this item is a conjunction and only one half holds.** Build side is clean and recorded: `./smackerel.sh check` exit 0, `./smackerel.sh format --check` exit 0 (78 files already formatted), `./smackerel.sh lint` exit 0, `go vet -tags e2e ./tests/e2e/assistant/` exit 0. `state-transition-guard.sh` is **FAIL** with `failureCount: 13` and `failedGateIds: [G022,G027,G136]`, which is the correct verdict while nine DoD items are legitimately unchecked.
- [x] Validate-owned certification remains authoritative; this packet does not self-certify. → Evidence: [state.json](state.json) — `certification.certifiedCompletedPhases` is `[]`, `certification.certifierAgent` is `null`, `certification.certifiedAt` is `null`, and `certification.status` remains `in_progress`. This phase wrote none of them; it recorded its claim in `execution.completedPhaseClaims` and `execution.executionHistory` only, which are the execution-owned fields.

```text
$ python3 -c "import json; s=json.load(open('state.json')); ..."   # read-only projection of state.json
certification.status = "in_progress"
certification.certifierAgent = null
certification.certifiedAt = null
certification.certifiedCompletedPhases = []
top-level status = "in_progress"
completedScopes = null
```

The four `certification.*` fields are unset because no `bubbles.validate` run
has certified this packet yet. That is the assertion this item makes, and the
projection above is the whole of the evidence for it: an agent other than
validate would have had to write `certifierAgent` to break it, and none did.
