# Report: BUG-074-002 No-ground E2E skip-guard masks the canonical-ack regression

## Summary

**What this session did:** filed this bug packet and completed root-cause
analysis. **What it did not do:** implement, test, or verify any fix.

- **Changed surfaces:** this packet only — `bug.md`, `spec.md`, `design.md`,
  `scopes.md`, `report.md`, `uservalidation.md`, `scenario-manifest.json`,
  `state.json`. **Zero** source files were modified.
  `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` was **read, not
  edited**, per the filing constraint.
- **Scenarios validated:** none by execution. SCN-BUG-074-002-01 through -04 are
  declared and unproven; they are the fix scope's obligation.
- **Repo HEAD at filing:** `d62f2e750ab315767199ce29b5862e9ae509cccd`
  (`d62f2e75`); working tree clean at read time.

## Completion Statement

**This bug is NOT fixed, NOT verified, and NOT done.** Status is
`route_required`, routed to `bubbles.implement`.

The only claim this report makes is that the defect described in `bug.md` exists
in the source as read in this session, and that claim is backed by the executed
commands in *Static verification* below. Every DoD item in `scopes.md` is
unchecked because none has been satisfied. No live stack was started, no test was
run, no fix was written.

Filing this packet discharges the routing obligation recorded as **DI-5** in
`specs/061-conversational-assistant/bugs/BUG-061-009-high-band-refusal-masked-as-saved-as-idea/report.md`
(line 1744), which named this bug id and explicitly stated it was "not filed
here".

### Code Diff Evidence

**Not applicable, deliberately.** This packet is a filing + root-cause task with
no implementation. There is no source diff to record, and recording an
artifact-only diff as implementation evidence would be exactly the substitution
the report-sections registry warns against. The fix scope owns this section.

## Test Evidence

### Verification method and its limits

The defect is **static**: it is a property of the test's control flow, provable
by reading the file. Every line number below was re-derived in this session with
an executed command; none was carried over from the DI-5 note on trust.

**What this evidence does NOT establish:** that the live stack currently takes
any particular branch. See *Open unknown* below.

---

### Static verification — control flow of the defective test

**Claim Source:** executed (current session)
**Command:**

```bash
wc -l tests/e2e/assistant/capture_fallback_trigger_e2e_test.go
grep -n 'Adversarial coverage\|this test would\|t\.Skipf\|t\.Fatalf\|t\.Errorf\|env\.Status != \|CaptureRoute\|ConfirmCard\|DisambiguationPrompt\|saved as an idea\|FacadeInvoked\|env\.Transport \|TransportMessageID != \|func TestAssistantHTTPE2E' \
  tests/e2e/assistant/capture_fallback_trigger_e2e_test.go
sha256sum tests/e2e/assistant/capture_fallback_trigger_e2e_test.go
```

**Exit Code:** 0
**Output:**

```text
=== FILE TOTAL LINES ===
119 tests/e2e/assistant/capture_fallback_trigger_e2e_test.go

=== KEY LINE NUMBERS (capture_fallback_trigger_e2e_test.go) ===
16:// Adversarial coverage: if the facade silently dropped the no-ground
19:// (regression of SCOPE-074-04B canonical-ack rule), this test would
39:func TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround(t *testing.T) {
71:            t.Skipf("assistant adapter not ready after 5min on this run; routing this as test-infra timing rather than a SCOPE-074-04B regression (unit + integration coverage proves the no-ground hook is wired). Last body=%s", string(body))
76:            t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(body))
80:            t.Fatalf("decode: %v\nbody=%s", err, string(body))
82:    if !env.FacadeInvoked {
83:            t.Errorf("facade_invoked = false; want true")
85:    if env.Transport != httpadapter.TransportName {
86:            t.Errorf("transport = %q, want %q", env.Transport, httpadapter.TransportName)
88:    if env.TransportMessageID != req.TransportMessageID {
89:            t.Errorf("transport_message_id echo = %q, want %q", env.TransportMessageID, req.TransportMessageID)
98:    if env.Status != string(contracts.StatusSavedAsIdea) {
99:            t.Skipf("live stack did not route through the open-knowledge no-ground capture path (status=%q, capture_route=%v); facade no-ground hook is covered by unit + integration tests",
100:                   env.Status, env.CaptureRoute)
102:    if !env.CaptureRoute {
103:            t.Errorf("capture_route = false; no-ground fallback MUST set capture_route=true (regression of SCOPE-074-04B canonical ack)")
105:    if env.ConfirmCard != nil {
106:            t.Errorf("confirm_card non-nil on no-ground capture path; want nil")
108:    if env.DisambiguationPrompt != nil {
109:            t.Errorf("disambiguation_prompt non-nil on no-ground capture path; want nil")
116:    if !strings.Contains(body4, "saved as an idea") {
117:            t.Errorf("body = %q; expected canonical 'saved as an idea' acknowledgement (SCOPE-074-04B canonical ack rule)", env.Body)

=== SHA ===
3d695eacccff830ac36e1fdd90ada197bed2643e491f0d2af66dbe0a97192b1d  tests/e2e/assistant/capture_fallback_trigger_e2e_test.go
```

**Reading:** the guard condition is at **98**, its `t.Skipf` body spans
**99–100**. The four SCOPE-074-04B contract assertions are at **103, 106, 109,
117** — all below it. The unconditional assertions are at **83, 86, 89** — all
above it. A second `t.Skipf` sits at **71**.

**Line-number reconciliation with DI-5:** every line number carried forward from
the DI-5 finding was re-verified and is **correct as stated**. No corrections
were required. One precision note, not a correction: DI-5 and the filing brief
refer to the guard as "line 98" and "line 99" respectively; both are right —
`if` at 98, `t.Skipf(` at 99, its argument continuation at 100. Likewise the
header claim is introduced at **16** (`// Adversarial coverage:`) while the
specific false clause — "OR if it routed to a different status … this test would
fail" — spans **18–20**.

---

### Policy anchor

**Claim Source:** executed (current session)
**Command:** `grep -n -A 10 'Adversarial Regression Tests For Bug Fixes' .github/copilot-instructions.md`
**Exit Code:** 0
**Output:**

```text
327:### Adversarial Regression Tests For Bug Fixes
328-
329-- Every bug-fix regression test MUST include at least one adversarial case that would fail if the bug were reintroduced.
330-- Tautological regressions are forbidden: if all fixtures already satisfy the broken filter, gate, or path, the regression cannot detect the bug.
331-- Required tests MUST NOT use bailout returns such as `if (page.url().includes('/login')) { return; }` or equivalent failure-condition early exits.
```

**Reading:** line 331 is the violated rule. The guard at 98–100 exits on a
condition that is an *outcome of the contract under test*, which is the
"equivalent failure-condition early exit" the policy names.

---

### Reference implementation census — the shape the fix should follow

**Claim Source:** executed (current session)
**Command:**

```bash
grep -cE 't\.Errorf|t\.Fatalf' tests/e2e/assistant/high_band_refusal_e2e_test.go
grep -nE 't\.Skip' tests/e2e/assistant/high_band_refusal_e2e_test.go
```

**Exit Code:** 0
**Output:**

```text
=== SIBLING TEST: skip/assert census ===
18
31:// `t.Skipf` keyed on the status it was hunting, so the regression its
36:// without proving something. The only `t.Skip` is for the adapter
90:            t.Skipf("assistant HTTP adapter never bound within 60s (still 503 assistant_http_not_ready) — the route is not up on this run, so no INV-HB-REFUSAL conclusion is available; last body=%s", string(raw))
```

**Reading:** `tests/e2e/assistant/high_band_refusal_e2e_test.go` has **18**
`t.Errorf`/`t.Fatalf` contract assertions and exactly **one** `t.Skip` in
executable code, at **line 90**, guarding the adapter never binding (HTTP 503
`assistant_http_not_ready`) — genuine infrastructure unavailability, not a
contract outcome. The matches at **31** and **36** are prose inside the header
comment explaining why the pattern is avoided. This is the pattern `design.md`
directs the fix to follow.

---

### Refinement of the DI-5 unknown

DI-5 recorded as **not verified** which branch the live stack takes for this
test's fabricated-city prompt. Reading the sibling packet's recorded evidence
**narrows that unknown without closing it**, and the distinction matters enough
to state carefully.

**Claim Source:** executed (current session — grep over committed artifacts);
the underlying test runs are **historical**, executed 2026-07-19/21 by
BUG-074-001, **not re-run here**.

**Command:**

```bash
grep -nE '^\s*---\s+(PASS|SKIP|FAIL)' \
  specs/074-capture-as-fallback-policy/bugs/BUG-074-001-canonical-capture-response/report.md
```

**Exit Code:** 0
**Output (relevant rows):**

```text
166:--- PASS: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (13.58s)
317:--- FAIL: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.75s)
337:--- PASS: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.08s)
```

The RED block at that packet's line 314–318 is decisive:

```text
=== RUN   TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround
    capture_fallback_trigger_e2e_test.go:117: body = "I don't have a sourced answer for that.";
    expected canonical 'saved as an idea' acknowledgement
--- FAIL: TestAssistantHTTPE2E_CaptureFallbackOpenKnowledgeNoGround (0.75s)
```

**What this proves.** The failure is reported **at line 117** — the canonical-body
assertion, which sits *below* the guard. Go emits `--- SKIP:` for a skipped test;
these runs emit `--- FAIL:` and `--- PASS:`. So on those runs the guard at 98 was
**not** taken, meaning the live stack **did** land on `saved_as_idea` for this
prompt, and the below-guard assertions **executed and were capable of failing**.

**Why this strengthens rather than weakens the bug.** It confirms the precise
framing carried from DI-5: the test is **not inert**, and the canonical-ack
*content* rules are genuinely enforced **while the status stays
`saved_as_idea`**. The defect is exactly and only that enforceability is
**contingent on a live routing outcome that has never been pinned**. The moment
that outcome drifts, the test converts silently from FAIL to SKIP — and the
historical evidence shows the assertions are load-bearing, so what would be lost
is real coverage, not a formality.

**Why it does not close the unknown.** This evidence is from **2026-07-19/21**,
at earlier commits, and **before** BUG-061-009 changed band-high refusal
rendering. It says nothing about the branch taken **today at HEAD
`d62f2e75`**. No live run was performed in this session.

---

### Open unknown (recorded, not smoothed over)

**Claim Source:** not-run

It was **NOT verified** which branch the live stack currently takes for the
prompt `"what is the population of the fictional city of Zorthonia-by-the-Sea in
2024?"`. That requires a live run against the disposable stack, which was **not
performed** — this is a filing task and starting the stack was out of scope.

**The defect holds regardless of the answer.** Were that prompt ever to move onto
the honest-refusal branch, the test would report **SKIP rather than FAIL** and
would tell nobody. Resolving the unknown is task 1 of the fix scope
(`scopes.md` → *Implementation Plan*), and it is what decides between the primary
fix and the declared fallback in `design.md`.

---

### Tests executed for a fix

**None.** No fix exists. `scopes.md` declares five test-plan rows
(TP-B074-002-01 … -05); all are **planned and unproven**. No required test was
skipped, because no required test was run.

## Guards & Quality Gates

**Claim Source:** executed (current session) — recorded in the RESULT-ENVELOPE
returned by this task.

- `artifact-lint.sh` was run against this packet directory; its raw exit code and
  output are reported by the filing task.
- `state-transition-guard.sh` is **not** applicable to this filing: the packet is
  not transitioning to a terminal status. Status is `route_required`.

## Discovered Issues (Gate G095)

| # | Date | Issue | Disposition | Reference |
|---|---|---|---|---|
| DI-1 | 2026-08-18 | Spec 074's own DoD for TP-074-14 (`specs/074-capture-as-fallback-policy/scopes.md` line 409) was closed on **Substitute Evidence** with an explicit **Uncertainty Declaration** (line 414) stating the live e2e was blocked by foreign-owned infra and that "a dedicated live e2e re-assertion will be added by the follow-on … spec". The live assertion was later exercised for real by BUG-074-001 (2026-07-19/21), so that gap has been partly discharged in practice — but the test now carrying the assurance is the one this bug is about. | **noted, not routed as a separate bug.** This is context that scopes the present fix, not an independent defect: fixing BUG-074-002 is what makes the re-assertion trustworthy. Recorded so the substitute-evidence history is not silently inherited as if it were a live proof. | `specs/074-capture-as-fallback-policy/scopes.md` lines 404, 409, 414; `design.md` → *Assurance Context Worth Recording* |
| DI-2 | 2026-08-18 | The line-71 `t.Skipf` message pre-judges its own condition, asserting the run is "test-infra timing rather than a SCOPE-074-04B regression" — a conclusion it has not tested. | **folded into this packet's fix scope** as a secondary correction, not filed separately: it is in the same file, the same change boundary, and the same class of defect (a skip that claims more than it proved). | `tests/e2e/assistant/capture_fallback_trigger_e2e_test.go` line 71; `scopes.md` → *Implementation Plan* step 4 |

No other issues surfaced. No sweep for the analogous pattern elsewhere in the
repository was performed; `spec.md` → *Non-Goals* records that as deliberately
out of scope rather than as a completed check.

## Invocation Audit

No subagents were invoked. This task was a filing + root-cause assignment
executed directly by `bubbles.bug` under an explicit instruction not to implement
the fix. The fix is routed via `state.json` → `routing` and
`transitionRequests`, not dispatched from here.
