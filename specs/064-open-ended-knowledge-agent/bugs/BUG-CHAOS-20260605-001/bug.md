# BUG-CHAOS-20260605-001: Open-Knowledge Routing Tests Fail with Relative `AGENT_SCENARIO_DIR`

## Status

Fixed — chaos verified. Standalone promotion remains validation-owned.

## Severity

P3 — Low test-infrastructure resilience defect. Production runtime is
unaffected; the chaos finding only manifests when the integration
tests are invoked outside the `./smackerel.sh test integration`
wrapper (for example, an IDE-driven `go test`, an ad-hoc reproduction
loop from the repo root, or any future CI step that calls `go test`
directly).

## Source

- Agent: bubbles.chaos (Round 9 of 20 of the stochastic-quality-sweep)
- Parent observation: OBS-037-CHAOS1-X1 (recorded by Round 1, routed
  to spec 064 because the failing test lives under
  `tests/integration/agent/openknowledge_routing_test.go`)
- Discovered at: 2026-06-05
- Feature: specs/064-open-ended-knowledge-agent
- Workflow mode: chaos-hardening (parent-expanded child mode under
  stochastic-quality-sweep)

## Symptom

Both `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` and
`TestOpenKnowledgeRouting_ScenarioHealthProbe` consume
`AGENT_SCENARIO_DIR` raw via `os.Getenv` and pass it directly to
`agent.DefaultLoader().Load(scenarioDir, ...)`.

`config/generated/dev.env` and `config/generated/test.env` declare
`AGENT_SCENARIO_DIR=config/prompt_contracts` (repo-relative). `go test`
sets the per-package working directory to the package directory
(`tests/integration/agent/`), which does NOT contain a
`config/prompt_contracts/` subtree. The loader then takes the spec
037 BS-001 "missing dir ≡ empty set" path and returns zero registered
scenarios with no fatal error. Both tests subsequently fail:

- `TestOpenKnowledgeRouting_ScenarioHealthProbe` →
  `open_knowledge scenario absent from scenario dir`
- `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` →
  `open_knowledge scenario not loaded from config/prompt_contracts`

`./smackerel.sh test integration` papers over this by rewriting the
env var to `/workspace/${path}` before exec'ing the test container
(see `smackerel.sh:913-918`), so the wrapper path stays green. Any
direct invocation hits the failure.

## Reproduction

From the repo root, with the live test stack NOT required (the
ScenarioHealthProbe variant skips the ML sidecar):

```bash
AGENT_SCENARIO_DIR=config/prompt_contracts \
  ML_SIDECAR_URL=http://stub.invalid \
  AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge \
  SMACKEREL_AUTH_TOKEN=stub \
  go test -count=1 -tags=integration \
    -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' \
    ./tests/integration/agent/...
```

Observed (2026-06-05, Round 9 chaos probe):

```text
--- FAIL: TestOpenKnowledgeRouting_ScenarioHealthProbe (0.00s)
    openknowledge_routing_test.go:164: open_knowledge scenario absent from scenario dir
FAIL
FAIL    github.com/smackerel/smackerel/tests/integration/agent  0.153s
FAIL
```

## Expected Behavior

The routing integration tests MUST resolve `AGENT_SCENARIO_DIR` to an
absolute path against the caller-supplied root before handing it to
the loader, so that any cwd the Go test runner picks produces the
same successful scenario load. If the resolved path does not exist
or is not a directory, the test MUST fail loudly with the original
relative value AND the resolved absolute value, so an operator can
see exactly what was tried.

## Actual Behavior

The tests pass the env-var string through unmodified. Outside the
`smackerel.sh` wrapper, the loader silently returns zero scenarios,
and the test surface manifests as a spurious "missing
open_knowledge" assertion failure that hides the real configuration
issue.

## Impact

- IDE-launched `go test` runs against this package fail without
  obvious explanation.
- Any future CI matrix or test-sharding change that bypasses the
  `smackerel.sh` wrapper silently regresses this surface.
- Local reproductions during incident response are slower and more
  error-prone because the failure mode does not point at the actual
  root cause.

## Audit Rework

None at the time of fix.
