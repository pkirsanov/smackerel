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

## Regression phase evidence {#regression-evidence}

Executed by `bubbles.regression` on 2026-08-21 against HEAD `d405f24a` (the planning-repair
commit), working tree clean at entry. Every command below was run in this session from the
repository root via the repo CLI; each block is bounded by `evidence-capture.sh`, whose sha256
covers every produced line and is re-derivable with `--verify`.

All Go runs used `SMACKEREL_SKIP_HOST_PREFLIGHT=1`. That is the documented host-preflight
opt-out at `smackerel.sh:715`; the host disk check refuses at roughly 34 GB free against a
40 GB threshold, and `test unit --go` builds no image, so the opt-out is scoped to a check that
does not apply. No shared cache was pruned. The opt-out is disclosed here rather than left
implicit.

### Suites executed

| Command | Exit | Lines | sha256 |
|---|---|---|---|
| `./smackerel.sh test unit --go` (full, restored tree) | **0** | 207 | `102eb9c7a9d918d9ff4305e8128a8f4f01acbef71dbd7fc97008e503ce8f6e7b` |
| `./smackerel.sh test unit --go --go-run 'HighBandNeverMaskedAsSavedAsIdea\|ExecutionErrorHonesty\|RequiresProvenanceScenarios_ClosedOverSST\|CanonicalRefusalBodyFor'` (mutation control) | **0** | 216 | `e24bd7a7cf4d3720fd26dbd5e6bcc04d6b5aab85a1345eda46356fd3f140760e` |

`go test ./... finished OK` on the full run. The integration suite is recorded in its own
block below.

### Mutation testing — do the four per-scenario DoD claims actually hold?

The DoD added four items that restate a scenario's behavioral claim and bind it to a named
assertion. A passing test does not prove a claim is *enforced*; it only proves the test agrees
with today's code. Each claim was therefore checked by MUTATION: introduce a targeted change
that reintroduces the masking defect the claim forbids, run the claim's own binding assertion,
and record whether the assertion kills the mutant. A claim whose mutant survives is
over-checked — the checkbox asserts more than the test enforces.

Pre-mutation bytes, recorded before the first mutant:

```text
139510ffc375b310e2dd8c4309afe7b07e085edb  internal/assistant/facade.go
482529251894b7abac1e5b2678b564c974964b01  internal/assistant/facade_execution_error_honesty_test.go
```

| Mutant | Mutation applied | Claim under test | Binding assertion run | Exit | Verdict |
|---|---|---|---|---|---|
| M1 | gate condition widened to `OutcomeOK \|\| OutcomeProviderError` — the provenance gate runs again on a provider error | SCN-061-008-01 | `TestHighBandNeverMaskedAsSavedAsIdea` | **0** | 🔴 **SURVIVED** |
| M2 | gate condition widened to `OutcomeOK \|\| OutcomeTimeout` — the gate runs again on a timeout | SCN-061-008-02 | `TestHighBandNeverMaskedAsSavedAsIdea` | **0** | 🔴 **SURVIVED** |
| M3 | gate condition inverted to `Outcome != OutcomeOK` — the fabrication guard no longer runs on an OK outcome | SCN-061-008-03 | `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly` | **1** | 🟢 KILLED |
| M4 | `open_knowledge` removed from `requiresProvenanceScenarios` — the sweep set is no longer closed over the SST | SCN-061-008-01/02/03 (the matrix) | `TestRequiresProvenanceScenarios_ClosedOverSST` | **1** | 🟢 KILLED |

Bounded captures: M1 narrow `b5ece537458f3de1abe6693d2b204c1a221bd7517c3a3640ed60ce9f9fa4798c`;
M2 `9acfa8a3f66e14b287fdff0dbcdd26bc7d06f354c4c41d0f54028d9a76f516b9`;
M3 `b61060ae7e0fec1d6917209582598d17b68f97c51dbd9e4cd74eac65bd0085fd`;
M4 `6e36a3aa962bd1937726d21b1796d751b9918a692709ef4e5b16035a4fcc9ccb`.

A method correction is recorded rather than hidden. The first M1 run used a wider `--go-run`
filter (`HighBandNeverMaskedAsSavedAsIdea|ExecutionErrorHonesty`) than the M2 run, exited 1,
and would have read as a kill. That comparison was invalid — the two mutants were not measured
against the same assertion. M1 was re-run against the identical narrow filter and exited 0.
The wide-filter kill was then attributed by running M1 against
`TestExecutionErrorHonesty_MetricIncrements` alone: exit **1**, capture
`76059cd03a16088526ea632c71994e4e6cb90c1b2a4597b9194edbc84b054642`.

### Finding R-1 — the named regression gate does not kill the defect it is named for

Two of the four claims survive. The mechanism is specific and worth stating exactly, because
the packet still behaves correctly at runtime; what fails is the *proof*.

`TestHighBandNeverMaskedAsSavedAsIdea` asserts five properties of the response: `Status` is not
`StatusSavedAsIdea`, `Status` is `StatusUnavailable`, `Body` is not the capture
acknowledgement, `CaptureRoute` is false, and `ErrorCause` is non-empty. When M1 or M2 lets the
provenance gate fire on a non-OK outcome, the gate does rewrite the response to
`StatusSavedAsIdea` with `CaptureRoute=true` — the original defect. But
`canonicalizeSuccessfulCaptureResponse` then runs, and BUG-061-009 scoped the capture
acknowledgement to band LOW (`facade.go:1832`, `if band != BandLow`). A band-high response
still carrying the capture shape is converted into the honest refusal. All five assertions pass
again.

So the sweep is a check on the response *shape at the boundary*, not on the P1 guard. After
BUG-061-009 landed, its band-scoping became a second, independent defence sitting downstream of
the P1 guard — and it is strong enough to hide the P1 guard's removal from the very test this
packet names as its mechanical enforcement.

One consequence is a real behavioral loss the suite does not catch. Under M2 the gate replaces
the true cause with `ErrNoGroundedAnswer`, so a timeout reaches the transport and alerting
labelled "no grounded answer". The sweep only asserts `ErrorCause != ""`, so the substitution
passes. That is precisely the clause the SCN-061-008-02 DoD item claims — *"the timeout cause
survives to the transport rather than being discarded by the gate"* — and no assertion in the
tree enforces it.

**Which DoD items are over-checked.** Both are in Scope 1 of `scopes.md`:

- SCN-061-008-01 — the "never rendered as saved as an idea" half is enforced (by the downstream
  band scoping, and separately by the P3 metric test, which is what actually killed M1). The
  item's attribution in "Scenario binding evidence" to `TestHighBandNeverMaskedAsSavedAsIdea`
  overstates what that test enforces.
- SCN-061-008-02 — same, plus the "the timeout cause survives" clause is unenforced and is
  false under M2. This is the stronger over-check of the two.

The user-visible invariant this packet ratified — *an execution error must never render as
"saved as an idea"* — is **not** falsified by this finding. Two independent mechanisms still
hold it. What the finding falsifies is the claim that the named sweep is what holds it. Sibling
packets that cite this packet's sweep as their guarantee are citing a check weaker than they
assume. Filed as D-4, owner `bubbles.test`; no code is at fault, so no fix cycle is opened here.

**SUPERSEDED 2026-08-22 by `bubbles.test` (commit `f3b80e22`).** The analysis above is left
unaltered — it was accurate for the tree it was run against, and editing a recorded finding to
match a later tree would fabricate evidence. It no longer describes the current tree: the sweep
now asserts the exact expected `ErrorCause` per row, M1 and M2 were re-run and both exit 1
(KILLED), and the "the timeout cause survives to the transport" clause is enforced by an
assertion rather than merely believed. Both DoD items were re-checked and Scope 1 returned to
Done. The section heading's present tense reads as still-open; it is not. Current state at
`#d4-remediation`.

### Cross-spec coherence

| Check | Result |
|---|---|
| Does this packet contradict the band-LOW-only capture-ack rule (BUG-061-009 / INV-HB-REFUSAL)? | **No.** `facade.go:1825-1832` carries the band scoping intact; this packet's guard sits upstream of it and the two compose. The gate narrows *when* the capture shape may be produced; the canonicalisation narrows *which band* may keep it. |
| BUG-061-006 (duplicate/contradictory capture ack) | Present at `specs/061-conversational-assistant/bugs/BUG-061-006-duplicate-contradictory-capture-ack`. Relies on exactly one capture-ack shaping path; this packet adds no second one. |
| BUG-061-007 (weather shortcut masked) | Present at `.../BUG-061-007-weather-shortcut-masked-as-saved-as-idea`. Its deterministic-dispatch seam is documented by this packet's Scope 4 and is unmodified here. |
| Refusal taxonomy free of the capture ack | `grep 'saved as an idea' internal/assistant/contracts/refusal.go` returns exactly **one** line, `refusal.go:56`, and it is a prohibitive comment (*"NOT a 'saved as an idea' capture tail"*), not a refusal body. Recorded as one match rather than claimed as zero. |
| `requires_provenance` closure proof | Confirmed to have arrived with **BUG-061-009**, not this packet — `facade_high_band_invariant_coverage_test.go` header names SCN-061-009-02. M4 shows it is genuinely load-bearing today. The matrix DoD item is true of the tree as it stands and was not proven when this packet shipped, which "Scenario binding evidence" already qualifies. |

### Restoration proof

Every mutant was reverted. Post-restore bytes equal the pre-mutation bytes exactly, and the
whole working tree carries zero residual diff:

```text
$ git hash-object internal/assistant/facade.go
139510ffc375b310e2dd8c4309afe7b07e085edb          # == pre-mutation
$ git hash-object internal/assistant/facade_execution_error_honesty_test.go
482529251894b7abac1e5b2678b564c974964b01          # == pre-mutation
$ git status --porcelain
                                                   # empty
$ git diff --stat
                                                   # empty
```

The full unit suite above was run **after** restoration, so its exit 0 describes the restored
tree and not a mutated one.

## D-4 remediation — the sweep now binds its clause {#d4-remediation}

Owner `bubbles.test`, 2026-08-22. Fix commit `f3b80e22`. All commands below were executed
this session against this working tree.

**What changed.** `TestHighBandNeverMaskedAsSavedAsIdea` asserted only
`resp.ErrorCause != ""`. Each row now also asserts the **exact** cause that row must carry,
read from a hand-written table:

```go
var errorOutcomeCauses = map[agent.Outcome]contracts.ErrorCause{
	agent.OutcomeProviderError: contracts.ErrProviderUnavailable,
	agent.OutcomeTimeout:       contracts.ErrProviderUnavailable,
}
```

and, per row, `if resp.ErrorCause != tc.wantCause { t.Errorf(...) }`. The `ok_uncited` row
declares `contracts.ErrNoGroundedAnswer`. A row whose outcome has no entry in the table calls
`t.Fatalf` rather than silently degrading back to a non-emptiness check. The table is written
by hand on purpose: reading it from `translateOutcomeToErrorCause` would assert the production
mapping against itself and could never detect a substitution. The five pre-existing assertions
are unchanged — this is an addition.

**Correction to the D-4 remedy text.** That text asked for "the timeout cause for
`OutcomeTimeout`". No such constant exists. `contracts.ErrorCause` is a closed vocabulary of
seven non-zero values (`response.go:194-229`) with no timeout member, and
`facade.go:1799-1806` maps `OutcomeProviderError` **and** `OutcomeTimeout` to the same
`ErrProviderUnavailable`. Both rows therefore expect `provider_unavailable`. Inventing a
timeout-specific constant to match the remedy wording would have changed the shipped contract
to satisfy a note; the note is corrected instead.

### Mutants re-run — both now KILLED {#d4-mutants}

Same two mutants as `#regression-evidence`, applied at `facade.go:1367` (the gate whose
`OutcomeOK`-only condition is the P1 fix), one at a time:

- **M1** — `(result.Outcome == agent.OutcomeOK || result.Outcome == agent.OutcomeProviderError)`
- **M2** — `(result.Outcome == agent.OutcomeOK || result.Outcome == agent.OutcomeTimeout)`

Command for every run below (the disk preflight refuses on this host at ~35 GB free against a
40 GB floor; `test unit --go` builds no image, and the opt-out is documented at
`smackerel.sh:715` — disclosed, not silent):

```text
SMACKEREL_SKIP_HOST_PREFLIGHT=1 ./smackerel.sh test unit --go \
  --go-run 'TestHighBandNeverMaskedAsSavedAsIdea'
```

| Run | Tree | Exit | Verdict |
|-----|------|------|---------|
| baseline | unmutated, pre-fix test | `0` | green |
| M1 | provider-error re-masked, pre-fix test | `0` | **SURVIVED** (D-4 reproduced) |
| M2 | timeout re-masked, pre-fix test | `0` | **SURVIVED** (D-4 reproduced) |
| clean | unmutated, strengthened test | `0` | green — no false positive |
| M1 | provider-error re-masked, strengthened test | `1` | **KILLED** |
| M2 | timeout re-masked, strengthened test | `1` | **KILLED** |

M1 against the strengthened test:

```text
M1_AFTER_EXIT=1
--- FAIL: TestHighBandNeverMaskedAsSavedAsIdea (0.01s)
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/provider-error (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band provider-error. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/provider-error (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band provider-error. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/provider-error (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band provider-error. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/provider-error (0.01s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band provider-error. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
FAIL	github.com/smackerel/smackerel/internal/assistant	0.585s
```

M2 against the strengthened test:

```text
M2_AFTER_EXIT=1
--- FAIL: TestHighBandNeverMaskedAsSavedAsIdea (0.00s)
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/weather_query/timeout (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band timeout. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/retrieval_qa/timeout (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band timeout. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/recipe_search/timeout (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band timeout. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
    --- FAIL: TestHighBandNeverMaskedAsSavedAsIdea/open_knowledge/timeout (0.00s)
        facade_execution_error_honesty_test.go:160: ErrorCause = "no_grounded_answer"; want "provider_unavailable" for a high-band timeout. A substituted cause mislabels this failure to the transport and to alerting even when the response shape looks honest
FAIL	github.com/smackerel/smackerel/internal/assistant	0.543s
```

The structured turn log emitted during the same two runs shows the mislabel reaching the
observable surface directly, and carries its own control — under M1 the timeout row is still
correct, and under M2 the provider-error row is still correct, so the delta is attributable to
the mutant and not to the harness:

```text
# M1 run — provider-error mislabelled, timeout still correct
assistant_turn user_id=u-weather_query/provider-error ... band=high status=unavailable error_cause=no_grounded_answer ... outcome=provider-error
assistant_turn user_id=u-weather_query/timeout        ... band=high status=unavailable error_cause=provider_unavailable ... outcome=timeout

# M2 run — timeout mislabelled, provider-error still correct
assistant_turn user_id=u-weather_query/provider-error ... band=high status=unavailable error_cause=provider_unavailable ... outcome=provider-error
assistant_turn user_id=u-weather_query/timeout        ... band=high status=unavailable error_cause=no_grounded_answer ... outcome=timeout
```

`outcome=timeout` carrying `error_cause=no_grounded_answer` is verbatim the failure mode the
SCN-061-008-02 DoD item claims and that nothing in the tree previously detected.

### Restoration proof {#d4-restoration}

`facade.go` was the only production file mutated. Restored and proven byte-identical by
content hash and by an empty diff, with the test file the sole remaining modification:

```text
POST_RESTORE facade.go = 139510ffc375b310e2dd8c4309afe7b07e085edb
PRE_MUTATION facade.go = 139510ffc375b310e2dd8c4309afe7b07e085edb
$ git status --porcelain
 M internal/assistant/facade_execution_error_honesty_test.go
$ git --no-pager diff --stat internal/assistant/facade.go
(no output — byte-identical)
```

Full Go unit suite on the restored tree, run **after** restoration:

```text
RESTORED_FULL_EXIT=0
ok_packages=148
(no FAIL lines)
ok  	github.com/smackerel/smackerel/internal/assistant	0.744s
```

**Scope of the claim.** This closes the attribution gap only. The delivered behavior was never
in question and is unchanged — `facade.go` is byte-identical to the commit of record. What is
new is that `TestHighBandNeverMaskedAsSavedAsIdea` now fails when the masking is reintroduced,
so the two DoD items may name it as their binding. The three other mechanisms that already
held the invariant (band-low-only canonicalisation, the P3 metric test, the sibling
OK-no-sources test) are untouched.

## Discovered Issues

| # | Date | Finding | Owner |
|---|------|---------|-------|
| D-4 | 2026-08-21 | `TestHighBandNeverMaskedAsSavedAsIdea` does not kill a reintroduction of the P1 masking defect: mutants M1 (provider error) and M2 (timeout) both re-enable the provenance gate on a non-OK outcome and the test still exits 0, because BUG-061-009's band-LOW-only `canonicalizeSuccessfulCaptureResponse` converts the resulting capture shape back into an honest refusal downstream. The sweep asserts only `ErrorCause != ""`, so it also misses that the gate substitutes `ErrNoGroundedAnswer` for the true timeout/provider cause — the exact clause the SCN-061-008-02 DoD item claims. The invariant itself still holds via two independent mechanisms (the band scoping, and the P3 metric test, which does kill M1); what is unproven is the attribution. **Consequence recorded 2026-08-21 by `bubbles.regression`:** the two Scope 1 DoD items whose binding this falsifies — SCN-061-008-01 and SCN-061-008-02 — were **UNCHECKED** in `scopes.md`. A checked DoD item that mutation proves unbound is a false green, and this packet's own subject is a masking defect, so tolerating a masked test weakness here would be the wrong precedent. The delivered behavior is unchanged and still holds; only the two attribution claims are withdrawn. SCN-061-008-03 and the Scope 2 matrix item stay checked — mutants M3 and M4 were KILLED, so those are genuinely bound. **Remedy for the owner:** assert the specific expected `ErrorCause` per outcome row (`ErrProviderUnavailable` for `OutcomeProviderError`, the timeout cause for `OutcomeTimeout`) instead of mere non-emptiness; that makes the sweep kill M1 and M2 directly and re-earns both checkmarks. Full analysis at `#regression-evidence`. **CLOSED 2026-08-22 by `bubbles.test` in commit `f3b80e22`** — the remedy was applied: each sweep row now asserts its exact expected `ErrorCause` from a hand-written `errorOutcomeCauses` table. M1 and M2 were re-run and both now exit 1 (**KILLED**, from 0/**SURVIVED**), the unmutated tree stays green, and `facade.go` was restored byte-identically (`139510ff…`). One correction to the remedy text above: there is no timeout-specific cause constant — the closed vocabulary maps `OutcomeProviderError` and `OutcomeTimeout` both to `ErrProviderUnavailable`, so both rows expect `provider_unavailable`; a constant was not invented to match the wording. Closure evidence at `#d4-remediation`. | `bubbles.test` (closed) |
| D-1 | 2026-08-21 | The `## P2 evidence` transcript above quotes test names that no longer exist in the tree: `TestExecutionErrorHonesty_NonOKNeverMaskedAsSavedAsIdea` is now `TestHighBandNeverMaskedAsSavedAsIdea`, and `TestExecutionErrorHonesty_OKNoSourcesStillRefuses` is now `TestExecutionErrorHonesty_OKNoSourcesRefusesHonestly`. Both were renamed by BUG-061-009 when it widened the invariant. The transcript was captured from a real run at the time and is left unaltered, because editing a recorded transcript to match today's names would fabricate evidence for a run that never produced it. The current binding is recorded separately under "Scenario binding evidence". **CLOSED 2026-08-21 by `bubbles.regression`** — superseded, not rewritten: `#regression-evidence` carries fresh bounded captures run against the current test names, so no reader depends on the stale block for current binding. | `bubbles.regression` (closed) |
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
