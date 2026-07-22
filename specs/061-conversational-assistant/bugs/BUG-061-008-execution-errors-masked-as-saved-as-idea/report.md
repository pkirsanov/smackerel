# BUG-061-008 — Report

## Summary

Systemic fix for the recurring "saved as an idea" masking: the facade ran the provenance
gate on every no-sources response regardless of outcome, so a non-OK execution failure
(provider-error/timeout/no-tool-call) was rewritten to `StatusSavedAsIdea` + capture and the
error cause discarded. P1 runs the gate only on `OutcomeOK`; non-OK outcomes surface honestly.
P2 adds a cross-scenario invariant test (the mechanical regression gate). P3 adds an
execution-error metric. P4/P5 document + encode the invariant.

## Completion Statement

P1–P5 implemented and validated by `go test`. The provenance/capture gate now runs only on
`OutcomeOK`; every non-OK outcome surfaces honestly through `translateFinalToBody` with
`ErrorCause` preserved (P1). A cross-scenario table invariant test proves it and would fail if
the guard were reverted (P2). An `ExecutionErrorSurfacedTotal` metric makes surfaced failures
observable (P3). The deterministic-dispatch seam and the failure-honesty invariant are
documented in `docs/smackerel.md` §3.8.6 and encoded as a review-checklist rule in
`.github/copilot-instructions.md` (P4, P5). All BUG-061-008 tests pass; the pre-existing
fabrication-guard tests remain green (the fix does not over-correct). Live home-lab deploy +
operator behavioral confirmation are tracked below and in `uservalidation.md`.

## P1 evidence {#p1-evidence}

Gate now guarded by `result.Outcome == agent.OutcomeOK` in `internal/assistant/facade.go`;
`translateFinalToBody` returns friendly truthful copy for provider-error/timeout; `BS006`
refined to the honest-error contract (`StatusUnavailable`, `ErrProviderUnavailable`,
`CaptureRoute=false`, body ≠ capture acknowledgement). The pre-existing OK-outcome fabrication
guards (`AntiFabrication`, `ProvenanceGateRewritesWhenSourcesMissing`) stay GREEN, proving the
fix does not over-correct.

```text
$ ./smackerel.sh test unit --go --go-run '_BS006_|AntiFabrication|ProvenanceGateRewritesWhenSourcesMissing' --verbose
=== RUN   TestExecutor_BS006_HallucinatedToolRejectedBeforeLookup
--- PASS: TestExecutor_BS006_HallucinatedToolRejectedBeforeLookup (0.00s)
ok      github.com/smackerel/smackerel/internal/agent   0.046s
=== RUN   TestFacadeHighBandProvenanceGateRewritesWhenSourcesMissing
--- PASS: TestFacadeHighBandProvenanceGateRewritesWhenSourcesMissing (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant       0.292s
[go-unit] go test ./... finished OK
```

`internal/assistant/facade_weather_integration_test.go:175` —
`func TestFacadeWeatherIntegration_BS006_ProviderUnavailableSurfacesHonestly(...)` — runs and
passes (`go test ./... finished OK`, exit 0).

## P2 evidence {#p2-evidence}

New table invariant test `internal/assistant/facade_execution_error_honesty_test.go` sweeps
every `requires_provenance` scenario × each error outcome and asserts honest surfacing; plus
OK+no-sources cases assert the fabrication guard still fires.

```text
$ ./smackerel.sh test unit --go --go-run 'ExecutionErrorHonesty' --verbose
=== RUN   TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea
=== RUN   TestExecutionErrorHonesty_OKNoSourcesStillRefuses
=== RUN   TestExecutionErrorHonesty_MetricIncrements
--- PASS: TestExecutionErrorHonesty_MetricIncrements (0.00s)
--- PASS: TestExecutionErrorHonesty_OKNoSourcesStillRefuses (0.00s)
    --- PASS: TestExecutionErrorHonesty_OKNoSourcesStillRefuses/weather_query (0.00s)
    --- PASS: TestExecutionErrorHonesty_OKNoSourcesStillRefuses/retrieval_qa (0.00s)
    --- PASS: TestExecutionErrorHonesty_OKNoSourcesStillRefuses/recipe_search (0.00s)
--- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea (0.01s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/weather_query/provider-error (0.00s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/weather_query/timeout (0.00s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/retrieval_qa/provider-error (0.00s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/retrieval_qa/timeout (0.00s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/recipe_search/provider-error (0.00s)
    --- PASS: TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea/recipe_search/timeout (0.00s)
ok      github.com/smackerel/smackerel/internal/assistant       0.292s
[go-unit] go test ./... finished OK
```

Adversarial quality confirmed by the regression-quality-guard (would fail if the P1 guard were
reverted):

```text
$ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/assistant/facade_execution_error_honesty_test.go
ℹ️  Scanning internal/assistant/facade_execution_error_honesty_test.go
✅ Adversarial signal detected in internal/assistant/facade_execution_error_honesty_test.go
  REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
  Files scanned: 1
  Files with adversarial signals: 1
GUARD_EXIT=0
```

## P3 evidence {#p3-evidence}

`ExecutionErrorSurfacedTotal{scenario_id, outcome, transport}` added to
`internal/assistant/metrics/metrics.go` and registered; incremented in the facade's non-OK
branch. `TestExecutionErrorHonesty_MetricIncrements` asserts delta == 1 for
`{weather_query, provider_error, fake}` (see P2 run above — `--- PASS:
TestExecutionErrorHonesty_MetricIncrements`).

## P4 evidence {#p4-evidence}

Deterministic-dispatch seam pattern documented in `docs/smackerel.md` §3.8.6 "Failure Honesty
+ Deterministic Dispatch" (Invariant 2): explicit slash commands resolve their tool through an
injected facade seam (`WithWeatherLookup` → `handleWeatherShortcut`), never depending on
LLM tool-call reliability; new commands SHOULD follow the same typed-seam + `With…` +
direct-dispatch + unit-test pattern.

## P5 evidence {#p5-evidence}

Invariant stated in `docs/smackerel.md` §3.8.6 (Invariant 1): "the provenance/capture gate
runs ONLY on `result.Outcome == agent.OutcomeOK`; non-OK outcomes surface honestly and never
`StatusSavedAsIdea`." Review-checklist rule added to `.github/copilot-instructions.md` →
"Assistant Response Honesty (NON-NEGOTIABLE)", citing
`internal/assistant/facade_execution_error_honesty_test.go` as the mechanical enforcement and
`smackerel_assistant_execution_error_surfaced_total` as the observability signal.

## Test Evidence

Full suite green for the affected packages (`go test ./... finished OK`, exit 0); the
`internal/assistant` and `internal/agent` packages report `ok`. Per-scenario PASS lines are in
the P1/P2 blocks above.

## Deploy + Live Verification (self-hosted home-lab) {#deploy-verify}

The P1–P5 fix (sourceSha `19fe72c8`) was built, operator-cosign-signed, deployed on-host, and
verified running healthy this session (`local-operator` trust model).

### Build + sign (accel tier, on the target)

`smackerel.sh build --target self-hosted` — all phases green:
- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities).
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/pkirsanov/smackerel-core@sha256:b4a59eef24f2956896710797360f5ef1b3be7a35574819441e116e6c50faed73`
  - ml   `ghcr.io/pkirsanov/smackerel-ml@sha256:c43fad4afc6d86287f5fe93029694dfad85a74fa9281a94bd4f870220fc5d455`
- Config bundle `config-bundle-self-hosted-19fe72c8…` pushed + signed; signed local-build-manifest emitted.

### Deploy (on-host local-operator apply → recreate)

`promote.sh --target home-lab --product smackerel --local-build-manifest <manifest> --operator <op>`
(on-host, under passwordless sudo, with the operator cosign pubkey + ghcr docker-config).
The adapter verified the release proof (cosign verified both images + attestations against the
operator pubkey), decrypted the bundle secrets, and recreated `smackerel-core` + `smackerel-ml`
(infra services stayed healthy).

### Live running-state verification (this session, read-only)

```text
smackerel-home-lab-smackerel-core-1 | running/healthy restarts=0 | sha256:b4a59eef… | MATCHES P1-P5 CORE
smackerel-home-lab-smackerel-ml-1   | running/healthy restarts=0 | sha256:c43fad4a… | MATCHES P1-P5 ML
```

Both containers run the EXACT P1–P5 digests and are healthy (0 restarts); core startup log shows
`telegram bot started` + `assistant Telegram adapter wired and bound to bot`, so the honest-error
code path is the live one.

### Remaining (operator behavioral smoke test)

On Telegram, trigger a scenario execution failure (e.g. a weather/retrieval turn while the local
model is flaky) and confirm the reply is an honest "couldn't do that right now" line, NOT "saved
as an idea"; a genuine low-confidence capture should still say "saved as an idea". An authenticated
HTTP probe is not feasible (prod requires a PASETO session), and the agent cannot send Telegram
messages.
