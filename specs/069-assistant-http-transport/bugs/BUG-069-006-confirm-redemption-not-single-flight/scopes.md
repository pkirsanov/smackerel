# Scopes: BUG-069-006 Atomic confirm redemption

**Bug:** BUG-069-006 - confirm redemption is not single-flight under concurrency
**Workflow mode:** bugfix-fastlane
**Status:** In Progress

## Execution Outline

One scope. The defect, the three call sites that carry it, and the store seam
that must change are a single indivisible unit: making `Confirm` atomic while
leaving `Discard` and `SweepTimeouts` on the old pattern would leave the
audit-integrity race open, and the store method they all need is the same
method. Splitting this would create a scope whose exit state is a
partially-atomic redemption path, which is not a coherent state to certify.

## Mechanical Allowed List

**Change Boundary — Allowed file families.** Only these concrete paths may be
modified by this packet.

### Implementation Files

| Path | Reason |
|---|---|
| `internal/assistant/context/store.go` | Add the conditional-clear method to the `Store` interface |
| `internal/assistant/context/pg_store.go` | Implement the conditional `UPDATE` and read `CommandTag.RowsAffected()` |
| `internal/assistant/testing_support.go` | Implement the same method atomically on `InMemoryContextStore` |
| `internal/assistant/confirm/machine.go` | Route `Confirm`, `Discard`, `SweepTimeouts` through it; correct the line 213 comment |

### Test Files

| Path | Reason |
|---|---|
| `internal/assistant/confirm/machine_concurrency_test.go` | New: concurrent redemption unit coverage |
| `internal/assistant/context/pg_store_test.go` | Conditional-clear integration coverage against a live store |
| `tests/e2e/assistant/http_confirm_test.go` | Extended: concurrent confirm regression at the API boundary, alongside the existing sequential replay case |

### Packet Artifacts

| Path | Reason |
|---|---|
| `specs/069-assistant-http-transport/bugs/BUG-069-006-confirm-redemption-not-single-flight/**` | This packet's own artifacts |

## Mechanical Excluded List

**Change Boundary — Excluded surfaces.** No scope in this packet may change any
of the following.

| Surface | Reason |
|---|---|
| `specs/069-assistant-http-transport/bugs/BUG-069-005-required-e2e-false-green/**` | Origin packet. Its scenario is satisfied and is not reopened here |
| `specs/069-assistant-http-transport/bugs/BUG-069-004-http-turn-dedup/**` | Separate in-flight packet with its own Change Boundary |
| `.github/bubbles/**` | Framework-managed |
| `internal/assistant/httpadapter/**` | The dedup key is correct for its purpose; retitling it is a non-goal per `spec.md` |
| `internal/db/migrations/**` | The selected design requires no schema change |
| `tests/e2e/assistant/http_confirm_test.go` | The existing sequential test must keep passing unmodified, which is what proves EB-6 |
| `tests/e2e/assistant/required_no_skip_guard_test.go` | Guard surface owned by BUG-069-005 |
| Any deploy, compose, or configuration file | The fix is code-local |

## Scope Inventory

| Scope | Name | Depends On | Scenario IDs | Status |
|---|---|---|---|---|
| 1 | Atomic redemption across confirm, discard, and timeout sweep | none | SCN-BUG069006-001, SCN-BUG069006-002, SCN-BUG069006-003, SCN-BUG069006-004 | Not Started |

## Scope 1: Atomic redemption across confirm, discard, and timeout sweep

**Status:** In Progress
**Depends On:** none

### Gherkin Scenarios

```gherkin
Feature: Confirm redemption is single-flight under concurrency

  Scenario: SCN-BUG069006-001 Two concurrent confirms redeem one reference once
    Given a live pending confirm holds reference "R" for an authenticated user
    And two confirm attempts for reference "R" are released simultaneously
    When both attempts execute concurrently
    Then exactly one attempt returns a populated ConfirmResult
    And the other attempt returns ErrPendingNotFound
    And the gated action has executed exactly once
    And exactly one confirmed audit row exists for reference "R"

  Scenario: SCN-BUG069006-002 Two concurrent HTTP confirms with distinct message ids
    Given a live pending confirm holds reference "R" for an authenticated user
    And two POSTs to the assistant turn endpoint both carry reference "R"
    And each POST carries a different transport_message_id
    When both requests are issued concurrently
    Then the gated action has executed exactly once
    And exactly one response reports a successful confirmation

  Scenario: SCN-BUG069006-003 A confirm racing the timeout sweep yields one outcome
    Given a pending confirm holds reference "R" and its expiry has elapsed
    And a confirm attempt and a timeout sweep for reference "R" run concurrently
    When both complete
    Then exactly one terminal audit row exists for reference "R"
    And that row is either confirmed or discarded_timeout but never both

  Scenario: SCN-BUG069006-004 Sequential redemption behaviour is unchanged
    Given a live pending confirm holds reference "R"
    When a single confirm for reference "R" is issued and then replayed
    Then the first returns a populated ConfirmResult and executes the action once
    And the replay returns ErrPendingNotFound without executing the action again
    And the conversation working context and other pending columns are unchanged
```

### Implementation Plan

1. Add the conditional-clear method to the `Store` interface in
   `internal/assistant/context/store.go`, returning whether this caller
   performed the clear.
2. Implement it on `PgStore` as a single `UPDATE ... WHERE user_id = $1 AND
   transport = $2 AND pending_confirm ->> 'confirm_ref' = $4`, reading
   `CommandTag.RowsAffected()` rather than discarding the tag as `Persist` does.
3. Implement it on `InMemoryContextStore` with genuine atomicity under its own
   lock, so unit-level concurrency assertions are meaningful.
4. Write the concurrent tests first and record them failing against the unfixed
   implementation, per the adversarial requirement in `spec.md`.
5. Route `Confirm`, `Discard`, and `SweepTimeouts` through the new method,
   mapping a `false` result to `ErrPendingNotFound`, keeping the audit write
   after a successful clear.
6. Correct the single-flight comment at `machine.go` line 213 so it states the
   guarantee the code provides.
7. Confirm the existing sequential E2E still passes unmodified.

### Test Plan

| Row | Scenario | Category | File/Location | Expected Test Title | Command | Live System |
|---|---|---|---|---|---|---|
| TP-BUG069006-01 | SCN-BUG069006-001 | unit | `internal/assistant/confirm/machine_concurrency_test.go` | `TestMachineConfirm_ConcurrentRedemptionExecutesOnce` | `./smackerel.sh test unit --go` | No |
| TP-BUG069006-02 | SCN-BUG069006-003 | unit | `internal/assistant/confirm/machine_concurrency_test.go` | `TestMachineConfirm_RacingSweepProducesOneTerminalOutcome` | `./smackerel.sh test unit --go` | No |
| TP-BUG069006-03 | SCN-BUG069006-001 | integration | `internal/assistant/context/pg_store_test.go` | `TestPgStoreClearPendingConfirm_IsConditionalAndAtomic` | `./smackerel.sh test integration` | Yes |
| TP-BUG069006-04 | SCN-BUG069006-002 | Regression E2E API | `tests/e2e/assistant/http_confirm_test.go` | `TestAssistantHTTPE2E_ConcurrentConfirmExecutesGatedActionOnce` | `./smackerel.sh test e2e` | Yes |
| TP-BUG069006-05 | SCN-BUG069006-004 | Regression E2E API | `tests/e2e/assistant/http_confirm_test.go` | `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` | `./smackerel.sh test e2e` | Yes |

TP-BUG069006-05 is an existing test that must pass unmodified. It is listed
because EB-6 depends on it, not because it is new work.

### Definition of Done

- [ ] Root cause is confirmed by execution, not only by static reading.
- [ ] The concurrent redemption test is recorded FAILING against the unfixed implementation before any fix lands.
- [x] The conditional-clear method exists on the `Store` interface and on every implementation, including `InMemoryContextStore`. → Evidence: [report.md](report.md#definition-of-done-what-this-evidence-settles)
- [ ] `PgStore` reads `CommandTag.RowsAffected()` and reports whether it performed the clear.
- [ ] `Confirm`, `Discard`, and `SweepTimeouts` all route through the conditional clear and map a lost race to `ErrPendingNotFound`.
- [x] Two concurrent confirms of one reference execute the gated action exactly once, proven by a test using real goroutines and a release barrier. → Evidence: [report.md](report.md#proving-the-two-concurrency-tests-actually-executed)
- [x] The losing caller receives `ErrPendingNotFound`, indistinguishable from a post-redemption replay. → Evidence: [report.md](report.md#proving-the-two-concurrency-tests-actually-executed)
- [x] Exactly one confirmed audit row exists for the reference after a concurrent race. → Evidence: [report.md](report.md#the-tests-are-sensitive-to-the-defect)
- [x] A confirm racing the timeout sweep produces exactly one terminal audit row. → Evidence: [report.md](report.md#the-tests-are-sensitive-to-the-defect)
- [ ] The single-flight comment at `machine.go` line 213 states the guarantee the code actually enforces.
- [ ] A redemption write leaves `working_context`, `pending_disambig`, `pending_clarify`, and `legacy_retirement_notices` untouched.
- [ ] `TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce` passes unmodified.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass.
- [ ] Broader E2E regression suite passes.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [x] `SCN-BUG069006-001` holds: two concurrent confirms carrying one reference redeem it exactly once — one caller observes the win, the other receives `ErrPendingNotFound`, and exactly one `confirmed` audit row exists. → Evidence: [report.md](report.md#unit-concurrency-re-executed-at-the-current-tree)
- [x] `SCN-BUG069006-002` holds at the API boundary: concurrent HTTP confirms with distinct transport message ids and one shared `ConfirmRef` execute the gated action exactly once, proven against the live stack rather than in-process. → Evidence: [report.md](report.md#the-second-defect-the-loser-path-resurrected-the-pending-row)
- [x] `SCN-BUG069006-003` holds: a confirm racing the timeout sweep yields exactly one terminal outcome, never both a `confirmed` and a `discarded_timeout` row for the same reference. → Evidence: [report.md](report.md#unit-concurrency-re-executed-at-the-current-tree)
- [x] `SCN-BUG069006-004` holds: sequential redemption behaviour is unchanged — a single confirm still succeeds and a sequential replay of the same reference still fails without re-executing the gated action. → Evidence: [report.md](report.md#2026-08-24-third-test-pass-the-sequential-replay-lane-ran)
- [ ] Build Quality Gate passes as a grouped block: zero warnings, zero deferrals, lint and format clean, artifact lint clean, documentation aligned.
