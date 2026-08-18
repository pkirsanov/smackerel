# BUG-061-009 — Scopes

Enforces **INV-HB-REFUSAL** (spec.md): a band-high turn never renders the
capture acknowledgement nor the "(saved as idea)" suffix.

---

## SCOPE-01 — Provenance gate refuses honestly (not as a capture)

**Status:** Done
**Depends On:** none

### Gherkin
- SCN-061-009-01 — open_knowledge OK-but-uncited answer refuses honestly.

### Implementation
- `contracts/response.go`: add `ErrNoGroundedAnswer ErrorCause = "no_grounded_answer"` + `AllErrorCauses`.
- `provenance/gate.go` `Enforce`: refusal shape → `Status=StatusUnavailable`, `ErrorCause=ErrNoGroundedAnswer`, `Body=CanonicalRefusalBody` (unchanged), `CaptureRoute=false`, `Sources=nil`.
- `gate_test.go`: update the rewrite assertions to the honest-refusal shape; keep the passthrough + empty-body cases.

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Unit | unit | `internal/assistant/provenance/gate_test.go` | refusal → StatusUnavailable + ErrNoGroundedAnswer + honest body + CaptureRoute=false; passthrough unchanged | `./smackerel.sh test unit --go` |
| Unit | unit | `internal/assistant/contracts/response_test.go` | ErrNoGroundedAnswer in closed vocabulary | `./smackerel.sh test unit --go` |

### Definition of Done
- [x] Gate refuses into the honest `StatusUnavailable` + `ErrNoGroundedAnswer` shape for every requires_provenance scenario → `provenance/gate.go` `Enforce` (Evidence: report.md#test-evidence)
- [x] `gate_test.go` + `response_test.go` pass with the new shape → `internal/assistant/provenance ok` + `internal/assistant/contracts ok` (Evidence: report.md#test-evidence second run, exit 0)
- [x] Build Quality Gate: build + `check` + lint clean, zero warnings → `check` OK; `lint` "All checks passed!" (Evidence: report.md#test-evidence)

---

## SCOPE-02 — Facade canonicalize scoped to band-low

**Status:** Done
**Depends On:** SCOPE-01

### Gherkin
- SCN-061-009-02 — no band-high requires_provenance path renders the capture ack.
- SCN-061-009-03 — band-low unrouted input still captures as an idea.

### Implementation
- `facade.go`: pass `band` into `canonicalizeSuccessfulCaptureResponse`; apply the capture ack ONLY for `BandLow`. For a residual band-high `StatusSavedAsIdea`, convert to the honest refusal (`StatusUnavailable` + `ErrNoGroundedAnswer` + honest body).
- `facade_open_knowledge_no_ground_test.go`: update `TestCanonicalizeSuccessfulCaptureResponse_*` for the band param (band-low flattens; band-high stays honest).

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Unit | unit | `internal/assistant/facade_open_knowledge_no_ground_test.go` | band-low flattens to capture ack; band-high preserves honest refusal | `./smackerel.sh test unit --go` |

### Definition of Done
- [x] Canonicalize applies capture ack for band-low only; band-high never gets the capture ack → `facade.go` `canonicalizeSuccessfulCaptureResponse(resp, band, …)` + band-high defense-in-depth test (Evidence: report.md#test-evidence)
- [x] Band-low unrouted capture is byte-for-byte unchanged → `facade_capture_fallback_test.go ok`, WhatsApp/Telegram capture goldens unchanged (Evidence: report.md#test-evidence)
- [x] Build Quality Gate: build + check + lint clean, zero warnings → (Evidence: report.md#test-evidence)

---

## SCOPE-03 — Cross-path invariant test (class-killer)

**Status:** Done
**Depends On:** SCOPE-01, SCOPE-02

### Gherkin
- SCN-061-009-02 — no band-high requires_provenance path renders the capture ack.

### Implementation
- `facade_execution_error_honesty_test.go`: FLIP `TestExecutionErrorHonesty_OKNoSourcesStillRefuses` → assert `StatusUnavailable` + non-empty `ErrorCause` + body ≠ `captureFallbackAcknowledgement`. EXTEND the invariant table so one test covers every requires_provenance scenario × {provider error, timeout, OK-uncited} → never `StatusSavedAsIdea`, never the capture ack. Keep a band-low case asserting the legitimate capture ack.

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Unit | unit | `internal/assistant/facade_execution_error_honesty_test.go` | cross-path invariant: band-high never masked; band-low capture preserved | `./smackerel.sh test unit --go` |
| Unit | unit | `internal/assistant/facade_execution_error_honesty_test.go` | regression: `requiresProvenanceScenarios` gains `open_knowledge`, so the INV-HB-REFUSAL sweep actually covers the `/ask` scenario this bug was reported against; no live system | `./smackerel.sh test unit` |
| Unit | unit | `internal/assistant/facade_high_band_invariant_coverage_test.go` | regression: `TestRequiresProvenanceScenarios_ClosedOverSST` closes the sweep list over `config/assistant/scenarios.yaml` — fails on drift in both directions, with an anti-vacuity `t.Fatal`; no live system | `./smackerel.sh test unit` |

### Definition of Done
- [x] OK-uncited assertion flipped to honest refusal → `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly` (Evidence: report.md#test-evidence)
- [x] One invariant test covers every requires_provenance × high-band-no-sources path → `TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea` sweeps {provider error, timeout, OK-uncited} (Evidence: report.md#test-evidence)
- [x] Reverting any fix layer fails the invariant test (adversarial) → invariant asserts `Status != StatusSavedAsIdea` AND `Body != captureFallbackAcknowledgement` AND `CaptureRoute == false` AND `ErrorCause != ""` per row
- [x] Scenario-specific regression coverage exists for the reported `open_knowledge` masking path — `requiresProvenanceScenarios` now contains `open_knowledge`, so the INV-HB-REFUSAL sweep exercises the exact `/ask` scenario the bug was filed against instead of skipping it → Evidence: [report.md#regression-invariant-closure]
- [x] Broader regression suite passes with the coverage-closure test in place — `TestRequiresProvenanceScenarios_ClosedOverSST` binds the sweep list to `config/assistant/scenarios.yaml` and the full unit suite is green → Evidence: [report.md#regression-invariant-closure]
- [x] Build Quality Gate: build + check + lint clean, zero warnings → (Evidence: report.md#test-evidence)

---

## SCOPE-04 — Adapter honest render + structural distinguishability

**Status:** Done
**Depends On:** SCOPE-01

### Gherkin
- SCN-061-009-01 — honest body rendered (not "<skill>: <cause>", not capture ack).
- SCN-061-009-04 — typed refusal cause renders an honest headline, no "(saved as idea)".

### Implementation
- Telegram `render_outbound.go`: render `resp.Body` verbatim for `ErrNoGroundedAnswer` (fast-path like `ErrModelNotSwitchable`).
- Telegram `render_openknowledge.go`: `RenderRefusalWithCapture` drops `OpenKnowledgeCaptureSuffix` (returns `CanonicalRefusalBodyFor(cause)` only); rename to `RenderRefusal`.
- `contracts/refusal.go`: reword the 5 cause strings to drop "— saved as an idea".
- Update `render_openknowledge_test.go` / `render_outbound_test.go` / `substrate_tool_test.go` / `render_outbound_test.go` G021 assertions to STRUCTURAL (StatusUnavailable/ErrorCause + no citations) instead of the "(saved as idea)" string.
- WhatsApp `assistant_adapter`: mirror the honest refusal render for `ErrNoGroundedAnswer`; band-low capture ack unchanged.

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Unit | unit | `internal/telegram/assistant_adapter/render_openknowledge_test.go` | refusal renders honest headline, no "(saved as idea)"; sourced answer structurally distinct | `./smackerel.sh test unit --go` |
| Unit | unit | `internal/telegram/assistant_adapter/render_outbound_test.go` | ErrNoGroundedAnswer → friendly body verbatim | `./smackerel.sh test unit --go` |
| Unit | unit | `internal/whatsapp/assistant_adapter/*_test.go` | honest refusal mirrored; band-low ack unchanged | `./smackerel.sh test unit --go` |

### Definition of Done
- [x] Gate refusal renders "I don't have a sourced answer for that." (no "<skill>:" prefix, no capture ack) → `render_outbound.go` `ErrNoGroundedAnswer` verbatim fast-path; WhatsApp `StatusUnavailable` renders `Body` verbatim (Evidence: report.md#test-evidence)
- [x] Typed refusal cause renders honest headline with no "(saved as idea)" → `RenderRefusal` dropped the suffix; 5 cause strings reworded; `contracts/refusal_test.go` asserts no "saved as an idea" substring (Evidence: report.md#test-evidence)
- [x] Refusal-vs-answer distinguishability asserted structurally → G021 assertions now on `Status`/`ErrorCause` + citation presence (Evidence: report.md#test-evidence)
- [x] Telegram + WhatsApp both honest; band-low ack unchanged → `internal/telegram/assistant_adapter ok`, `internal/whatsapp/assistant_adapter ok` (Evidence: report.md#test-evidence)
- [x] Build Quality Gate: build + check + lint clean, zero warnings → (Evidence: report.md#test-evidence)

---

## SCOPE-05 — Grounding-gap diagnosis + docs/invariant encoding

**Status:** Done
**Depends On:** SCOPE-01..04

### Gherkin
- SCN-061-009-05 — grounding gap diagnosed and routed (no code fix here).

### Implementation
- Diagnose why open_knowledge grounded nothing for a question about the user's own product (retrieval wiring vs un-ingested docs vs agent search); document the finding in `report.md` and route the grounding gap to its own bug artifact.
- `docs/smackerel.md` §3.8.6: extend the honesty invariant to OK-uncited + "saved as an idea is band-low-only".
- `.github/copilot-instructions.md`: extend the Assistant Response Honesty review rule to INV-HB-REFUSAL.

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Doc | n/a | `docs/smackerel.md`, `.github/copilot-instructions.md` | invariant documented; grounding gap routed to `BUG-061-010-open-knowledge-grounding-gap` | n/a (review) |

### Definition of Done
- [x] Grounding gap diagnosed with evidence + routed to a real bug artifact whose id is recorded → `BUG-061-010-open-knowledge-grounding-gap`, which exists at `specs/061-conversational-assistant/bugs/BUG-061-010-open-knowledge-grounding-gap/` and owns the grounding gap (Evidence: report.md#grounding-gap-diagnosis-and-routing-scope-05)
- [x] `docs/smackerel.md` §3.8.6 + copilot-instructions encode INV-HB-REFUSAL → §3.8.6 Invariant 3 added; copilot-instructions "Assistant Response Honesty" INV-HB-REFUSAL bullet added
- [x] artifact-lint clean → (Evidence: report.md#artifact-lint)

---

## Packet-Wide Regression E2E Coverage — NOT DELIVERED

**This is not a scope.** It carries no `Status:` line and adds nothing to the
packet's scope accounting. It exists because guard Check 8A asks a packet-wide
*planning* question — is persistent scenario-specific regression E2E coverage
planned? — and the honest answer for BUG-061-009 is **"planned, not delivered."**
Recording that truthfully is the whole point. Recording it as delivered would
reproduce the exact failure mode this bug exists to fix.

### What was actually delivered

Every proof this packet cites is an **in-package Go unit test**:
`internal/assistant/provenance/gate_test.go`,
`internal/assistant/facade_execution_error_honesty_test.go`,
`internal/assistant/facade_high_band_invariant_coverage_test.go`,
`internal/assistant/contracts/refusal_test.go`, and the Telegram/WhatsApp adapter
render suites. Those are real, they assert the honest-refusal shape, and they
pass. They are **not** E2E — they construct responses in-process and never cross
a wire.

### What does not exist

No test under `<repo-root>/tests/` asserts the honest-refusal **wire** contract
this packet introduced — `StatusUnavailable` + `no_grounded_answer` + the
canonical body — against a live stack. Verified 2026-08-18 by searching the whole
`tests/` tree for `ErrNoGroundedAnswer`, `no_grounded_answer`, `StatusUnavailable`,
and the canonical body string `I don't have a sourced answer for that.`:

- `tests/integration/openknowledge/self_knowledge_provenance_test.go` *names*
  "BUG-061-009 INV-HB-REFUSAL" in a header comment, but it exercises the cite-back
  verifier (`citeback.Verify` → `ReasonNotInTrace`). It never builds a facade turn
  and never asserts a status, an error cause, or a body. It is spec-104-owned and
  is not coverage of this contract. A comment naming an invariant is not a test of
  it.
- `tests/e2e/agent/openknowledge_e2e_test.go` asserts the **agent loop's**
  `status:"refused"` — a different layer from the facade envelope this packet
  changed.

The two DoD items below therefore stay `[ ]`.

### These two unchecked items are blocking — measured, not assumed

Check 8A's regexes deliberately accept `[ ]`, so adding these entries turns Check
8A green while zero E2E coverage exists. Check 8A asks whether the coverage is
*planned*, not whether it was *delivered*. **A green Check 8A on this packet is
not evidence of E2E coverage.**

What stops that from being a loophole is Check 4. It picks its completion basis
from `scenario-manifest.json`, using receipt-derived scenario states only when at
least one scenario carries a canonical `id`. This packet's manifest declares five
scenarios and **none carries an `id`**, so Check 4 falls back to the legacy
checkbox basis, where every unchecked DoD item blocks `done`.

Measured on 2026-08-18, both runs captured in `report.md`: adding the two items
below moved the guard from `failureCount: 5, failedChecks: []` to `failureCount:
2, failedChecks: [Check-4-completion]`. Four Check 8A *planning-shape* failures
cleared and one real *completion* failure took their place. The number fell; the
packet did not move closer to done. It is now blocked on the true reason — the
absence of E2E coverage — instead of on the shape of the plan.

Do not clear `Check-4-completion` by ticking these boxes. The only honest way to
tick them is to write the E2E described below and run it.

### Test Plan

| Test Type | Category | File | Description | Command | Delivery status |
|---|---|---|---|---|---|
| Regression E2E | e2e-api | *(none — not written)* | Regression: drive a live `POST /api/assistant/turn` with a band-high `requires_provenance` turn that grounds nothing, then assert the wire envelope is `status=unavailable` + `error_cause=no_grounded_answer` + the canonical refusal body, with `capture_route=false` and no `saved as an idea` substring — SCN-061-009-01/02 asserted over the wire instead of in-process | `./smackerel.sh test e2e` | **NOT DELIVERED** — no such file exists; nothing is claimed |

### Definition of Done

- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — NOT DELIVERED. INV-HB-REFUSAL is proven only by in-package Go unit tests; no live-stack test asserts the refusal envelope. Left unchecked deliberately.
- [ ] Broader E2E regression suite passes — NOT DELIVERED. `./smackerel.sh test e2e` was not run for this packet and no result is claimed. Left unchecked deliberately.

### Whoever writes this E2E: it must be able to fail

An E2E that exists is not automatically coverage. See **DI-5** in `report.md` →
*Discovered Issues*: a live E2E in this repository places every assertion below a
status-conditional `t.Skipf`, so the regression it advertises in its own header
cannot fail it. The E2E that closes the two items above MUST assert the refusal
envelope **unconditionally** — an unexpected status is the failure being hunted,
so it must fail the test rather than skip it.
