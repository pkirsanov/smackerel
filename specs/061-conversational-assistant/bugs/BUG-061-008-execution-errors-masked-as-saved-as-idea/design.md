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

Same local-operator home-lab path as BUG-061-007 (build → cosign-sign → on-host promote/apply
→ verify running digests + health). Recreate rollout.
