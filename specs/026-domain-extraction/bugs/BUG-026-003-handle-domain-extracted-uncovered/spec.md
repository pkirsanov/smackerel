# Feature: BUG-026-003 handleDomainExtracted has zero unit coverage despite five same-named tests

## Problem Statement
Spec 026's `internal/pipeline/domain_subscriber.go` contains the production handler `(*DomainResultSubscriber).handleDomainExtracted`, which on every `domain.extracted` JetStream message decides whether to issue an `UPDATE artifacts SET domain_extraction_status = 'completed' / 'failed'` SQL, whether to Ack/Nak, and whether to route the message to the dead-letter stream after `domainMaxDeliver` retries. Spec 026 originally claimed (`report.md` line 171, finding TG1) that this handler was covered by five `TestHandleDomainExtracted_*` unit tests in `internal/pipeline/domain_subscriber_test.go`. Coverage probing revealed that those tests only call `ValidateDomainExtractResponse(&resp)` on a hand-built struct literal — none of them invoke `sub.handleDomainExtracted(ctx, msg)`. The function therefore sits at 0.0% unit coverage. The only test that actually exercises the path end-to-end is the live-stack `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`, which is skipped by `./smackerel.sh test unit` (it requires `CORE_EXTERNAL_URL`).

## Outcome Contract
**Intent:** Every documented branch of `handleDomainExtracted` is covered by a unit test that actually invokes the receiver method, so a refactor that breaks the SQL UPDATE shape, swaps Ack/Nak, or drops the failure-path stamping fails `./smackerel.sh test unit`.
**Success Signal:** `go test ./internal/pipeline/...` exercises `handleDomainExtracted` at ≥ 80% coverage with assertions on (a) the success-path SQL UPDATE column set, (b) the failure-path SQL UPDATE column set including `domain_extracted_at = NOW()` per S-001, (c) the success Ack, (d) the validation-failure Ack-without-DB-write, (e) the unparseable-JSON Ack-without-DB-write, (f) the DB-error → Nak-below-domainMaxDeliver routing on both success and failure paths.
**Hard Constraints:** No live-stack dependency in the new unit tests. The production refactor must be backward-compatible with the existing `cmd/core/services.go:152` call site that passes `*pgxpool.Pool`. The existing five `TestHandleDomainExtracted_*` struct-validation tests are kept (they prove the validator behavior); the new tests are appended with distinct, honest names beginning with `TestHandleDomainExtractedInvocation_`.
**Failure Condition:** Coverage on `handleDomainExtracted` remains below 80%, or the new tests pass while the production handler is broken (i.e., assertions are too lax to detect a real regression).

## Goals
- Surface the prior false-coverage claim transparently in the parent spec's `report.md` finding TG1.
- Refactor the `DB` field minimally so `handleDomainExtracted` can be unit-tested without standing up a real Postgres.
- Add real handler-invocation unit tests covering every documented branch.
- Preserve live-stack E2E coverage (`tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`) as defense-in-depth.

## Non-Goals
- Adding any new business behavior to `handleDomainExtracted`. The handler logic does not change.
- Lifting `Start` coverage beyond its current 20.6% — the long-running consumer goroutine is intentionally tested via the live-stack E2E only and is out of scope for this bug.
- Removing the existing `TestHandleDomainExtracted_*` struct-validation tests — they remain valid assertions on `ValidateDomainExtractResponse` and are kept for that purpose.

## Requirements
- The production refactor MUST keep the `cmd/core/services.go:152` call site (which passes `*pgxpool.Pool`) compiling and behaving identically.
- Every new unit test MUST invoke the actual `(*DomainResultSubscriber).handleDomainExtracted` method.
- New tests MUST assert on the SQL string contents (column names, status enum, `domain_extracted_at = NOW()`) and on the bound parameters, not only on whether `Exec` was called.
- The DB-error → Nak path MUST be covered for BOTH the success branch and the failure branch of `handleDomainExtracted`.
- Coverage on `handleDomainExtracted` MUST be ≥ 80% from `go test ./internal/pipeline/...` alone.

## User Scenarios (Gherkin)

```gherkin
Scenario: handleDomainExtracted is covered by real invocation tests
  Given the domain.extracted handler refactor exposes a DomainDB interface for the DB field
  And the test file contains a mockDomainDB implementing Exec
  When `go test -coverprofile ./internal/pipeline/...` runs
  Then the coverage report shows handleDomainExtracted at >= 80% coverage
  And every passing test under TestHandleDomainExtractedInvocation_* invokes the real receiver method

Scenario: A regression in the success-path UPDATE is caught by go test
  Given a future refactor accidentally drops `domain_extracted_at = NOW()` from the success-path SQL
  When `go test ./internal/pipeline/...` runs
  Then TestHandleDomainExtractedInvocation_Success_UpdatesArtifactAndAcks fails with a SQL-content assertion mismatch
  And the failure points at the missing column

Scenario: A DB error during the success path Naks instead of silent Ack
  Given the injected mockDomainDB returns an Exec error on the success-path UPDATE
  And the message Metadata reports NumDelivered below domainMaxDeliver
  When handleDomainExtracted runs
  Then the message is Nak'd, not Ack'd
  And the test fails loudly if the handler ever silently Acks on DB error
```

## Acceptance Criteria
- A `DomainDB` interface is introduced in `internal/pipeline/domain_subscriber.go` and the struct field `DB` uses that interface.
- `cmd/core/services.go:152` continues to pass `svc.pg.Pool` (a `*pgxpool.Pool`) without modification — the production call site is unchanged.
- Six new tests `TestHandleDomainExtractedInvocation_*` are added to `internal/pipeline/domain_subscriber_test.go` covering: success UPDATE+Ack, failure-status UPDATE+Ack, invalid-JSON Ack-without-DB, missing-artifact-id Ack-without-DB, success-path DB-error → Nak, failure-path DB-error → Nak.
- `go test -coverprofile ./internal/pipeline/...` reports `handleDomainExtracted` at ≥ 80%.
- `go build ./...`, `go vet ./...`, `gofmt -l internal/pipeline/`, and `go test ./internal/pipeline/...` all exit 0.
- Spec 026's `report.md` finding TG1 is amended in-place to reflect the real status: the original "Fixed" claim is annotated as struct-validation-only, with a forward link to BUG-026-003.

## Current Evidence Status

- Pre-fix coverage probe captured: `handleDomainExtracted` at 0.0%.
- Post-fix coverage probe captured: `handleDomainExtracted` at 96.8% (see `report.md`).
- Production refactor verified non-breaking: `go build ./...` exits 0, `cmd/core/services.go:152` unchanged.
- All six new `TestHandleDomainExtractedInvocation_*` tests pass.
