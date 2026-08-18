# Bug: BUG-074-002 No-ground E2E skip-guard masks the canonical-ack regression it advertises

## Summary

`TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` places all four SCOPE-074-04B contract assertions below a `t.Skipf` keyed on the very status it is hunting, so a regression that moves the turn off `saved_as_idea` is reported as SKIP instead of FAIL.

## Severity

- [ ] Critical
- [ ] High
- [x] Medium
- [ ] Low

**Severity class: test-integrity, NOT production behaviour.** No user-facing
defect is asserted or proven by this packet. Nothing about the shipped facade is
claimed to be broken. What is broken is the *detection capability* of one live
E2E: it cannot fail on one specific regression class it advertises that it
guards. Filing this at Medium reflects a real hole in the safety net, not a
degraded product surface. Do not read this packet as evidence of a production
regression — see *Explicit Non-Claims* below.

## Status

- [x] Reported
- [x] Confirmed (defect confirmed by source inspection — see *Verification Method*)
- [ ] In Progress
- [ ] Fixed
- [ ] Verified
- [ ] Closed

## Provenance

Discovered as finding **DI-5** while working
`specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea/`
(the `## Discovered Issues` table, row DI-5, `report.md` line 1744). That packet
dispositioned DI-5 as **"routed — spec-074-owned, not this packet's to fix"** and
named this exact bug id as *"not filed here"*. This packet closes that routing
loop. The DI-5 analysis is carried forward faithfully and is refined — not
contradicted — by one additional verified fact recorded in `report.md`
(*Refinement of the DI-5 unknown*).

## Affected Artifact

| Field | Value |
|---|---|
| File | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` |
| Test | `TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround` (line 39) |
| Contract | spec 074 **SCOPE-074-04B** (`specs/074-capture-as-fallback-policy/scopes.md` line 365) |
| Test-plan row | **TP-074-14** → scenario **SCN-074-A01** (`scopes.md` line 404) |
| File length | 119 lines |
| File sha256 | `3d695eacccff830ac36e1fdd90ada197bed2643e491f0d2af66dbe0a97192b1d` |
| Repo HEAD at filing | `d62f2e750ab315767199ce29b5862e9ae509cccd` |

## Reproduction Steps

This is a static test-integrity defect. It reproduces by reading the file; it
does not require the stack to be up.

1. Open `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go`.
2. Read the header comment beginning line 16. It claims that if the facade
   "routed to a different status (regression of SCOPE-074-04B canonical-ack
   rule), this test would fail" (clause spans lines 18–20).
3. Read line 98: `if env.Status != string(contracts.StatusSavedAsIdea) {`.
4. Read lines 99–100: the body of that `if` is `t.Skipf(...)`.
5. Observe that all four SCOPE-074-04B contract assertions sit **below** that
   guard — lines 103, 106, 109, 117.
6. Conclude: a different status takes the branch at 98 and **skips**. The
   outcome the header promises (FAIL) is unreachable for that regression class.

Confirming grep:

```bash
grep -n 't\.Skipf\|t\.Errorf\|t\.Fatalf\|env\.Status != ' \
  tests/e2e/assistant/capture_fallback_trigger_e2e_test.go
```

## Expected Behavior

A required regression test MUST fail — not skip — when the behaviour it guards
regresses. For this test specifically: if a live no-ground open-knowledge turn
lands on any status other than `saved_as_idea`, that IS the SCOPE-074-04B
canonical-ack regression, and the test must report FAIL.

## Actual Behavior

An unexpected status is reported as *"live stack did not route through the
open-knowledge no-ground capture path"* and the test is **skipped**. The run is
green. Nothing is reported to anyone. The four contract assertions never
execute.

## Precise Defect Statement

The defect is **narrower** than "the test does nothing", and this packet must not
overstate it. Stated exactly:

| Line(s) | Assertion | Enforcement |
|---|---|---|
| 83 | `facade_invoked` | `t.Errorf` — **unconditional**, always runs |
| 86 | `transport` | `t.Errorf` — **unconditional**, always runs |
| 89 | `transport_message_id` echo | `t.Errorf` — **unconditional**, always runs |
| 98–100 | `env.Status != StatusSavedAsIdea` | **`t.Skipf` — the defect** |
| 103 | `capture_route == true` | `t.Errorf` — **below the guard** |
| 106 | `confirm_card == nil` | `t.Errorf` — **below the guard** |
| 109 | `disambiguation_prompt == nil` | `t.Errorf` — **below the guard** |
| 117 | body contains `saved as an idea` | `t.Errorf` — **below the guard** |

Therefore:

- The test is **NOT inert wholesale**. Lines 83, 86 and 89 assert
  unconditionally via `t.Errorf`.
- The header's **other** half — "the facade silently dropped the no-ground
  capture" — **does remain enforceable** via line 103, but **only while the
  status is still `saved_as_idea`**.
- What is **unenforceable** is any regression that moves the status **off**
  `saved_as_idea` — which is the canonical-ack rule itself, i.e. precisely the
  thing the test advertises that it guards.
- A **second** `t.Skipf` at **line 71** (adapter not ready after 5 minutes) adds
  a further silent-pass path.

So the accurate one-line form is: *the half of the header claim about status
routing is false as written — the outcome is SKIP, not FAIL.*

## Policy Violation

`.github/copilot-instructions.md` → **Adversarial Regression Tests For Bug
Fixes**, line 331:

> Required tests MUST NOT use bailout returns such as
> `if (page.url().includes('/login')) { return; }` or equivalent
> failure-condition early exits.

The guard at lines 98–100 is exactly an equivalent failure-condition early exit:
the condition it exits on (`status != saved_as_idea`) is an *outcome of the
contract under test*, not an infrastructure precondition.

## Explicit Non-Claims

Recorded so this packet cannot be misread as a production incident:

1. **No production regression is asserted or proven.** No user-facing defect is
   implied. The facade's no-ground capture behaviour is not claimed to be broken.
2. **No contradiction with BUG-061-009 is asserted.** BUG-061-009 changed
   band-high refusal rendering, so it is reasonable to *ask* whether it perturbs
   this test. The honest answer carried forward from DI-5 is that no
   contradiction was proven and none is asserted. SCOPE-074-04B governs an
   open_knowledge turn the agent *refused* (`status="refused"`) landing on
   `SavedAsIdea` + `capture_route=true`; BUG-061-009 governs a band-high
   `requires_provenance` turn that returns *OK but uncited*, rendering
   `StatusUnavailable` + `ErrNoGroundedAnswer`. Those are plausibly different
   branches and may legitimately coexist.
3. **Which branch the live stack takes today is NOT verified.** See *Open
   Unknown* below.

## Open Unknown (recorded, not smoothed over)

It was **NOT verified** which branch the live stack currently takes for this
test's fabricated-city prompt (`"what is the population of the fictional city of
Zorthonia-by-the-Sea in 2024?"`). That needs a live run, which was **not
performed** in this session.

The defect that holds **regardless of branch** is the skip-guard itself: were
that prompt ever to move onto the honest-refusal branch, the test would report
**SKIP rather than FAIL** and would tell nobody.

`report.md` → *Refinement of the DI-5 unknown* records verified historical
evidence that **narrows but does not close** this unknown.

## Root Cause

The test absorbs live-LLM nondeterminism **at the assertion layer** (skip when
the observed status is not the expected one) instead of at the **input layer**
(choose an input whose outcome is determined) or in the **outcome space** (assert
an invariant that holds across every legitimate outcome). Because the tolerated
variable *is itself the contract under test*, tolerance and detection collapse
into the same branch — and tolerance wins.

## Related

- Feature: `specs/074-capture-as-fallback-policy/` (SCOPE-074-04B)
- Origin finding: `specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea/report.md` → `## Discovered Issues` → **DI-5**
- Sibling bug (same spec, does not cover this): `specs/074-capture-as-fallback-policy/bugs/BUG-074-001-canonical-capture-response/` (status `done`)
- Correct-shape reference: `tests/e2e/assistant/high_band_refusal_e2e_test.go`
- Policy: `.github/copilot-instructions.md` line 331
