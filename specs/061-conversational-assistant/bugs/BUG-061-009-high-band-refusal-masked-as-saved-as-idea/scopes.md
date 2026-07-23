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

### Definition of Done
- [x] OK-uncited assertion flipped to honest refusal → `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly` (Evidence: report.md#test-evidence)
- [x] One invariant test covers every requires_provenance × high-band-no-sources path → `TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea` sweeps {provider error, timeout, OK-uncited} (Evidence: report.md#test-evidence)
- [x] Reverting any fix layer fails the invariant test (adversarial) → invariant asserts `Status != StatusSavedAsIdea` AND `Body != captureFallbackAcknowledgement` AND `CaptureRoute == false` AND `ErrorCause != ""` per row
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
- Diagnose why open_knowledge grounded nothing for a question about the user's own product (retrieval wiring vs un-ingested docs vs agent search); document the finding in `report.md` and route as a separate follow-up.
- `docs/smackerel.md` §3.8.6: extend the honesty invariant to OK-uncited + "saved as an idea is band-low-only".
- `.github/copilot-instructions.md`: extend the Assistant Response Honesty review rule to INV-HB-REFUSAL.

### Test Plan
| Test Type | Category | File | Description | Command |
|---|---|---|---|---|
| Doc | n/a | `docs/smackerel.md`, `.github/copilot-instructions.md` | invariant documented; grounding follow-up routed | n/a (review) |

### Definition of Done
- [x] Grounding gap diagnosed with evidence + routed as a follow-up (bug/spec id recorded) → `BUG-061-010-open-knowledge-grounding-gap` (Evidence: report.md#grounding-gap-follow-up-scope-05-diagnosis)
- [x] `docs/smackerel.md` §3.8.6 + copilot-instructions encode INV-HB-REFUSAL → §3.8.6 Invariant 3 added; copilot-instructions "Assistant Response Honesty" INV-HB-REFUSAL bullet added
- [x] artifact-lint clean → (Evidence: report.md#artifact-lint)
