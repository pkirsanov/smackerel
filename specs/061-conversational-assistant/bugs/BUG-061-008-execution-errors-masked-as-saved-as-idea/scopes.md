# BUG-061-008 — Scopes (P1–P5)

Status: in_progress

Five cohesive scopes: the systemic honest-error fix (P1), its mechanical regression gate
(P2), observability (P3), the deterministic-dispatch pattern (P4), and the encoded invariant
(P5). SCOPE-02 depends on SCOPE-01; the rest are additive.

---

## Scope 1 (P1): Honest execution-error surfacing

**Status:** Done

Returned to In Progress on 2026-08-21 because two DoD items (SCN-061-008-01 and SCN-061-008-02)
were unchecked under finding D-4 once mutation showed the test named as their binding does not
bind them. Returned to Done on 2026-08-22 by `bubbles.test`: the remedy landed in commit
`f3b80e22`, mutants M1 and M2 were re-run and both now exit 1 (KILLED, from 0/SURVIVED), the
unmutated tree stays green, and `facade.go` was restored byte-identically. The delivered
behavior never changed — only the attribution is now earned. Zero unchecked DoD items.

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
- [x] SCN-061-008-01 — a provider error surfaces honestly and is never rendered as "saved as an idea": the reply carries `StatusUnavailable` with `ErrorCause = provider_unavailable`, `CaptureRoute=false`, and a body that is not the capture acknowledgement, so the failure reaches the user and alerting instead of being laundered into a benign capture. **UNCHECKED 2026-08-21 by `bubbles.regression`** — the behavior was delivered, but the test named as this item's binding did not bind it: mutant M1 re-enables the provenance gate on `OutcomeProviderError` and `TestHighBandNeverMaskedAsSavedAsIdea` still exited 0, because the downstream `canonicalizeSuccessfulCaptureResponse` converts the resulting capture shape back into an honest refusal and the sweep asserted only that `ErrorCause` was non-empty. **RE-CHECKED 2026-08-22 by `bubbles.test` (commit `f3b80e22`, D-4 closed)** — each row now asserts its exact expected cause, so M1 is KILLED. **Claim Source:** executed. Evidence: [report.md](report.md) → `#d4-remediation`, and inline below.

  ```text
  # M1 applied at facade.go:1367 — (Outcome == OutcomeOK || Outcome == OutcomeProviderError)
  $ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go \
      --go-run 'TestHighBandNeverMaskedAsSavedAsIdea'
  M1_BEFORE_EXIT=0          # pre-fix test: SURVIVED
  M1_AFTER_EXIT=1           # strengthened test: KILLED
  --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea (0.01s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/provider-error (0.00s)
          facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band provider-error. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/provider-error (0.00s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/provider-error (0.00s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/provider-error (0.01s)
  FAIL	github.com/smackerel/smackerel/internal/assistant	0.585s
  # observable surface during the same run — the mislabel, with its own control:
  assistant_turn user_id=u-weather_query/provider-error ... error_cause=no_grounded_answer ... outcome=provider-error
  assistant_turn user_id=u-weather_query/timeout        ... error_cause=provider_unavailable ... outcome=timeout
  # restored tree (facade.go byte-identical 139510ff…), strengthened test:
  RESTORED_FULL_EXIT=0      # full Go unit suite, ok_packages=148, no FAIL lines
  ```

- [x] SCN-061-008-02 — a timeout surfaces honestly on exactly the same terms as a provider error and is never rendered as "saved as an idea"; the timeout cause survives to the transport rather than being discarded by the gate. **UNCHECKED 2026-08-21 by `bubbles.regression`** — same unbinding as SCN-061-008-01 and strictly worse: mutant M2 survived for the same downstream reason, and the "cause survives to the transport" clause was additionally FALSE under M2, where the gate substitutes `ErrNoGroundedAnswer` for the true cause with no assertion detecting it. **RE-CHECKED 2026-08-22 by `bubbles.test` (commit `f3b80e22`, D-4 closed)** — the row now asserts `provider_unavailable` explicitly, so M2 is KILLED and the surviving-cause clause is enforced. Note the closed vocabulary has no timeout-specific member: `translateOutcomeToErrorCause` maps `OutcomeTimeout` and `OutcomeProviderError` both to `ErrProviderUnavailable`, so that is the cause this row requires. **Claim Source:** executed. Evidence: [report.md](report.md) → `#d4-remediation`, and inline below.

  ```text
  # M2 applied at facade.go:1367 — (Outcome == OutcomeOK || Outcome == OutcomeTimeout)
  $ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go \
      --go-run 'TestHighBandNeverMaskedAsSavedAsIdea'
  M2_BEFORE_EXIT=0          # pre-fix test: SURVIVED
  M2_AFTER_EXIT=1           # strengthened test: KILLED
  --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea (0.00s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/timeout (0.00s)
          facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band timeout. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/timeout (0.00s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/timeout (0.00s)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/timeout (0.00s)
  FAIL	github.com/smackerel/smackerel/internal/assistant	0.543s
  # the clause itself, on the observable surface — a timeout labelled "no grounded answer":
  assistant_turn user_id=u-weather_query/timeout        ... error_cause=no_grounded_answer ... outcome=timeout
  assistant_turn user_id=u-weather_query/provider-error ... error_cause=provider_unavailable ... outcome=provider-error
  # restored tree (facade.go byte-identical 139510ff…), strengthened test:
  RESTORED_FULL_EXIT=0      # full Go unit suite, ok_packages=148, no FAIL lines
  ```
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
