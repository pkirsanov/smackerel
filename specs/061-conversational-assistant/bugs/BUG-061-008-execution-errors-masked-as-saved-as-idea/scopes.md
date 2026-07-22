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

### Implementation
- `internal/assistant/facade.go`: gate `enforceProvenanceWithSpan` on
  `result != nil && result.Outcome == agent.OutcomeOK`; upgrade `translateFinalToBody`'s
  provider-error/timeout body to a friendly truthful line.
- `internal/assistant/facade_weather_integration_test.go`: update `BS006` to assert the
  honest error contract (`StatusUnavailable`, `ErrProviderUnavailable`, `CaptureRoute=false`,
  no capture body).

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|-----------|----------|------|-------------|---------|------|
| Unit | `unit` | `internal/assistant/facade_weather_integration_test.go` | `BS006` refined: provider-error → honest `StatusUnavailable`, never saved-as-idea | `./smackerel.sh test unit --go --go-run 'BS006\|AntiFabrication\|ProvenanceGateRewrites'` | No |
| Unit (guard preserved) | `unit` | `internal/assistant/facade_high_band_test.go` + `..._weather_integration_test.go` | `ProvenanceGateRewritesWhenSourcesMissing` + `AntiFabrication` (OK+no-sources) still refuse — GREEN unchanged | same | No |

### Definition of Done
- [x] Provenance gate runs only on `OutcomeOK`; non-OK outcomes surface honest `StatusUnavailable` + `ErrorCause`, never the capture acknowledgement. **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [x] Friendly truthful provider/timeout body (not a bare token). **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [x] `BS006` updated to the honest-error contract; `ProvenanceGateRewritesWhenSourcesMissing` + `AntiFabrication` (OK+no-sources) remain GREEN (fabrication guard intact). **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".
- [x] Build Quality Gate — module compiles + vet clean; zero warnings. **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence".

---

## Scope 2 (P2): Cross-scenario invariant test (the regression gate)

**Status:** Done

**Depends on:** Scope 1

### Gherkin
```gherkin
Scenario: SCN-061-008-01/02/03 — every requires_provenance scenario × each error outcome is honest; OK+no-sources still refuses
```

### Implementation
- `internal/assistant/facade_execution_error_honesty_test.go` (new): table over
  {weather_query, retrieval_qa, recipe_search} × {OutcomeProviderError, OutcomeTimeout} →
  assert honest surfacing; plus per-scenario OK+no-sources → assert fabrication guard fires.

### Test Plan
| Test Type | Category | File | Description | Command | Live |
|-----------|----------|------|-------------|---------|------|
| Unit (adversarial, table) | `unit` | `internal/assistant/facade_execution_error_honesty_test.go` | every requires_provenance scenario × error outcome → honest, never saved-as-idea; OK+no-sources → still refuses | `./smackerel.sh test unit --go --go-run 'ExecutionErrorHonesty'` | No |

### Definition of Done
- [x] Table-driven invariant test covers all requires_provenance scenarios × {provider-error, timeout} asserting honest surfacing (never `StatusSavedAsIdea`, never capture body, `CaptureRoute=false`). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".
- [x] Complementary OK+no-sources cases assert the fabrication guard still fires (P1 does not over-correct). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".
- [x] Adversarial — the test FAILS if the P1 guard is reverted (regression-quality-guard PASS). **Claim Source:** executed. Evidence: [report.md](report.md) → "P2 evidence".

---

## Scope 3 (P3): Execution-error observability metric

**Status:** Done

**Depends on:** Scope 1

### Implementation
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

### Implementation
- `docs/smackerel.md` (or the assistant design section): document the BUG-061-007
  `WithWeatherLookup` seam as the recommended pattern for explicit slash commands with an
  unambiguous tool argument (dispatch directly; never depend on LLM tool-call reliability).

### Definition of Done
- [x] The deterministic-dispatch seam pattern is documented with the `WithWeatherLookup` reference and the "explicit command = deterministic" rule. **Claim Source:** executed. Evidence: [report.md](report.md) → "P4 evidence".

---

## Scope 5 (P5): Invariant encoded + review checklist

**Status:** Done

**Depends on:** Scope 2

### Implementation
- Assistant design/docs: add the invariant ("execution errors never rendered as
  capture/soft-refusal; provenance gate runs only on OK outcomes").
- `.github/copilot-instructions.md` (or the assistant review checklist): add the review rule,
  citing the P2 test as mechanical enforcement.

### Definition of Done
- [x] The invariant is stated in the assistant design/docs. **Claim Source:** executed. Evidence: [report.md](report.md) → "P5 evidence".
- [x] A review-checklist rule references the invariant + the P2 test as mechanical enforcement. **Claim Source:** executed. Evidence: [report.md](report.md) → "P5 evidence".
