# Design: Spec 055 state.json done_with_concerns Schema Compliance

## Suspected Root Cause

The promotion of spec 055 to `status: done_with_concerns` was executed by `bubbles.validate` in the 2026-05-24T22:39:14Z certification entry. At promotion time the agent wrote the prose summary that motivates the concerns into the parent's `notes` field but never populated the structured `certification.concerns` array. The state-transition-guard's existing 22 checks (1-22) validate scope status canonicalization (Check 4B), DoD completeness (Check 4), evidence freshness, traceability, Gherkin fidelity, and more, but none currently asserts that `status == done_with_concerns ⇒ certification.concerns` exists, is non-empty, and matches the structured shape required by `completion-governance.md`.

The child bug `BUG-CHAOS-20260524-001` was promoted to `done_with_concerns` with a populated `concerns` array, but the agent at the time recorded the entries as plain summary strings rather than the structured object form. Two facts confirm the structured form is the canonical shape:

- `completion-governance.md` explicitly defines the required keys (`id`, `severity`, `summary`, `followUpOwner`, `followUpAction`).
- The example block at `completion-governance.md` lines 195-209 shows a single concern as a JSON object inside a `certification.concerns` array.

## Impacted Components

- `specs/055-notification-source-ntfy-adapter/state.json` — add structured `certification.concerns` array.
- `specs/055-notification-source-ntfy-adapter/bugs/BUG-CHAOS-20260524-001/state.json` — convert the existing string-form `concerns` array into structured-object form (preserving every original summary verbatim).
- `internal/deploy/state_concerns_contract_test.go` — new contract test that loads both files, asserts the schema, and fails deterministically on regression.

## Fix Direction

### Parent state.json concerns population

Mine the parent's existing `notes` field for the canonical concerns and convert them into structured entries. The notes explicitly call out two concerns:

1. *"Validation classified the stale evidence-signal warnings as non-blocking planning/interpreted sections and promoted certification.status to done_with_concerns."* This is one concern: legacy report.md evidence fences need cleanup.
2. *"literal child done remains blocked by legacy report evidence-fence cleanup."* This is one concern: child bug `BUG-CHAOS-20260524-001` literal `done` promotion is gated on the same cleanup.

Both reduce to a single root-cause follow-up (legacy evidence-fence cleanup), but the impact surfaces on two distinct artifacts (parent report.md, child bug state.json), so they are recorded as two structured concerns sharing the same `followUpOwner` (`bubbles.docs`) and `followUpAction` (`next-sprint-todo`).

### Child bug state.json concerns normalization

The existing 7 string entries are summaries already; convert each into a structured object whose `summary` field is the original string verbatim. Severity is `low` for the entire set (every entry describes a stabilize/regression/security/audit/validate provenance note, not a runtime risk). `followUpOwner` for the legacy-cleanup-blocked entry is `bubbles.docs`; for the remaining provenance notes, owner is `human` with `followUpAction: accept` (informational acceptance of recorded provenance).

### Contract test

Create `internal/deploy/state_concerns_contract_test.go` with one test function `TestSpec055StateConcernsContract` that:

1. Reads `specs/055-notification-source-ntfy-adapter/state.json` and the child bug `state.json`.
2. Parses the top-level `status` field and the `certification.status` field.
3. If either is `done_with_concerns`, asserts `certification.concerns` is a non-empty array of objects, each with the required keys, valid severity, valid followUpAction, and a unique `id` within its array.
4. Fails with a precise error message naming the offending file and entry index when any rule is violated.

Placement under `internal/deploy/` mirrors existing contract-style tests (`compose_contract_test.go`, `bundle_secret_contract_test.go`, `ci_integration_topology_contract_test.go`). The test is pure parsing — no environment, no Docker, no network — so it stays in the unit category and is picked up by `./smackerel.sh test unit --go` via `./...` auto-discovery.

### TDD Sequence

1. **Red:** Add the contract test FIRST against the pre-fix tree. Expect: parent fails ("concerns missing"); child fails ("entry 0 is not an object").
2. **Capture red evidence:** `./smackerel.sh test unit --go --go-run 'TestSpec055StateConcernsContract'` produces FAIL output. Record in `report.md#deterministic-red-evidence`.
3. **Green:** Edit parent state.json + child bug state.json to satisfy the schema.
4. **Capture green evidence:** Same command produces PASS output. Record in `report.md#green-evidence`.
5. **Final format/lint/guard sweep:** `./smackerel.sh format --check`, `./smackerel.sh lint`, artifact-lint on bug + parent, traceability-guard on bug + parent, state-transition-guard on parent.

## Risk Notes

- The change boundary is artifact-only plus a single new test file. Zero runtime, deploy, security, or operator-facing surface is touched.
- The child bug's existing `concerns` array uses summary strings authored by `bubbles.validate` at promotion time. Preserving those summaries verbatim inside structured entries avoids introducing new claims and keeps the rework strictly schema-shape, not content-rewriting.
- The contract test deliberately reads files at test-runtime rather than using build-time embeds: this keeps the test sensitive to future drift on the same two files, which is exactly the regression we want to catch.
- The contract test is narrowly scoped to spec 055 and its bug — extending it to the entire `specs/**/state.json` tree is out of scope for this finding (and would risk false positives on specs whose `done_with_concerns` semantics differ). Broader enforcement belongs upstream in `state-transition-guard.sh` (framework-managed) or a future dedicated `governance` spec.
