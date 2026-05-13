# Bug Fix Design: BUG-026-003

## Root Cause Analysis

### Investigation Summary
Stochastic-quality-sweep round 10 of 20 (regression trigger, seed 20520512) probed spec 026 for cross-spec conflicts and baseline regressions on extraction code paths. Baseline tests passed; cross-spec design coherence with 035 (recipe enhancements, read-time transform with no schema changes) and 037 (LLM agent tools, preserves Go-owned data ownership) was intact. The regression probe surfaced one real defect: coverage on `(*DomainResultSubscriber).handleDomainExtracted` is 0.0% from `go test ./...`, despite five tests in `internal/pipeline/domain_subscriber_test.go` named `TestHandleDomainExtracted_*`.

Source inspection of those five tests confirmed they only call `ValidateDomainExtractResponse(&resp)` on a hand-built struct literal. None of them construct a `DomainResultSubscriber`, none inject a fake DB, none build a `jetstream.Msg`, and none call `sub.handleDomainExtracted(ctx, msg)`. The function name appears in the test names but never in the test bodies as a function call.

### Root Cause
Two cooperating contract gaps in spec 026's original delivery:

1. **Test naming fabrication.** The five `TestHandleDomainExtracted_*` tests were authored to satisfy the trace-guard mapping `Gherkin scenario → DoD bullet → Test Plan row → test function name → file mention in report.md` mechanically, by pattern-matching on the function name. They prove the validator works on struct literals, not that the receiver method works on jetstream messages with a real DB write.
2. **Concrete DB type blocks unit testing.** The struct field `DB *pgxpool.Pool` is a concrete pgx pool. Without a real Postgres or an interface boundary, the only way to invoke `handleDomainExtracted` from a test is to either (a) stand up integration infrastructure or (b) pass `nil` for `DB` (which segfaults inside the SQL UPDATE branch). Neither was done; the validator-only tests were chosen as a no-infrastructure shortcut and labelled as if they covered the handler.

The runtime path **is** covered by `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`, but that test is gated on `CORE_EXTERNAL_URL` and is skipped by `./smackerel.sh test unit`. Without a unit-level safety net, any future change to the SQL string (column rename, status enum drift, missed `domain_extracted_at = NOW()` stamping, swapped Ack/Nak) ships green from the unit suite and is only caught by the live-stack E2E run, which is not part of every developer's pre-commit loop.

### Impact Analysis
- **Affected components:** `internal/pipeline/domain_subscriber.go` (production), `internal/pipeline/domain_subscriber_test.go` (test), spec 026 `report.md` finding TG1 (truth-in-evidence claim).
- **Affected data:** None at runtime — the production handler logic is correct (verified by spec-review entry 2026-04-24 and by the live-stack E2E pass on 2026-04-29 commit c6d2b26). The defect is a regression-protection gap, not an active runtime bug.
- **Affected users:** Any future contributor refactoring `handleDomainExtracted` who relies on `./smackerel.sh test unit` as their pre-commit safety net. The defect creates a silent-regression risk on a documented-as-Fixed surface.

## Fix Design

### Solution Approach
Minimal-impact production refactor + appended real-invocation tests:

1. **Production refactor (single file: `internal/pipeline/domain_subscriber.go`):**
   - Add a new exported interface `DomainDB` with one method: `Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)`.
   - Change the struct field `DB *pgxpool.Pool` to `DB DomainDB`.
   - Keep the constructor signature `NewDomainResultSubscriber(db *pgxpool.Pool, nc *smacknats.Client)` unchanged (`*pgxpool.Pool` already implements `Exec(ctx, sql, args...) (pgconn.CommandTag, error)`, so it satisfies `DomainDB` by structural typing). The `cmd/core/services.go:152` call site is unaffected.
2. **Test additions (single file: `internal/pipeline/domain_subscriber_test.go`):**
   - Add a `mockDomainDB` type implementing `DomainDB`, recording every `Exec` call (sql + args) and returning a configurable error.
   - Add six new tests under the prefix `TestHandleDomainExtractedInvocation_*` covering every documented branch.
3. **Truth-in-evidence amendment (single file: `specs/026-domain-extraction/report.md`):**
   - Annotate finding TG1 in place: change the status from `Fixed — added domain_subscriber_test.go` to `Re-opened by BUG-026-003 (the added tests cover only ValidateDomainExtractResponse on struct literals; not the receiver method); closed by BUG-026-003 with real handler invocation tests in domain_subscriber_test.go`.

### Alternative Approaches Considered
1. **Add `pgxmock` dependency.** Rejected — pulls in a new third-party module for one usage when a six-line interface suffices.
2. **Use a real test Postgres.** Rejected — turns this from a unit test into an integration test, bloats `./smackerel.sh test unit` runtime, and the live-stack E2E already provides the integration-level proof.
3. **Rename the existing five `TestHandleDomainExtracted_*` tests to `TestValidateDomainExtractResponse_*` to match what they actually do.** Considered as an additional cleanup but deferred — the existing names are referenced by `report.md`, `scopes.md`, and the trace-guard mapping. Renaming would require updating those references too. The new `TestHandleDomainExtractedInvocation_*` tests use a distinct, unambiguous prefix that cannot be confused with the validator tests.

## Affected Files
- `internal/pipeline/domain_subscriber.go` — added `DomainDB` interface, changed `DB` field type from `*pgxpool.Pool` to `DomainDB`, added `pgconn` import.
- `internal/pipeline/domain_subscriber_test.go` — added `mockDomainDB`, added 6 `TestHandleDomainExtractedInvocation_*` tests, added imports (`strings`, `sync`, `pgconn`).
- `specs/026-domain-extraction/report.md` — annotated finding TG1 with truth-in-evidence correction.

`cmd/core/services.go` is intentionally NOT modified — `*pgxpool.Pool` satisfies the new `DomainDB` interface.
`tests/e2e/domain_e2e_test.go` is intentionally NOT modified — live-stack E2E remains the integration-level proof.

## Regression Test Design
- **Unit-level coverage gain:** `handleDomainExtracted` coverage moves from 0.0% to 96.8% (verified 2026-05-12). The remaining 3.2% is one error-log slog call when both `Exec` and `handleDomainDeliveryFailure` paths complete, which is intentionally not asserted because slog output is not contractually stable.
- **Adversarial assertions:** Every new test asserts on the SQL **string contents** (column names, status enum, `NOW()` stamping) and on **bound parameters** by index — not just on whether `Exec` was called. A regression that swaps `'completed'` for `'processed'`, drops the `domain_extracted_at = NOW()` clause from the failure path, or changes the bind order of `(artifact_id, domain_data, contract_version)` will fail at least one new test.
- **DB-error path coverage:** Two new tests inject a failing `Exec` and assert that the message is Nak'd (not Ack'd) on both the success branch and the failure branch. A regression that ever silently Acks on DB error fails loudly.
- **Live-stack defense-in-depth preserved:** `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` is unchanged and continues to provide the integration-level proof.

## Ownership
- Owning feature/spec: `specs/026-domain-extraction`
- Discovery owner: `bubbles.workflow` (parent-expanded `regression-to-doc` mode, stochastic-quality-sweep round 10 of 20, regression trigger, seed 20520512)
- Fix owner: `bubbles.implement` (parent-expanded)
- Test owner: `bubbles.test` (parent-expanded)
- Validation owner: `bubbles.validate` (parent-expanded)
