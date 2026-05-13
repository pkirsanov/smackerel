# User Validation Checklist

## Checklist

- [x] Bug packet exists for the handleDomainExtracted unit-coverage gap.
- [x] Pre-fix evidence captured: `handleDomainExtracted` at 0.0% coverage from `go test ./internal/pipeline/...`.
- [x] Production refactor is backward-compatible: `cmd/core/services.go:152` is byte-identical to before (`git diff --stat` exits 0 with no output).
- [x] Six new `TestHandleDomainExtractedInvocation_*` tests exist and pass; each invokes the real receiver method.
- [x] Post-fix coverage on `handleDomainExtracted` is 96.8% (≥ 80% acceptance criterion).
- [x] Pre-existing pipeline tests still pass after the refactor.
- [x] Spec 026 `report.md` finding TG1 is amended in place with truth-in-evidence annotation.
- [x] Live-stack `tests/e2e/domain_e2e_test.go::TestE2E_DomainExtraction` is unmodified (preserved as defense-in-depth).

## Evidence Notes

- Pre-fix probe: `go test -coverprofile=/tmp/dc.out ./internal/pipeline/... && go tool cover -func=/tmp/dc.out | grep "domain_subscriber.go"` showed `handleDomainExtracted 0.0%`.
- Post-fix probe: same command shows `handleDomainExtracted 96.8%`.
- New tests verbose run: 6 `TestHandleDomainExtractedInvocation_*` tests pass, each emitting slog output from inside the real handler (success: `INFO domain extraction completed`; failure: `WARN domain extraction failed`; invalid JSON: `ERROR invalid domain.extracted payload`; missing artifact_id: `ERROR domain.extracted payload validation failed`; success-path DB error: `ERROR store domain extraction result`; failure-path DB error: `ERROR update artifact domain status to failed`). The slog calls are inside `handleDomainExtracted` itself and only fire when the receiver method actually runs.
- Build/vet/format gates all exit 0 after the refactor.
- Production call site untouched: `git diff --stat cmd/core/services.go` exits 0 with no output.
- Sweep provenance: stochastic-quality-sweep round 10 of 20, regression trigger, seed 20520512, parent-expanded `regression-to-doc` mode.
