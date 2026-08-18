# Bug Fix Design: BUG-074-002

## Root Cause Analysis

### Verification Method

Every claim below comes from reading the file in this session at repo HEAD
`d62f2e750ab315767199ce29b5862e9ae509cccd`. No line number was inferred from the
originating DI-5 note without independent re-verification; the confirming
commands and their raw output are recorded in `report.md` → *Static
verification*.

### Investigation Summary

`tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` (119 lines, sha256
`3d695eac…92b1d`) declares itself an adversarial regression for spec 074
SCOPE-074-04B. Its header, lines 16–20:

```go
// Adversarial coverage: if the facade silently dropped the no-ground
// capture (regression of SCOPE-074-04A change-boundary "capture
// failure must be observable") OR if it routed to a different status
// (regression of SCOPE-074-04B canonical-ack rule), this test would
// fail.
```

The control flow does not support the second half of that claim. At line 98:

```go
if env.Status != string(contracts.StatusSavedAsIdea) {
        t.Skipf("live stack did not route through the open-knowledge no-ground capture path (status=%q, capture_route=%v); ...",
                env.Status, env.CaptureRoute)
}
```

All four SCOPE-074-04B contract assertions are placed after it (103, 106, 109,
117). "Routed to a different status" is the exact condition that takes the skip.

### Root Cause

**The test absorbs live-model nondeterminism at the assertion layer, and the
variable it tolerates is the contract under test.**

The author had a real problem: an LLM might ground a prompt intended to be
ungroundable, in which case the no-ground path was never exercised and asserting
the canonical ack would be wrong. The chosen remedy was to observe the status and
bail out when it was not the expected one.

That remedy cannot distinguish the two reasons the status might differ:

| Reason the status is not `saved_as_idea` | Correct verdict |
|---|---|
| The model genuinely grounded the answer — the no-ground path was never entered | not applicable; not a failure |
| The facade regressed the canonical-ack rule | **FAIL** |

Both take the same branch, and that branch is `t.Skipf`. Tolerance and detection
were implemented as the same predicate, so tolerance wins in every case. The
result is a test that is structurally incapable of failing on the regression it
names in its own header.

A second `t.Skipf` at line 71 (adapter not ready after 5 minutes) is a further
silent-pass path. It is closer to legitimate — adapter binding is infrastructure
availability, not a contract outcome — but its 5-minute budget and its message,
which pre-emptively characterises the condition as "test-infra timing rather than
a SCOPE-074-04B regression", make it a second place the test can go green having
proven nothing.

### Policy Mapping

`.github/copilot-instructions.md` line 331 forbids "bailout returns … or
equivalent failure-condition early exits". Lines 98–100 are such an exit: the
condition is an outcome of the contract under test.

### Impact Analysis

- **Affected components:** `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` only. No `internal/` production code is implicated.
- **Affected data:** none.
- **Affected users:** none. This is a detection-capability defect.
- **Affected assurance:** spec 074 SCOPE-074-04B's live-regression claim (TP-074-14 / SCN-074-A01) rests on a test that cannot fail on one regression class it advertises.

### Assurance Context Worth Recording

Spec 074's own DoD for TP-074-14 (`specs/074-capture-as-fallback-policy/scopes.md`
line 409) was closed on **Substitute Evidence** — the live e2e was blocked by
foreign-owned infra findings, and the item carries an explicit **Uncertainty
Declaration** at line 414 stating that "a dedicated live e2e re-assertion will be
added by the follow-on … spec once foreign infra is resolved."

This matters for scoping the fix, and it cuts in a direction that must be stated
honestly rather than used for emphasis: the live assertion was *later* exercised
for real (see `report.md` → *Refinement of the DI-5 unknown*), so the spec-074
substitute-evidence gap has since been partly discharged in practice. The point
retained here is narrower — the test that now carries that assurance is the one
with the skip-guard, so the assurance is contingent on a live routing outcome
that has never been pinned.

## Fix Design

### Solution Approach — PRIMARY: option (a), assert unconditionally

**Chosen: assert the envelope unconditionally. An unexpected status IS the hunted
regression, so it must fail rather than skip.**

Applied naively — deleting the guard and asserting `status == saved_as_idea`
flat — option (a) would trade a silent-pass for a flaky required test, and a
flaky required test eventually gets muted, which reproduces this bug by another
route. So the guard is not merely deleted; it is **replaced by an
outcome-space assertion**, which is how the sibling test solves the same problem.

The legitimate outcome space of this turn is closed and small:

| Outcome | Wire evidence | Verdict |
|---|---|---|
| (i) The model genuinely grounded the prompt | an answered status **with** `len(sources) > 0` **and** `capture_route == false` | legitimate; SCOPE-074-04B does not apply; assert *that* shape |
| (ii) No-ground + fallback-eligible | `saved_as_idea` + `capture_route == true` + nil confirm/disambiguation + canonical body | legitimate; assert the full SCOPE-074-04B contract |
| (iii) anything else | — | **FAIL** |

The current test collapses (i) and (iii) into one skip. The fix keeps tolerance
for (i) — but grants it only **on evidence** (sources actually present), never on
a bare status mismatch — and routes (iii) to `t.Errorf`/`t.Fatalf`. Every branch
then asserts something, so there is no path on which the test passes having
proven nothing.

This is the shape of `tests/e2e/assistant/high_band_refusal_e2e_test.go`,
delivered in the same session as the DI-5 finding and deliberately built not to
repeat this defect. Verified properties of that reference:

- Exactly **one** `t.Skip` in executable code, **line 90**, guarding HTTP 503
  `assistant_http_not_ready` — the adapter never binding. Infrastructure
  availability, not a contract outcome.
- Its other two `t.Skip` matches (lines 31, 36) are **prose inside the header
  comment** explaining why the pattern is avoided.
- All **18** contract assertions use `t.Errorf` / `t.Fatalf`.
- It absorbs nondeterminism twice, and never at the assertion layer: at the
  **input** (`/ask` is a slash shortcut that sets `ScenarioID` explicitly, so the
  band-high precondition cannot drift with model behaviour) and in the **outcome
  space** (the invariant asserted holds for every honest refusal cause).

Cite it as the pattern to follow.

**Why primary:** it preserves the one property TP-074-14 exists to provide — a
*live* wire-level assertion — while removing the forbidden bailout. It is
test-only, needs no new fixture or seam, and makes the line 16–20 header claim
true rather than requiring it to be softened.

### Alternative Considered — option (b), pin the branch with a deterministic stub

Pin the no-ground branch with a deterministic stub so the assertions always run.

**Rejected as primary, retained as the declared fallback.** Rejected because
TP-074-14 is specified as `e2e-api` **live** (`scopes.md` line 404) and stubbing
the open-knowledge outcome removes the live model from the exact path under
test — converting the strongest available proof into something closer to an
integration test, and hollowing out the "live" claim that spec 074's
substitute-evidence DoD is already leaning on. It also does not follow from the
evidence: nondeterminism here is **hypothesised, not observed** (see the open
unknown), so paying liveness to suppress it is premature.

**Adopt (b) only if** a live run demonstrates the branch genuinely oscillates in
a way that cannot be disambiguated on the wire — i.e. the (i)/(iii) split above
is not decidable from the envelope. That condition is falsifiable and is what
the fix scope's first task exists to test.

### Secondary Correction

The line-71 adapter-readiness skip is retained (AC-4 permits it) but tightened to
the sibling's shape: it must fire **only** on HTTP 503
`assistant_http_not_ready`, and its message must not pre-judge the condition as
"not a SCOPE-074-04B regression". Narrowing, not removal — the adapter genuinely
binds late, and that is real infrastructure unavailability.

### Explicitly Out Of Scope

- Any change under `internal/assistant/`. This packet asserts no production defect.
- Reopening spec 074 SCOPE-074-04B's product rule.
- Sweeping the analogous pattern elsewhere in the repository. If the fix surfaces
  siblings, they are routed as new findings, not folded in silently.

## Complexity Tracking

| Decision | Simpler fix considered | Why rejected |
|---|---|---|
| Replace the guard with a two-branch outcome-space assertion rather than deleting it and asserting a single status | Delete lines 98–100; assert `status == saved_as_idea` flat | The flat form fails on a legitimately-grounded run, i.e. on model behaviour rather than on a code regression. A required test that fails for a non-defect gets muted or `-skip`ed, which restores the silent-pass this bug exists to remove — by a slower route. The two-branch form costs a handful of lines and removes that failure mode. |
| Tighten the line-71 skip instead of leaving it as-is | Leave line 71 untouched; fix only lines 98–100 | Leaving it is defensible (it is infrastructure, not an outcome), but its message asserts a conclusion it has not tested — that the condition is "not a SCOPE-074-04B regression". Narrowing the wording costs one line and removes a second place the suite can go green having proven nothing. |

## Verification Design

The fix is proven by flipping the outcome, not by asserting it was fixed:

1. **RED (pre-fix, on the current file):** drive the test with an envelope whose
   status is not `saved_as_idea` and show the current code reports **SKIP** with
   exit 0 — the defect, demonstrated rather than described.
2. **GREEN (post-fix):** the same condition reports **FAIL**.
3. **Non-tautology:** the legitimately-grounded outcome (i) must still pass
   post-fix, proving the fix did not simply harden into an always-fail.
4. **Live:** the full assistant e2e package runs against the disposable stack to
   show no neighbouring regression.

Task 1 of the fix scope must also record which branch the live stack actually
takes today — the open unknown this packet declines to guess at.
