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
- [x] Provenance gate runs only on `OutcomeOK`; non-OK outcomes surface honest `StatusUnavailable` + `ErrorCause`, never the capture acknowledgement. Bounded 2026-08-22 by `bubbles.audit` (finding D-6) so the second clause is not read wider than the code: `StatusUnavailable`, `CaptureRoute=false` and never-the-capture-body hold for all ten non-OK outcomes through the one guarded branch at `facade.go:1368`; a non-empty `ErrorCause` holds for the two that `translateOutcomeToErrorCause` maps, which are the two this item's evidence exercises. The gate guard itself — the deliverable — is verified present and is unaffected. **Claim Source:** executed. Evidence: [report.md](report.md) → "P1 evidence" and `#audit-evidence`.
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

Returned to In Progress on 2026-08-22 by `bubbles.audit` under finding D-6: the matrix DoD item
below was unchecked because its outcome-axis quantifier ("each error outcome") is not merely
unproven by the sweep — it is contradicted by the production mapping. The three other items in
this scope, and the delivered behavior, are unaffected.

Updated 2026-08-22 by `bubbles.plan`: the spec question blocking that item is answered — the four
unmapped terminal outcomes resolve to `contracts.ErrInternalError` (see [design.md](design.md) →
"Decision — per-outcome `ErrorCause` for terminal non-OK outcomes"). The item stays unchecked and
its wording stays as written; it is re-earned by code plus a test, not by an artifact edit. Scope
remains In Progress.

Returned to Done on 2026-08-22 by `bubbles.implement`: the answered decision was implemented in
code (`translateOutcomeToErrorCause` gained the `ErrInternalError` arm for the four terminal
outcomes) and the sweep was widened to all six terminal outcomes with its cause map still
hand-written. The item's wording was NOT touched — it was re-earned exactly as `bubbles.plan`
required. Zero DoD items in this scope are now unchecked, and a two-mutant campaign proves the
widened sweep binds rather than merely passes (raw evidence inline on the item below).

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
- [x] SCN-061-008-01/02/03 — the honesty invariant holds for EVERY `requires_provenance` scenario × each error outcome and for OK+no-sources, not only the paths patched by hand; the sweep set is proven closed over the scenario SST, so a scenario added to the manifest cannot silently escape the check and still refuses honestly. **UNCHECKED 2026-08-22 by `bubbles.audit`** — the scenario half of the quantifier is genuinely earned (`TestRequiresProvenanceScenarios_ClosedOverSST` reads `config/assistant/scenarios.yaml` and fails in both directions). The outcome half is not, and the cited evidence section never addresses that axis. Read this turn rather than taken from the report: `translateOutcomeToErrorCause` (`internal/assistant/facade.go:1798-1804`) returns `ErrProviderUnavailable` for `OutcomeProviderError` and `OutcomeTimeout` and `contracts.ErrNone` for everything else; `ErrNone ErrorCause = ""` (`internal/assistant/contracts/response.go:194`); `internal/agent/executor.go:59-88` declares 10 non-OK outcomes. So for the other 8 the `ErrorCause` half of the honesty invariant — which `spec.md` P1 states as "`Status=StatusUnavailable` + a set `ErrorCause` + a truthful body" — does not hold. `D-5` frames this as an unproven cause; it is stronger than that, and a checked item may not assert a universal its own code contradicts. The `StatusUnavailable` / `CaptureRoute=false` / never-capture-body half DOES hold for all ten, via the single `result.Outcome != OutcomeOK` branch at `facade.go:1368`. **Remedy for the owner:** the `D-5` spec question is **ANSWERED 2026-08-22 by `bubbles.plan`** — a schema failure, an invalid tool return, an input-schema violation and a loop limit each owe the transport a cause, and that cause is the existing `contracts.ErrInternalError`; `ErrNone` is not correct for them. The count in this item is also corrected: the mapping gap is **four**, not eight, because `OutcomeAllowlistViolation` / `OutcomeHallucinatedTool` / `OutcomeToolError` are per-`ExecutedToolCall` records the loop continues past and `OutcomeUnknownIntent` is emitted only by `Bridge.Invoke`, which never reaches the facade mapping. What re-earns this check is now purely mechanical and owned by `bubbles.implement`: add the `ErrInternalError` arm to `translateOutcomeToErrorCause`, then widen `errorOutcomes` and `errorOutcomeCauses` to the six terminal outcomes. The narrowing option this remedy previously offered is **withdrawn** — rewording the item to match current delivery is the G068 anti-pattern. Rationale: [design.md](design.md) → "Decision — per-outcome `ErrorCause` for terminal non-OK outcomes". **Claim Source:** executed. Evidence: [report.md](report.md) → "Scenario binding evidence" and `#audit-evidence`. **RE-CHECKED 2026-08-22 by `bubbles.implement`** — the remedy above was executed exactly as written, with no rewording of this item's claim; commit of record `7e85e076`. `translateOutcomeToErrorCause` now maps the four terminal outcomes `OutcomeToolReturnInvalid` / `OutcomeSchemaFailure` / `OutcomeLoopLimit` / `OutcomeInputSchemaViolation` to `contracts.ErrInternalError`, and `errorOutcomes` + `errorOutcomeCauses` were widened to all six terminal outcomes with the cause map still hand-written (deriving it from the production mapping would assert that mapping against itself — the D-4 tautology). The three per-`ExecutedToolCall` outcomes and router-emitted `OutcomeUnknownIntent` are deliberately excluded, re-derived this turn from the `terminal:` / "§5.1 loop continues" comments on the `Outcome` constants in `internal/agent/executor.go:52-89` rather than taken from the prior annotation. Both quantifier axes are now earned: the scenario axis by `TestRequiresProvenanceScenarios_ClosedOverSST`, and the outcome axis by the widened sweep binding each row's exact cause. The sweep is proven to BIND rather than merely pass, by a two-mutant campaign in which each mutant dropped one newly-mapped outcome so it fell through to `ErrNone`; both were KILLED at exit 1 across all four scenarios, and the tree was then proven byte-identical to its pre-mutation blobs. `SMACKEREL_SKIP_HOST_PREFLIGHT=1` was set for every run and is disclosed here: the disk preflight refuses at ~35 GB free against a 40 GB floor, the opt-out is the documented one at `smackerel.sh:715`, and `test unit --go` builds no image; no shared cache was pruned. **Claim Source:** executed. Raw evidence inline below.

  ```text
  $ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go   # widened sweep, restored tree
  ok      github.com/smackerel/smackerel/internal/assistant       0.271s
  [go-unit] go test ./... finished OK
  UNIT_EXIT=0

  # MUTANT A — drop OutcomeSchemaFailure from the ErrInternalError arm (falls through to default ErrNone)
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/schema-failure
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/schema-failure
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/schema-failure
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/schema-failure
          facade_execution_error_honesty_test.go:189: ErrorCause = ""; want "internal_error" for a high-band schema-failure.
  FAIL    github.com/smackerel/smackerel/internal/assistant       0.278s
  MUTANT_A_EXIT=1

  # MUTANT B — drop OutcomeLoopLimit from the ErrInternalError arm
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/loop-limit
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/loop-limit
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/loop-limit
      --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/loop-limit
          facade_execution_error_honesty_test.go:189: ErrorCause = ""; want "internal_error" for a high-band loop-limit.
  FAIL    github.com/smackerel/smackerel/internal/assistant       0.298s
  MUTANT_B_EXIT=1

  # RESTORATION — byte-identity against the pre-mutation blobs
  $ git hash-object internal/assistant/facade.go internal/assistant/facade_execution_error_honesty_test.go
  b93320457a874ecc5a643a7e3a907b9c91b3695d   (expected b93320457a874ecc5a643a7e3a907b9c91b3695d)
  6230ce733c6b73a0dccb3e8bc109b57af3a6dbe2   (expected 6230ce733c6b73a0dccb3e8bc109b57af3a6dbe2)
  $ git status --porcelain
   M internal/assistant/facade.go
   M internal/assistant/facade_execution_error_honesty_test.go
  $ SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go
  ok      github.com/smackerel/smackerel/internal/assistant       (cached)
  [go-unit] go test ./... finished OK
  RESTORED_EXIT=0
  ```

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
