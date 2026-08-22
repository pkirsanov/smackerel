# BUG-061-008 — Design: honest execution errors + systemic prevention (P1–P5)

## Root-cause trace

```
facade.Handle (BandHigh)
  result = executor.Run(...)              # e.g. OutcomeProviderError (weather 5xx / no tool call)
  resp.Status     = translateOutcomeToStatus(OutcomeProviderError)   = StatusUnavailable   (honest)
  resp.ErrorCause = translateOutcomeToErrorCause(OutcomeProviderError)= ErrProviderUnavailable (honest)
  resp.Body       = translateFinalToBody(...)                        = "provider unavailable."
  assembler(result) -> empty Sources (Outcome != OK)
  enforceProvenanceWithSpan(requires_provenance, ...)   # <-- runs UNCONDITIONALLY
     provenance.Enforce: Sources empty + Body non-empty
        -> resp.Status = StatusSavedAsIdea               # masks the honest status
        -> resp.Body   = CanonicalRefusalBody
        -> resp.CaptureRoute = true
  canonicalizeSuccessfulCaptureResponse(resp)            # end of Handle
     CaptureRoute && StatusSavedAsIdea
        -> resp.ErrorCause = ""                           # DISCARDS the honest cause
        -> resp.Body = "saved as an idea — i'll surface it later."
```

The gate exists to stop **fabrication** — a synthesised body with no citations. That is only
meaningful for an **OK** outcome. Running it on a non-OK outcome converts a failure into a
lie.

## Fix (P1) — one guard

In the facade high-band path, gate the provenance-enforce call on the OK outcome:

```go
// before
if assemblerOverride == nil {
    resp = f.enforceProvenanceWithSpan(ctx, f.manifest.RequiresProvenance(scenarioID), ...)
}
// after
if assemblerOverride == nil && result != nil && result.Outcome == agent.OutcomeOK {
    resp = f.enforceProvenanceWithSpan(ctx, f.manifest.RequiresProvenance(scenarioID), ...)
}
```

For a non-OK outcome the pre-computed honest response (`StatusUnavailable` +
`ErrProviderUnavailable` + truthful body) stands; `canonicalizeSuccessfulCaptureResponse`
becomes a no-op (Status ≠ StatusSavedAsIdea), so `ErrorCause` and the honest body survive.
`translateFinalToBody(OutcomeProviderError)` is upgraded from `"provider unavailable."` to a
friendlier truthful line.

### Why this is safe (test impact)

The two behaviours are already separated by outcome in the existing tests:

| Test | Outcome | Meaning | After P1 |
|------|---------|---------|----------|
| `TestFacadeHighBandProvenanceGateRewritesWhenSourcesMissing` | `OutcomeOK` | synthesized body, no sources = fabrication | **unchanged** (gate still fires) |
| `TestFacadeWeatherIntegration_AntiFabrication_MissingProviderTriggersRefusal` | `OutcomeOK` | body without provider_name = fabrication | **unchanged** (gate still fires) |
| `TestFacadeWeatherIntegration_BS006_ProviderUnavailableTriggersRefusal` | `OutcomeProviderError` | execution failure | **updated** → honest `StatusUnavailable` error |

Only `BS006` changes — it is the sole test that asserted masking on a non-OK outcome. Its
contract is refined: provider-unavailable → honest error (matching the BUG-061-007
`/weather` fast-path and the operator's expectation), not a masked capture.

## P2 — cross-scenario invariant test

`facade_execution_error_honesty_test.go`: table-driven over
{`weather_query`, `retrieval_qa`, `recipe_search`} × {`OutcomeProviderError`, `OutcomeTimeout`}
asserting for each: `Status==StatusUnavailable`, `ErrorCause!=""`, `Body!=captureFallbackAcknowledgement`,
`CaptureRoute==false`. Plus a complementary OK+no-sources case per scenario asserting the
fabrication guard STILL fires (so P1 cannot over-correct). This is the mechanical gate that
would have caught weather AND catches every future scenario.

## P3 — execution-error observability

Add `assistantmetrics.ExecutionErrorSurfacedTotal{scenario_id, outcome, transport}`,
incremented in the facade high-band non-OK branch. A dashboard/alert on this counter surfaces
execution failures per scenario proactively (before a user screenshot). Complements the
existing `provenance.ViolationsCounter` (which now only tracks genuine fabrication refusals).

## P4 — deterministic-dispatch seam (documented pattern)

The BUG-061-007 `WithWeatherLookup` seam (an explicit `/weather` shortcut dispatches the
weather tool directly, bypassing the LLM tool-call loop) is the reference pattern for any
explicit slash command whose argument is an unambiguous tool input. Documented in the
assistant design as the recommended approach; not a blanket refactor (scenarios whose
argument needs interpretation stay on the routed path, now made safe by P1).

## P5 — invariant encoded + enforced

The invariant is added to the assistant design/docs and the review checklist:
"execution errors are never rendered as capture/soft-refusal; the provenance gate runs only
on OK outcomes." The P2 test is its mechanical enforcement (a regression re-introducing the
masking fails the suite).

## Deploy

Use the same local-operator deployment contract as BUG-061-007: build, cosign-sign,
promote/apply through `<knb-repo>/<product>/<target>/...`, then verify running digests
and health on `<deploy-host>`. Recreate the rollout.

## Decision — per-outcome `ErrorCause` for terminal non-OK outcomes (2026-08-22, `bubbles.plan`)

Resolves the spec question `bubbles.audit` raised as `D-6` and `bubbles.simplify` recorded as
`D-5`: does a schema failure, an invalid tool return, an input-schema violation, or a loop limit
owe the transport a distinct `ErrorCause`, or is `ErrNone` correct for them?

**Decision: map all four to the existing `contracts.ErrInternalError`. The closed vocabulary does
not grow.** Every code fact below was re-derived from the source tree during this phase.

### First — the terminal set is 4, not 8

`D-5` and `D-6` both state the gap as "8 of the 10 non-OK outcomes". That count is too broad. It
counts every declared non-OK constant, but `translateOutcomeToErrorCause` is only ever reached
with a **terminal** `result.Outcome` (`facade.go:1287`). `internal/agent/executor.go:56-88`
declares 11 constants — `OutcomeOK` plus 10 non-OK — and they divide three ways:

| Class | Members | Why |
|---|---|---|
| **Terminal** (6) | `OutcomeProviderError`, `OutcomeTimeout`, `OutcomeSchemaFailure`, `OutcomeToolReturnInvalid`, `OutcomeInputSchemaViolation`, `OutcomeLoopLimit` | Each is assigned to `result.Outcome` in `executor.go` (`:336,345,408,437,444,455,475,487,502,658`) and each carries a `terminal:` doc comment. |
| **Per-tool-call, never terminal** (3) | `OutcomeAllowlistViolation`, `OutcomeHallucinatedTool`, `OutcomeToolError` | Assigned only to an `ExecutedToolCall` appended to `result.ToolCalls`, each followed by `continue` (`executor.go:535,552,571,585,633`). The loop proceeds; they never become the invocation's outcome. |
| **Terminal but unreachable here** (1) | `OutcomeUnknownIntent` | Produced only by `Bridge.Invoke` (`bridge.go:211,220`), which returns **without** an executor call. `Bridge.Invoke(ctx, env) (*InvocationResult, *RoutingDecision)` cannot satisfy the facade's `Executor` interface, which requires an already-chosen `*agent.Scenario` (`facade.go:66`). The facade's only other result producer, `runOpenKnowledgeDirect`, sets `OutcomeProviderError` or `OutcomeOK` only (`facade.go:1467,1473`). |

Two of the six terminal outcomes are already mapped, so **the real gap is four**:
`OutcomeSchemaFailure`, `OutcomeToolReturnInvalid`, `OutcomeInputSchemaViolation`,
`OutcomeLoopLimit`. This matches the four named in `state.json.nextRequiredOwner`.

The `translateOutcomeToStatus` arm for `OutcomeUnknownIntent` (`facade.go:1778`) is defensive,
consistent with the "defensive — should not happen" comment at `facade.go:1907`. Adding a cause
arm for it would bind a row no production path can exercise.

### Why `ErrInternalError`, and not a new token

1. **The sibling translator already coarsens exactly this way.** `translateFinalToBody`
   (`facade.go:1743-1748`) collapses `OutcomeSchemaFailure`, `OutcomeToolReturnInvalid` and
   `OutcomeInputSchemaViolation` into one body — "something went wrong handling that — please try
   again." — and gives `OutcomeLoopLimit` its own. The user-visible surface **already** treats
   these as one internal-error class. Mapping them to `ErrInternalError` makes the cause agree
   with the body the product already ships, rather than inventing a granularity the body does not
   carry.
2. **The token's own contract fits.** `ErrInternalError` is documented as "an unexpected
   capability-layer error not better described by another cause" (`contracts/response.go:206`).
   An LLM that exhausted its schema-retry budget, a tool returning a value that failed its own
   output schema, an exceeded iteration budget, and an envelope failing its input schema are each
   precisely that.
3. **It does not mislabel — the `D-4` lesson.** `D-4` was severe because a timeout reached the
   transport labelled `no_grounded_answer`. `ErrProviderUnavailable` would repeat that mistake
   here: no external provider fails in a loop-limit or an input-schema violation. Coarse and true
   beats specific and wrong.
4. **Diagnostic granularity is not lost.** `ExecutionErrorSurfacedTotal` is labelled
   `{scenario_id, outcome, transport}` (`internal/assistant/metrics/metrics.go:158-163`), so a
   dashboard still separates a schema failure from a loop limit. `ErrorCause` is the
   transport-facing token; `outcome` is the telemetry-facing one. They are not redundant, and the
   coarse cause costs no alerting fidelity.
5. **`spec.md` P1 requires "a set `ErrorCause`", not a unique one.** Four outcomes sharing one
   honest token satisfies the clause as written.
6. **No downstream churn.** `AllErrorCauses` is unchanged, so no transport renderer and no alert
   rule needs a new arm.

### Alternative considered and rejected

**A distinct caller-fault token for `OutcomeInputSchemaViolation`** (the "400 versus 500"
argument: the envelope failed its contract, so blaming the server is wrong).

Rejected on evidence about who authors the envelope. On the only path that reaches this function,
the facade builds the envelope itself — `facade.go:1018` sets `Source` from the transport,
`RawInput` from the message text and `ScenarioID` from a shortcut prefix or compiled hint, then
assembles `StructuredContext` from its own payload map. No caller-supplied `StructuredContext`
passes through. An input-schema violation on that path is an internal construction defect, so
`ErrInternalError` is the correct label rather than a compromise.

A caller-authored envelope path does exist — `internal/api/agent_invoke.go:200` forwards
`req.StructuredContext` verbatim — but it calls `h.Runner.Invoke` (the `Bridge`) and renders its
own HTTP response, never calling `translateOutcomeToErrorCause`. Should that transport ever want
a caller-fault discriminator, it is a separate decision on a separate surface and does not change
this one.

### Consequence for the sweep

With a cause defined for all six terminal outcomes, `errorOutcomes`
(`facade_execution_error_honesty_test.go:48`) can widen to the full terminal set and
`errorOutcomeCauses` (`:62-65`) gains the four rows. That widening is guarded: the `t.Fatalf` at
`:129-133` already fails if a member joins `errorOutcomes` without a declared cause, so the sweep
cannot silently degrade back to a non-emptiness check. The test's own comment at `:41-46` names
this as "a spec-061 question, not a test edit" — this section is that answer.

The four non-terminal outcomes stay out of the sweep by the classification above, not by
omission. A row for `OutcomeAllowlistViolation`, `OutcomeHallucinatedTool`, `OutcomeToolError` or
`OutcomeUnknownIntent` would assert a path production cannot reach.
