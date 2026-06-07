# BUG-021-010: hospitality guest/property alerts must be LLM-driven (on a reusable judgment foundation)

**Status:** Resolved (LLM-driven hospitality concern judgment + reusable judgment foundation via bugfix-fastlane — see report.md)
**Severity:** Medium
**Reported:** 2026-06-07
**Resolved:** 2026-06-07
**Reporter:** Owner directive — "there should be NO const limits; all should be decided by LLM depending on situation" + "we need best solution long term, but also short term value"
**Owner:** `bubbles.workflow` (parent-expanded bugfix-fastlane; the active runtime lacks `runSubagent`)
**Affected feature:** `specs/021-intelligence-delivery/` (cross-references `specs/013-guesthost-connector/`)
**Affected surface:** `internal/agent/judgment.go` (new foundation), `internal/digest/hospitality.go`, `internal/digest/hospitality_eval.go` (new), `config/prompt_contracts/hospitality-concern-evaluate-v1.yaml` (new)

## Summary

This bug closes the hospitality threshold gap AND introduces the reusable
foundation the owner asked for ("best solution long term, but also short term
value"):

- **Long-term (foundation):** the four prior conversions (BUG-021-005/006/007/008)
  each re-implemented the same `marshal → invoke → validate → decode` plumbing
  for an LLM judgment. `agent.InvokeJudgment[T]` captures that contract once. New
  judgments (and, later, the existing four) route through one primitive.
- **Short-term (value):** the GuestHost digest's guest/property concern alerts —
  previously decided by hardcoded thresholds — are now LLM-judged, the first
  hospitality use of the foundation and the first LLM judgment in the `digest`
  package.

The hardcoded thresholds removed:

- **Guest** (`queryGuestAlerts`): `sentiment_score < 0.3` (low_sentiment),
  `total_stays > 1` (repeat_guest) — decided in SQL.
- **Property** (`queryPropertyAlerts`): `avg_rating < 3.5` (low_rating),
  `issue_count >= 5` (high_issue_count) — decided in SQL.

Whether a guest or property warrants the host's attention is domain reasoning
(docs/smackerel.md §3.6): a 0.31 sentiment is not categorically fine while 0.29
is alarming; a 3.4-star property with one stray review differs from a
chronically soft one; a high-value repeat guest who dipped matters more than a
one-off. Fixed cutoffs cannot weigh that.

## Mechanism (the old, hardcoded path)

`queryGuestAlerts` / `queryPropertyAlerts` put the decision in the SQL `WHERE` +
`CASE`: rows below `0.3` sentiment / `3.5` rating or at/above `5` issues (or with
`>1` stays) became alerts with a templated description; everything else was
dropped. The thresholds were both the candidate filter and the judgment.

## Fix (delivered — reusable foundation + LLM-driven hospitality)

1. **New foundation** `internal/agent/judgment.go`: `InvokeJudgment[T]` (generic)
   + `JudgmentRunner` interface (which `*agent.Bridge` satisfies) +
   `ErrJudgmentUnavailable` sentinel. One marshal/invoke/validate/decode path,
   unit-tested in `internal/agent/judgment_test.go`.
2. **New scenario** `config/prompt_contracts/hospitality-concern-evaluate-v1.yaml`
   (`hospitality_concern_evaluate`): batch input of guest + property signals;
   output of the guest/property alerts the LLM judges worth surfacing (it OMITS
   non-concerns — silence is the default, Product Principle 6).
3. **New evaluator** `internal/digest/hospitality_eval.go`:
   `HospitalityEvaluator` + `BridgeHospitalityEvaluator` (on `InvokeJudgment`),
   `GuestSignal` / `PropertySignal` (internal Email via `json:"-"`),
   `HospitalityDecision`, `HospitalityBounds`, and a `noop_hospitality_concern`
   tool for the loader contract.
4. **`hospitality.go` reworked**: `queryGuestAlerts`/`queryPropertyAlerts`
   (threshold queries) are replaced by `gatherGuestSignals`/`gatherPropertySignals`
   (signal candidates within operational caps) + `assembleConcernAlerts` (LLM
   judges, maps back by `ref`). `AssembleHospitalityContext` takes the evaluator
   + bounds. Nil evaluator ⇒ no concern alerts (no threshold fallback); the rest
   of the digest is unaffected.
5. **Operational bounds → SST** (fail-loud): `digest.hospitality.{guest_candidate_limit,
   property_candidate_limit}` — per-digest candidate-retrieval caps. They do not
   decide concern; the LLM does.
6. **Wiring**: `wireHospitalityEvaluator` (cmd/core) builds the evaluator from
   the bridge + SST and calls `SetHospitalityEvaluator`; `cmd/scenario-lint`
   gains a blank import of `internal/digest` so the new scenario passes BS-010.

## Operational vs business boundary

Per docs/smackerel.md §3.6 + constitution C8: **business reasoning → LLM**;
**operational limits → SST config (fail-loud)**. The "is this a concern?"
JUDGMENT is the LLM's. The remaining numbers (guest/property candidate caps)
bound the job — they do not decide concern.

## Relationship to BUG-021-005/006/007/008/009 and the foundation

This is the fifth threshold conversion and the one that pays down the
duplication debt: it introduces `agent.InvokeJudgment` and is the first native
consumer. The existing four `agent.Bridge` evaluators (cooling, alert-timing,
resurface, expertise) still carry their own copy of the plumbing; migrating them
to `InvokeJudgment` is a behaviour-preserving follow-up (their unit tests guard
it) tracked separately — not bundled here, to keep this packet's blast radius on
the new behaviour.

## Cross-References

- Foundation: `internal/agent/judgment.go`
- Scenario: `config/prompt_contracts/hospitality-concern-evaluate-v1.yaml`
- Evaluator: `internal/digest/hospitality_eval.go`
- Assembly: `internal/digest/hospitality.go`
- Wiring: `cmd/core/wiring_hospitality.go`
- SST loader: `internal/config/hospitality.go`
- Sibling (seasonal): `../BUG-021-009-seasonal-llm-driven/`
- Origin feature: `specs/013-guesthost-connector/`
- Architecture: `docs/smackerel.md` §3.6
