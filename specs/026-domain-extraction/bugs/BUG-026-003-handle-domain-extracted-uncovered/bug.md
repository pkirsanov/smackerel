# Bug: BUG-026-003 handleDomainExtracted has zero unit coverage despite five same-named tests

## Summary
Spec 026's `report.md` (line 171, finding TG1) claims "No unit tests for `DomainResultSubscriber.handleDomainExtracted`" was **Fixed — added `domain_subscriber_test.go`**. The added tests (`TestHandleDomainExtracted_SuccessPayload`, `_FailurePayload`, `_InvalidJSONDetected`, `_MissingArtifactIDRejected`, `_FailureSQL_IncludesDomainExtractedAt`) only invoke `ValidateDomainExtractResponse` on a struct literal — they never call the receiver method `(*DomainResultSubscriber).handleDomainExtracted`. The function therefore sits at 0.0% unit coverage even though five tests bear its name. Coverage on the success-path SQL UPDATE, failure-path SQL UPDATE, success Ack, and failure-path Nak/dead-letter routing is provided only by the live-stack `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`, which is skipped by plain `go test ./...` (requires `CORE_EXTERNAL_URL`). Any future refactor of `handleDomainExtracted` (column rename, SQL typo, Ack/Nak swap, metrics-label drift) will pass `./smackerel.sh test unit` silently.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Domain extraction live-stack certification blocked
- [x] Medium - Test integrity gap that creates a regression risk on a documented-as-Fixed surface
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (coverage probe + source inspection both confirmed 0.0% on `handleDomainExtracted`)
- [x] In Progress
- [x] Fixed (production refactor + 6 new invocation tests; coverage 0.0% → 96.8%)
- [x] Verified
- [x] Closed

## Reproduction Steps
1. From repo root, run `go test -coverprofile=/tmp/cov.out ./internal/pipeline/... && go tool cover -func=/tmp/cov.out | grep "domain_subscriber.go"`.
2. Observe that `handleDomainExtracted` reports `0.0%` coverage even though `internal/pipeline/domain_subscriber_test.go` contains five `TestHandleDomainExtracted_*` functions.
3. Open `internal/pipeline/domain_subscriber_test.go` and confirm that none of those tests call `sub.handleDomainExtracted(...)` — they only construct a `DomainExtractResponse` struct and call `ValidateDomainExtractResponse` on it.

## Expected Behavior
Tests named `TestHandleDomainExtracted_*` should actually exercise the receiver method `(*DomainResultSubscriber).handleDomainExtracted`, covering the JSON unmarshal branch, the validation branch, the success-path SQL UPDATE + Ack, and the failure-path SQL UPDATE + Ack/Nak routing. Coverage on the function should be > 80% from `go test ./...` alone (live-stack E2E should be a defense-in-depth layer, not the only proof).

## Actual Behavior
- `handleDomainExtracted` shows 0.0% coverage in the unit-test profile.
- The five `TestHandleDomainExtracted_*` tests collectively prove only that `ValidateDomainExtractResponse` accepts/rejects struct literals — they do not prove that the JetStream message → SQL UPDATE → Ack/Nak path works.
- A regression that breaks the SQL string (e.g., column renamed, status enum changed, contract version bind dropped) would still ship green from `./smackerel.sh test unit`. Only the live-stack E2E (`CORE_EXTERNAL_URL` required) catches it.

## Environment
- Service: Go core, `internal/pipeline.DomainResultSubscriber`
- Version: Workspace state on 2026-05-12 during stochastic-quality-sweep round 10 of 20 (regression trigger, seed 20520512)
- Platform: Linux, plain `go test ./...` (no live stack)

## Error Output
```text
$ go test -coverprofile=/tmp/dc.out ./internal/pipeline/...
ok  github.com/smackerel/smackerel/internal/pipeline  0.290s

$ go tool cover -func=/tmp/dc.out | grep domain_subscriber.go
internal/pipeline/domain_subscriber.go:38:   NewDomainResultSubscriber       100.0%
internal/pipeline/domain_subscriber.go:46:   Start                            20.6%
internal/pipeline/domain_subscriber.go:113:  Stop                             28.6%
internal/pipeline/domain_subscriber.go:137:  handleDomainExtracted             0.0%
internal/pipeline/domain_subscriber.go:214:  handleDomainDeliveryFailure      55.6%
```

## Root Cause
Two cooperating contract gaps:

1. The five `TestHandleDomainExtracted_*` unit tests added during spec 026's original delivery test only the response *validator* on hand-built struct literals. They never construct a `DomainResultSubscriber`, never inject a fake DB, never build a `jetstream.Msg`, and never call `sub.handleDomainExtracted(ctx, msg)`. The test names lie about what is being tested.
2. The struct field `DB *pgxpool.Pool` is a concrete type, so unit tests cannot inject a mock without either (a) standing up a real Postgres or (b) refactoring `DB` to an interface that exposes `Exec(ctx, sql, args...) (pgconn.CommandTag, error)`. Spec 026's original delivery chose neither and instead added struct-validation tests that pattern-match the function name to satisfy `Gherkin scenario` → `DoD` → `Test Plan` traceability without actually exercising the handler.

## Related
- Feature: `specs/026-domain-extraction/`
- Surfaced by: stochastic-quality-sweep round 10 of 20, regression trigger, seed 20520512 (parent-expanded child mode `regression-to-doc`)
- Touched production file: `internal/pipeline/domain_subscriber.go`
- Touched test file: `internal/pipeline/domain_subscriber_test.go`
- Live-stack defense-in-depth (preserved): `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction`
- Spec 026 finding the closure invalidates: `report.md` line 171, finding TG1, status changed from `Fixed — added domain_subscriber_test.go` to `Re-opened by BUG-026-003 (struct-validation only); closed by BUG-026-003 with real handler invocation tests.`
