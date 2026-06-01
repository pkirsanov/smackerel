# Report — Spec 065 Generic Micro-Tools

Links: [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

## Summary

Planning packet created by `bubbles.plan` on 2026-05-31 for the product-to-planning pass. This report is a scaffold for execution evidence only; no implementation, source tests, config generation, or runtime verification was performed by this planning pass.

## Planning Evidence

- Scope plan created in [scopes.md](scopes.md).
- Scenario contracts created in [scenario-manifest.json](scenario-manifest.json).
- Structured test handoff created in [test-plan.json](test-plan.json).
- User validation baseline created in [uservalidation.md](uservalidation.md).

## Test Evidence

No test evidence is recorded here by `bubbles.plan`. Execution agents must append raw terminal output with `**Phase:**`, `**Command:**`, `**Exit Code:**`, and `**Claim Source:**` fields when they run the planned checks.

## Completion Statement

Planning artifacts are prepared for planning maturity review. Delivery is not claimed in this report.

## Execution Evidence (partial — SCOPE-4 entity_resolve foundation)

**Phase:** implement  
**Agent:** bubbles.implement  
**Date:** 2026-06-01  
**Scope:** 4 (entity_resolve) — partial; SCOPE-4 DoD remains Not Started.  
**Claim Source:** executed.

Added the `entity_resolve` micro-tool source + unit tests to unblock spec 066 SCOPE-4 (legacy keyword surface retirement) which lists entity_resolve as a prerequisite. The change is narrowly scoped:

- New: `internal/agent/tools/microtools/entity_resolve.go` — defines `EntityResolver` interface, `EntityResolveServices` wiring, `entity_resolve` tool registration (input `{input, user_id, scope?, top_k?}`), and resolved/ambiguous/failed envelope construction respecting the spec 065 envelope contract.
- New: `internal/agent/tools/microtools/entity_resolve_test.go` — six unit cases covering resolved (top score ≥ floor), ambiguous (top below floor), zero candidates → failed, resolver error → failed, missing user_id/input rejection, not-configured fail-loud, and top_k clamping to MaxCandidates.

**Command:** `go test -count=1 -timeout 60s -run 'EntityResolve' ./internal/agent/tools/microtools/`  
**Exit Code:** 0  
**Output:** `ok  github.com/smackerel/smackerel/internal/agent/tools/microtools  0.018s`

**Command:** `go test -count=1 -timeout 120s ./internal/agent/tools/microtools/ ./internal/agent/`  
**Exit Code:** 0  
**Output:**
```
ok      github.com/smackerel/smackerel/internal/agent/tools/microtools  0.029s
ok      github.com/smackerel/smackerel/internal/agent   0.133s
```

### What is NOT done (SCOPE-4 DoD remains Not Started)

The following SCOPE-4 DoD items are NOT delivered by this micro-fix and must be routed back to `bubbles.plan` / `bubbles.implement` for a full SCOPE-4 pass:

- Production wiring in `cmd/core` that constructs an `EntityResolver` adapter over the live graph/search substrate and calls `SetEntityResolveServices` at startup.
- Scenario `allowed_tools` updates and prompt-side normalization text removal where `entity_resolve` now owns the behavior.
- Weather prompt-size 40% reduction regression test (`TestWeatherPromptUsesLocationNormalizeAndShrinksByFortyPercent`).
- Integration tests proving user-scoped isolation against the live store (`TestEntityResolveIntegration_*`).
- E2E tests (`TestMicroToolsE2E_EntityResolveClarifiesLowConfidenceLease`, `TestMicroToolsE2E_ComposesWeatherAndUnitConversionWithoutScenarioParsing`).
- Consumer Impact Sweep proving spec 066 can consume the resolver without regex intent parsing.
- Broader unit/integration/e2e suites and artifact lint passing for spec 065.

### SCOPE-1 verification status (NOT marked done)

On-disk inspection (not full DoD verification) shows the SCOPE-1 foundation surface is implemented: `envelope.go` (Envelope, Status, SourceKind, Candidate, Error, ValidateEnvelope, ValidateEnvelopeBytes, CurrentSchemaVersion), `envelope_test.go`, `internal/config/assistant_tools.go` (all 12 required ASSISTANT_TOOLS_* keys with fail-loud `AssistantToolsMissingKeyError`), `internal/config/assistant_tools_test.go`, and the per-tool files all using `agent.RegisterTool` from `init()`. Full SCOPE-1 DoD verification (canary integration test, regression E2E, broader suites, artifact lint) was NOT run by this pass and the SCOPE-1 DoD checkboxes remain `[ ]`. Route to `bubbles.validate` for certification.
