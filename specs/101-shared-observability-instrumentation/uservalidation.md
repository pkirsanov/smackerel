# User Validation — Spec 101 Shared-Observability Instrumentation Contract

**Status:** in_progress (SCOPE-01 Done · SCOPE-02 DEFERRED-to-flip)

## What was delivered (SCOPE-01 — verifiable now, offline)

smackerel now consumes the knb spec-014 scope-03 shared-observability contract
and is flip-ready:

1. **Canonical 3-var contract** (`OTLP_TRACES_ENDPOINT`, `OTLP_LOGS_ENDPOINT`,
   `METRICS_SCRAPE_LABEL_PRODUCT`) replaces the old declared-not-consumed
   `OTEL_EXPORTER_ENDPOINT`, end-to-end across `config/smackerel.yaml` →
   `config.sh` generator → generated env → `internal/config/config.go`.
2. **Fail-loud reader** — `internal/observability` validates the 3 vars
   non-empty; missing/empty aborts startup when `OTEL_ENABLED=true` (no default,
   no fallback). Inert (startup unaffected) when disabled — bundled/dev/test are
   safe.
3. **Discovery labels** — `com.bubbles.product` (SST-sourced) +
   `com.bubbles.service` on every smackerel container in both compose files,
   contract-locked by a Go test.
4. **Reused, not forked** — the existing `/metrics` endpoint and OTLP/gRPC
   exporter are cited and reused (knb FINDING-014-03-1: no duplication).

### How to re-verify (operator, local, no host mutation)

```bash
cd /Users/pkirsanov/Projects/smackerel
./smackerel.sh test unit --go --go-run 'Validate_Accepts|Validate_Rejects|FromLookup_|SharedObservabilityLabels' --verbose
./smackerel.sh config generate && grep -nE 'OTLP_TRACES_ENDPOINT|OTLP_LOGS_ENDPOINT|METRICS_SCRAPE_LABEL_PRODUCT' config/generated/dev.env
./smackerel.sh lint
```

Expect: all observability + labels tests PASS; the 3 vars present in the env;
`go vet` "All checks passed!".

## What is DEFERRED (SCOPE-02 — needs an operator flip on <deploy-host>)

The live proof cannot be run without live host mutation, which the acceptance
constraints forbid. To complete SCOPE-02, the operator (out of this session):

1. Sets smackerel `sharedServices.observability: shared` in the knb adapter
   params and runs `apply-shared-obs` on <deploy-host> (this also injects the real OTLP
   endpoints + sets the shared posture).
2. Confirms the shared Prometheus discovers `smackerel-core` by
   `com.bubbles.product` and `GET /metrics` returns 200 with the
   `product=smackerel` scrape label.
3. Confirms OTLP spans from smackerel land in the shared Tempo.

Until then, the SCOPE-02 DoD items remain unchecked with their precise reason
(anti-fabrication).

## Verdict

smackerel is **instrumentation-complete and flip-ready pending live
verification**. No live <deploy-host> mutation was performed.

## Checklist

- [x] Current-truth audit performed (no greenfield assumption) — existing `/metrics`, OTLP exporter, otel-go SDK verified + cited.
- [x] knb 3-var contract adopted across SST (`smackerel.yaml` → `config.sh` → generated env → `config.go`).
- [x] Fail-loud `internal/observability` reader + `OTEL_ENABLED`-gated boot gate (9 unit tests PASS).
- [x] `com.bubbles.product` + `com.bubbles.service` labels on all smackerel containers (dev 9/9, deploy 7/7; contract test PASS).
- [x] Existing exporter + `/metrics` reused, NOT forked (knb FINDING-014-03-1).
- [x] gofmt clean · `go vet` clean · `config generate` exit 0 · PII scan 0 hits.
- [ ] **SCOPE-02 live verification on <deploy-host> — DEFERRED-to-flip** (operator `apply-shared-obs`; no live host mutation this session).
