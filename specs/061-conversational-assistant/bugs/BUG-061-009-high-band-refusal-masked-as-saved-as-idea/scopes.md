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

## Packet-Wide Regression E2E Coverage

**This is not a scope.** It carries no `Status:` line and adds nothing to the
packet's scope accounting. It exists because guard Check 8A asks a packet-wide
question — is persistent scenario-specific regression E2E coverage present? — and
this section is where BUG-061-009 answers it.

**History, kept deliberately.** This section was created reading *"NOT
DELIVERED"*, because that was the truth: every proof the packet cited was an
in-package Go unit test and nothing asserted the refusal contract over a wire.
The E2E has since been written and run, so the section now records delivery. The
earlier state is preserved here rather than erased, because the reason the gap
was recorded honestly is the same reason this bug exists.

### What is delivered

`tests/e2e/assistant/high_band_refusal_e2e_test.go` —
`TestAssistantHTTPE2E_HighBandUncitedRefusesHonestly`. It drives the live
chi-mounted `POST /api/assistant/turn` route with a band-HIGH `/ask` turn and
asserts the envelope a real client receives. Evidence:
[report.md#check-8a-live-e2e](report.md#check-8a-live-e2e).

The unit-level proofs remain what they always were and are not superseded:
`internal/assistant/provenance/gate_test.go`,
`internal/assistant/facade_execution_error_honesty_test.go`,
`internal/assistant/facade_high_band_invariant_coverage_test.go`,
`internal/assistant/contracts/refusal_test.go`, and the Telegram/WhatsApp adapter
render suites. Those construct responses in-process; the new test is the only one
that crosses a transport boundary.

### What the delivered run proves — and what it does not

**Read this before citing the E2E.** The passing run observed
`error_cause="provider_unavailable"`, **not** `no_grounded_answer`. It therefore
exercised the **provider-outage** branch of INV-HB-REFUSAL, not the
**OK-but-uncited** branch that BUG-061-009 was specifically filed about.

- **Proven on the wire:** a band-HIGH turn did not render the capture
  acknowledgement; `capture_route=false`; the body carried no `saved as an idea`
  substring; the refusal carried a typed cause inside the closed
  `contracts.AllErrorCauses` vocabulary.
- **Not proven on the wire this run:** the `no_grounded_answer` rewrite
  end-to-end, and the bidirectional canonical-body ↔ cause binding (both
  conditionals were vacuously true because neither side was present). Those stay
  proven at the unit level only.

The test accepts either honest branch by design — demanding one specific cause
would make it flaky rather than stronger — which is exactly why the evidence must
name the branch that actually fired. Full analysis:
[report.md#check-8a-branch-nuance](report.md#check-8a-branch-nuance).

### Test Plan

| Test Type | Category | File | Description | Command | Delivery status |
|---|---|---|---|---|---|
| Regression E2E | e2e-api | `tests/e2e/assistant/high_band_refusal_e2e_test.go` | Regression: drive a live `POST /api/assistant/turn` with a band-high `requires_provenance` turn that grounds nothing, then assert the wire envelope is an honest refusal — `capture_route=false`, no `saved as an idea` substring, never `saved_as_idea`, and a typed cause from `contracts.AllErrorCauses` — with the `status=unavailable` + `error_cause=no_grounded_answer` + canonical-body binding asserted bidirectionally whenever either side is present. SCN-061-009-01/02 asserted over the wire instead of in-process | `./smackerel.sh test e2e --go-package assistant --go-run 'TestAssistantHTTPE2E_HighBandUncitedRefusesHonestly'` | **DELIVERED** — exit 0, `--- PASS` ([report.md#check-8a-focused-run](report.md#check-8a-focused-run)); observed branch was `provider_unavailable`, so the `no_grounded_answer` path is **not** covered end-to-end ([report.md#check-8a-branch-nuance](report.md#check-8a-branch-nuance)) |

### Definition of Done

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior — `tests/e2e/assistant/high_band_refusal_e2e_test.go` (`TestAssistantHTTPE2E_HighBandUncitedRefusesHonestly`), exit 0, `--- PASS` (Evidence: [report.md#check-8a-focused-run](report.md#check-8a-focused-run)). **Scoped claim, not a blanket one:** the observed envelope was `status=unavailable error_cause=provider_unavailable capture_route=false sources=0`, so the run exercised the PROVIDER-OUTAGE branch — **not** the OK-but-uncited `no_grounded_answer` branch this bug was filed about. What is proven on the wire is INV-HB-REFUSAL itself: no capture acknowledgement on a band-high turn, and a typed cause from the closed `contracts.AllErrorCauses` vocabulary. The `no_grounded_answer` rewrite remains proven at the unit level only. Checking this box does not retire that distinction — see *What the delivered run proves — and what it does not* above and [report.md#check-8a-branch-nuance](report.md#check-8a-branch-nuance), both of which stay accurate as written.
- [x] Broader E2E regression suite passes — `./smackerel.sh test e2e` exit 0, 4703 lines, `sha256 69f65d1cc9…`, 2283s wall (Evidence: [report.md#check-8a-broader-suite](report.md#check-8a-broader-suite)). Exactly one failure-shaped line appears inside the omitted region and did **not** fail the run: it is the required success output of the negative-path readiness canary `SCN-002-BUG-002-001`, attributed from source in that section rather than glossed.

### This test can fail — verified, not asserted

An E2E that exists is not automatically coverage. See **DI-5** in `report.md` →
*Discovered Issues*: a sibling live E2E in this repository places every assertion
below a status-conditional `t.Skipf`, so the regression it advertises in its own
header cannot fail it. The test delivered here does not repeat that. Its only
`t.Skip` in executable code is at line 90, guarding HTTP 503
`assistant_http_not_ready` — the adapter genuinely not being bound, which is
infrastructure availability rather than an outcome of the contract. All 18
contract assertions use `t.Errorf`/`t.Fatalf`, so a wrong status **fails** the
test instead of skipping it. Scan recorded at
[report.md#check-8a-bailout-scan](report.md#check-8a-bailout-scan).
