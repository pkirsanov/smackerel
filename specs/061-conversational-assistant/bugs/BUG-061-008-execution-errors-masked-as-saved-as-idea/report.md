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
fabrication-guard tests remain green (the fix does not over-correct). Live `<target>` deploy +
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

## Scenario binding evidence {#scenario-binding-evidence}

Each declared scenario in `scenario-manifest.json` is bound to a named, currently-passing
assertion. This section exists because the DoD previously carried no item that restated a
scenario's own behavioral claim, so the checkboxes could read complete while nothing tied them
to a test. Binding was re-derived by reading the tests, not by trusting the earlier transcript.

| Scenario | Binding assertion | What the test actually asserts |
|---|---|---|
| `SCN-061-008-01` (provider error) | `TestHighBandNeverMaskedAsSavedAsIdea`, rows `<scenario>/provider_error` | `Status != StatusSavedAsIdea`; `Status == StatusUnavailable`; `Body != captureFallbackAcknowledgement`; `CaptureRoute == false`; `ErrorCause != ""` |
| `SCN-061-008-02` (timeout) | same table, rows `<scenario>/timeout` — driven by `errorOutcomes = {OutcomeProviderError, OutcomeTimeout}` | identical five assertions, so a timeout cannot regress independently of a provider error |
| `SCN-061-008-03` (OK + no sources still refuses) | `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly` | `ErrorCause == ErrNoGroundedAnswer`; `Body == CanonicalRefusalBodyFor(RefusalDefault)`, which is what proves the uncited answer never leaks; not `StatusSavedAsIdea`; not a capture |
| `SCN-061-008-01/02/03` (the matrix) | `TestHighBandNeverMaskedAsSavedAsIdea` × `TestRequiresProvenanceScenarios_ClosedOverSST` | the sweep iterates `requiresProvenanceScenarios`, and the closure test reads `config/assistant/scenarios.yaml` and fails both when an SST-gated scenario is missing from the sweep and when the sweep names one the SST does not gate |

Two qualifications are recorded rather than smoothed over, because each changes what the
checkmark means:

1. The quantifier in the matrix item ("every `requires_provenance` scenario") is only as strong
   as the closure test, and that test arrived with BUG-061-009, not with this packet. When
   BUG-061-008 shipped, the sweep was a hardcoded three-element list and `open_knowledge` sat
   outside it. The claim is true of the repository as it stands today, which is what the DoD
   item asserts; it was not proven at the time this packet was written.
2. `SCN-061-008-03`'s refusal *shape* was superseded by BUG-061-009. This packet's gate rewrote
   an OK-but-uncited turn to `StatusSavedAsIdea`, which BUG-061-009 identified as the same
   masking defect one layer down. The claim the DoD item makes — the guard still fires and the
   uncited body never reaches the user — survives that change and is what the test asserts.

Command and result:

```text
$ ./smackerel.sh test unit --go --go-run 'HighBandNeverMaskedAsSavedAsIdea|ExecutionErrorHonesty|RequiresProvenanceScenarios_ClosedOverSST|CanonicalRefusalBodyFor' --verbose
exit: 0
lines: 584
sha256: d884077a043f0e0ac543701c15510a7f398ffeda1de84c2af5304a33189764f8
[go-unit] go test ./... finished OK
```

Captured via `evidence-capture.sh`; the hash covers every produced line and is re-derivable
with `--verify`. The run used `SMACKEREL_SKIP_HOST_PREFLIGHT=1`, the documented host-preflight
opt-out, disclosed here rather than left implicit.

## Test Evidence

Full suite green for the affected packages (`go test ./... finished OK`, exit 0); the
`internal/assistant` and `internal/agent` packages report `ok`. Per-scenario PASS lines are in
the P1/P2 blocks above.

### Code Diff Evidence

The fix commit of record on `main` is
`44dc0c94b165371ec8b68e6213a964619851fe41` (authored 2026-07-22 22:20:29 +0000), subject
`fix(061): BUG-061-008 — execution errors no longer masked as 'saved as an idea' (P1-P5)`.

Every subject appears twice in the log. Only one commit of each pair is reachable from `HEAD`;
the other is orphaned. The reachable one is the commit of record, and it was selected by
execution rather than by taking the first line of the log:

```text
$ git log --oneline --all --grep='BUG-061-008'
b6e44bc9 docs(061): BUG-061-008 — record <deploy-target> deploy + live infra-verification (P1-P5)
aa401981 docs(061): BUG-061-008 — record <deploy-target> deploy + live infra-verification (P1-P5)
0281bdca fix(061): BUG-061-008 — execution errors no longer masked as 'saved as an idea' (P1-P5)
44dc0c94 fix(061): BUG-061-008 — execution errors no longer masked as 'saved as an idea' (P1-P5)

$ git merge-base --is-ancestor <sha> HEAD   # per candidate, exit 0 = reachable
0281bdca : ORPHANED (not an ancestor of HEAD) : branches=[none]
44dc0c94 : ANCESTOR-OF-HEAD : branches=[ ... * main   remotes/origin/main ]
b6e44bc9 : ORPHANED (not an ancestor of HEAD) : branches=[none]
aa401981 : ANCESTOR-OF-HEAD : branches=[ ... * main   remotes/origin/main ]
```

The two `docs(061)` subjects are quoted with the concrete deployment-target name redacted to
`<deploy-target>` per the product-deployment-boundary policy; the SHAs are unaltered and remain
the verifiable anchor, so `git show <sha>` resolves the real subject.

Per-file delta of the commit of record, as executed:

```text
$ git show --numstat --format='' 44dc0c94
14      0       .github/copilot-instructions.md
69      0       docs/smackerel.md
22      9       internal/assistant/facade.go
163     0       internal/assistant/facade_execution_error_honesty_test.go
17      17      internal/assistant/facade_weather_integration_test.go
16      0       internal/assistant/metrics/metrics.go
49      0       specs/.../bug.md
96      0       specs/.../design.md
123     0       specs/.../report.md
10      0       specs/.../scenario-manifest.json
125     0       specs/.../scopes.md
70      0       specs/.../spec.md
64      0       specs/.../state.json
14      0       specs/.../uservalidation.md

$ git show --stat --format='' 44dc0c94   # trailing summary line
 14 files changed, 852 insertions(+), 26 deletions(-)
```

The `specs/...` rows are elided to their basenames for width; each is this packet's own
directory under `specs/061-conversational-assistant/bugs/`.

Non-artifact delta — the six files that carry the behavior and governance change:

| File | + | − | Role in the fix |
|------|---|---|-----------------|
| `internal/assistant/facade.go` | 22 | 9 | gates the provenance rewrite on `Outcome == OutcomeOK`; friendly truthful non-OK body; P3 metric increment |
| `internal/assistant/facade_execution_error_honesty_test.go` | 163 | 0 | the cross-scenario honesty table, the OK+no-sources guard cases, and the metric assertion |
| `internal/assistant/facade_weather_integration_test.go` | 17 | 17 | `BS006` re-pointed from the capture contract to the honest-error contract |
| `internal/assistant/metrics/metrics.go` | 16 | 0 | `ExecutionErrorSurfacedTotal{scenario_id, outcome, transport}` |
| `docs/smackerel.md` | 69 | 0 | §3.8.6 — the failure-honesty invariant (P5) and the deterministic-dispatch seam pattern (P4) |
| `.github/copilot-instructions.md` | 14 | 0 | the review rule citing the P2 test as mechanical enforcement |

The remaining 8 files are this packet's own artifacts, which are planning surface and are not
counted as behavior delta.

#### Traceability finding — the recorded `sourceSha` is orphaned {#code-diff-orphaned-sha}

`state.json` records `deployment.sourceSha: "19fe72c8"`. That object exists in the object
database but is not an ancestor of `HEAD` and no branch contains it, so it is orphaned and does
not even appear in the `--all` log above. Its tree is byte-identical to the commit of record:

```text
$ git rev-parse 19fe72c8^{tree}   -> 3481978a797dafbb3e50d3aa7d7bae295aa15f30
$ git rev-parse 44dc0c94^{tree}   -> 3481978a797dafbb3e50d3aa7d7bae295aa15f30
$ git diff --stat 19fe72c8 44dc0c94   (no output — identical trees)
$ git diff --stat 0281bdca 44dc0c94   (no output — identical trees)
```

The running binary therefore corresponds to the code on `main`, and the defect is traceability
only. This section cites `44dc0c94` for that reason. The recorded `sourceSha` was left as
measured rather than silently rewritten, because this pass can prove tree equivalence but
cannot prove which object the build host consumed. Same finding class as the sibling packet
BUG-061-007, recorded at `report.md#code-diff-orphaned-sha` there.

## Discovered Issues

| # | Date | Finding | Owner |
|---|------|---------|-------|
| D-1 | 2026-08-21 | The `## P2 evidence` transcript above quotes test names that no longer exist in the tree: `TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea` is now `TestHighBandNeverMaskedAsSavedAsIdea`, and `TestExecutionErrorHonesty_OKNoSourcesStillRefuses` is now `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly`. Both were renamed by BUG-061-009 when it widened the invariant. The transcript was captured from a real run at the time and is left unaltered, because editing a recorded transcript to match today's names would fabricate evidence for a run that never produced it. The current binding is recorded separately under "Scenario binding evidence". | `bubbles.regression` (re-capture the P2 block against current test names, or supersede it with the scenario-binding section) |
| D-2 | 2026-08-21 | `deployment.sourceSha` `19fe72c8` is orphaned — see `#code-diff-orphaned-sha`. Tree equivalence with the commit of record is proven; which object the build host consumed is not. | `bubbles.devops` (re-point `sourceSha` to `44dc0c94` only if the build host's consumed object can be confirmed) |
| D-3 | 2026-08-21 | No `e2e-api`/`e2e-ui` test drives these scenarios against a provider-enabled assistant stack. `scenario-manifest.json` declares `requiredTestType: "unit"` for all four, so the unit binding satisfies the declared contract, but the scenario-level end-to-end path is unproven. A Test Plan row naming this as end-to-end regression coverage was deliberately not written: `planning-checks.sh:72` text-matches that phrase in any table row, so putting it on a `unit`-category row would green the gate while misdescribing the category. | assistant e2e harness owner — outside this bug's six-file fix surface |

## Deploy + Live Verification (`<target>`) {#deploy-verify}

The P1–P5 fix (sourceSha `19fe72c8`) was built, operator-cosign-signed, deployed on-host, and
verified running healthy this session (`local-operator` trust model).

### Build + sign (knb-owned configured tier on `<deploy-host>`)

`smackerel.sh build --target <target>` — all phases green:
- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities).
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/<operator>/smackerel-core@sha256:b4a59eef24f2956896710797360f5ef1b3be7a35574819441e116e6c50faed73`
  - ml   `ghcr.io/<operator>/smackerel-ml@sha256:c43fad4afc6d86287f5fe93029694dfad85a74fa9281a94bd4f870220fc5d455`
- Config bundle `config-bundle-<target>-19fe72c8…` pushed + signed; signed local-build-manifest emitted.

### Deploy (on-host local-operator apply → recreate)

`<knb-repo>/scripts/deploy/promote.sh --target <target> --product smackerel --local-build-manifest <manifest> --operator <operator>`
(on-host, under passwordless sudo, with the operator cosign pubkey + ghcr docker-config).
The adapter verified the release proof (cosign verified both images + attestations against the
operator pubkey), decrypted the bundle secrets, and recreated `smackerel-core` + `smackerel-ml`
(infra services stayed healthy).

### Live running-state verification (this session, read-only)

```text
smackerel-<target>-smackerel-core-1 | running/healthy restarts=0 | sha256:b4a59eef… | MATCHES P1-P5 CORE
smackerel-<target>-smackerel-ml-1   | running/healthy restarts=0 | sha256:c43fad4a… | MATCHES P1-P5 ML
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
