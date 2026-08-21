# BUG-061-008 — Scopes (P1–P5)

Status: in_progress

Five cohesive scopes: the systemic honest-error fix (P1), its mechanical regression gate
(P2), observability (P3), the deterministic-dispatch pattern (P4), and the encoded invariant
(P5). SCOPE-02 depends on SCOPE-01; the rest are additive.

---

## Scope 1 (P1): Honest execution-error surfacing

**Status:** Done

**Depends on:** none

### Gherkin
```gherkin
Scenario: SCN-061-008-01 — provider error surfaces honestly, never "saved as an idea"
Scenario: SCN-061-008-02 — timeout surfaces honestly, never "saved as an idea"
Scenario: SCN-061-008-03 — OK + no sources still refuses (fabrication guard preserved)
```

### Implementation Files
- `internal/assistant/facade.go`: gate `enforceProvenanceWithSpan` on
  `result != nil && result.Outcome == agent.OutcomeOK`; upgrade `translateFinalToBody`'s
  provider-error/timeout body to a friendly truthful line.
- `internal/assistant/facade_weather_integration_test.go`: update `BS006` to assert the
  honest error contract (`StatusUnavailable`, `ErrProviderUnavailable`, `CaptureRoute=false`,
  no capture body).

Paths above are the non-artifact files this scope changed in the commit of record
`44dc0c94` — see [report.md](report.md) → "Code Diff Evidence" for the per-file delta.

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|-----------|----------|------|-------------|---------|------|
| Unit | `unit` | `internal/assistant/facade_weather_integration_test.go` | `BS006` refined: provider-error → honest `StatusUnavailable`, never saved-as-idea | `./smackerel.sh test unit --go --go-run 'BS006\|AntiFabrication\|ProvenanceGateRewrites'` | No |
| Unit (guard preserved) | `unit` | `internal/assistant/facade_high_band_test.go` + `..._weather_integration_test.go` | `ProvenanceGateRewritesWhenSourcesMissing` + `AntiFabrication` (OK+no-sources) still refuse — GREEN unchanged | same | No |

### Definition of Done
- [x] Provenance gate runs only on `OutcomeOK`; non-OK outcomes surface honest `StatusUnavailable` + `ErrorCause`, never the capture acknowledgement. **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [x] Friendly truthful provider/timeout body (not a bare token). **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [x] `BS006` updated to the honest-error contract; `ProvenanceGateRewritesWhenSourcesMissing` + `AntiFabrication` (OK+no-sources) remain GREEN (fabrication guard intact). **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [ ] SCN-061-008-01 — a provider error surfaces honestly and is never rendered as "saved as an idea": the reply carries `StatusUnavailable` with a non-empty `ErrorCause`, `CaptureRoute=false`, and a body that is not the capture acknowledgement, so the failure reaches the user and alerting instead of being laundered into a benign capture. **UNCHECKED 2026-08-21 by `bubbles.regression`** — the behavior IS delivered and is held by two independent mechanisms (BUG-061-009's band-LOW-only canonicalisation, and the P3 metric test), but the test named as this item's binding does NOT bind it. Mutant M1 re-enables the provenance gate on `OutcomeProviderError` and `TestHighBandNeverMaskedAsSavedAsIdea` still exits 0, because the downstream `canonicalizeSuccessfulCaptureResponse` converts the resulting capture shape back into an honest refusal; the sweep asserts only that `ErrorCause` is non-empty, so a substituted cause is undetected. Owner-routed as D-4 (`bubbles.test`). **Claim Source:** executed. Evidence: [report.md](report.md) → "Discovered Issues" D-4 and `#regression-evidence`.
- [ ] SCN-061-008-02 — a timeout surfaces honestly on exactly the same terms as a provider error and is never rendered as "saved as an idea"; the timeout cause survives to the transport rather than being discarded by the gate. **UNCHECKED 2026-08-21 by `bubbles.regression`** — same unbinding as SCN-061-008-01, and strictly worse: mutant M2 survives for the same downstream reason, and the "timeout cause survives to the transport" clause is additionally FALSE under M2, where the gate substitutes `ErrNoGroundedAnswer` for the true cause and no assertion in the tree detects the substitution. A timeout would reach transport and alerting labelled "no grounded answer". Owner-routed as D-4 (`bubbles.test`). **Claim Source:** executed. Evidence: [report.md](report.md) → "Discovered Issues" D-4 and `#regression-evidence`.
- [x] SCN-061-008-03 — an OK outcome that produced a body with no valid sources still refuses: the anti-fabrication guard is preserved, the uncited body never reaches the user, and the refusal itself reads honestly rather than as a capture. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario binding evidence".
- [x] Build Quality Gate — module compiles + vet clean; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".

---

## Scope 2 (P2): Cross-scenario invariant test (the regression gate)

**Status:** Done

**Depends on:** Scope 1

### Gherkin
```gherkin
Scenario: SCN-061-008-01/02/03 — every requires_provenance scenario × each error outcome is honest; OK+no-sources still refuses
```

### Implementation Files
- `internal/assistant/facade_execution_error_honesty_test.go` (new): table over
  every `requires_provenance` scenario × {OutcomeProviderError, OutcomeTimeout} →
  assert honest surfacing; plus per-scenario OK+no-sources → assert fabrication guard fires.
- `internal/assistant/facade_high_band_invariant_coverage_test.go`: closes the sweep set over
  the scenario SST (`config/assistant/scenarios.yaml`) so the "every scenario" quantifier is
  proven rather than asserted against a hand-maintained list. Landed with BUG-061-009, which
  found `open_knowledge` outside the original hardcoded sweep.

Paths above are the non-artifact files backing this scope — see [report.md](report.md) →
"Code Diff Evidence" for the per-file delta of the commit of record `44dc0c94`.

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|-----------|----------|------|-------------|---------|------|
| Unit (adversarial, table) | `unit` | `internal/assistant/facade_execution_error_honesty_test.go` | every requires_provenance scenario × error outcome → honest, never saved-as-idea; OK+no-sources → still refuses | `./smackerel.sh test unit --go --go-run 'ExecutionErrorHonesty'` | No |

### Definition of Done
- [x] Table-driven invariant test covers all requires_provenance scenarios × {provider-error, timeout} asserting honest surfacing (never `StatusSavedAsIdea`, never capture body, `CaptureRoute=false`). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".
- [x] Complementary OK+no-sources cases assert the fabrication guard still fires (P1 does not over-correct). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".
- [x] SCN-061-008-01/02/03 — the honesty invariant holds for EVERY `requires_provenance` scenario × each error outcome and for OK+no-sources, not only the paths patched by hand; the sweep set is proven closed over the scenario SST, so a scenario added to the manifest cannot silently escape the check and still refuses honestly. **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario binding evidence".
- [x] Adversarial — the test FAILS if the P1 guard is reverted (regression-quality-guard PASS). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".

---

## Scope 3 (P3): Execution-error observability metric

**Status:** Done

**Depends on:** Scope 1

### Implementation Files
- `internal/assistant/metrics/metrics.go`: add `ExecutionErrorSurfacedTotal{scenario_id, outcome, transport}`.
- `internal/assistant/facade.go`: increment it in the high-band non-OK branch.

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|-----------|----------|------|-------------|---------|------|
| Unit | `unit` | `internal/assistant/metrics/*_test.go` (or facade test) | counter increments once per surfaced non-OK outcome, labelled by scenario+outcome | `./smackerel.sh test unit --go --go-run 'ExecutionErrorSurfaced\|ExecutionErrorHonesty'` | No |

### Definition of Done
- [x] `ExecutionErrorSurfacedTotal{scenario_id, outcome, transport}` defined + registered. **Claim Source:** executed. Evidence: [report.md](report.md) → "P3 evidence".
- [x] Incremented exactly once when a non-OK outcome is surfaced; asserted by a unit test. **Claim Source:** executed. Evidence: [report.md](report.md) → "P3 evidence".

---

## Scope 4 (P4): Deterministic-dispatch seam pattern (documented)

**Status:** Done

**Depends on:** none

### Implementation Files
- `docs/smackerel.md`: document the BUG-061-007
  `WithWeatherLookup` seam as the recommended pattern for explicit slash commands with an
  unambiguous tool argument (dispatch directly; never depend on LLM tool-call reliability).

### Definition of Done
- [x] The deterministic-dispatch seam pattern is documented with the `WithWeatherLookup` reference and the "explicit command = deterministic" rule. **Claim Source:** executed. Evidence: [report.md](report.md) → "P4 evidence".

---

## Scope 5 (P5): Invariant encoded + review checklist

**Status:** Done

**Depends on:** Scope 2

### Implementation Files
- `docs/smackerel.md`: add the invariant ("execution errors never rendered as
  capture/soft-refusal; provenance gate runs only on OK outcomes").
- `.github/copilot-instructions.md`: add the review rule, citing the P2 test as mechanical
  enforcement.

### Definition of Done
- [x] The invariant is stated in the assistant design/docs. **Claim Source:** executed. Evidence: [report.md](report.md) → "P5 evidence".
- [x] A review-checklist rule references the invariant + the P2 test as mechanical enforcement. **Claim Source:** executed. Evidence: [report.md](report.md) → "P5 evidence".
