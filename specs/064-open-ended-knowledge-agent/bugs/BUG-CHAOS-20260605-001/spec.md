# Spec: BUG-CHAOS-20260605-001 — Open-Knowledge Routing Tests Must Resolve `AGENT_SCENARIO_DIR` to Absolute

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## 1. Problem Statement

The open-knowledge routing integration tests at
`tests/integration/agent/openknowledge_routing_test.go` consume
`AGENT_SCENARIO_DIR` raw from the process environment and pass it
to `agent.DefaultLoader().Load(scenarioDir, ...)`. The committed
SST value in `config/generated/{dev,test}.env` is
`config/prompt_contracts` (repo-relative). When `go test` runs the
package, the working directory is set to the package directory,
which contains no `config/prompt_contracts/` subtree, so the loader
returns zero scenarios and the tests fail with misleading
"open_knowledge scenario absent" messages.

The runtime stack itself is unaffected — production binaries do not
get cwd-rebased mid-flight, and the `smackerel.sh test integration`
wrapper rewrites the path to `/workspace/${path}` before exec'ing
the test container. The defect is scoped strictly to the
integration-test invocation surface for spec 064's routing tests.

## 2. Outcome Contract

**Intent:** A developer or operator running
`go test ./tests/integration/agent/...` with the committed
`AGENT_SCENARIO_DIR=config/prompt_contracts` value (or any other
relative path) from any working directory under the repo root MUST
see the open-knowledge routing tests load the real scenarios and
either succeed, skip (when live dependencies are absent), or fail
with a message that names BOTH the supplied relative path AND the
resolved absolute path.

**Success Signal:**
- `TestOpenKnowledgeRouting_ScenarioHealthProbe` with a relative
  `AGENT_SCENARIO_DIR` and a stub `ML_SIDECAR_URL` either passes
  (when the live test stack is running and scenarios load) or
  fails with an error string that contains the resolved absolute
  path.
- `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` with a
  relative `AGENT_SCENARIO_DIR` and a stub `ML_SIDECAR_URL` skips
  (with the documented "ML sidecar unreachable" reason) instead of
  silently returning zero scenarios and failing with the misleading
  "open_knowledge scenario not loaded" message.
- The smackerel.sh wrapper path remains green (the wrapper's
  `/workspace/${path}` rewrite continues to work, and is now
  redundant rather than load-bearing).

**Hard Constraints:**

1. **No product-code change.** The fix MUST stay inside the test
   file plus this bug packet. `internal/agent/loader.go` and the
   open-knowledge runtime are untouched.
2. **No new env vars or config keys.** Resolution happens entirely
   inside the test using `filepath.Abs` against the value already
   in `AGENT_SCENARIO_DIR`.
3. **Fail-loud on bad input.** If `filepath.Abs` fails, or the
   resolved path does not exist, or the resolved path is not a
   directory, the test MUST `t.Fatalf` with both the raw value and
   the resolved absolute value so an operator can correct the env
   immediately.
4. **Adversarial regression.** A targeted Go unit test MUST exist
   that, with cwd set to a temp directory and a relative
   `AGENT_SCENARIO_DIR` pointing at the real
   `config/prompt_contracts/` tree under the repo root, would FAIL
   if the absolute-resolution fix were reverted.

**Failure Condition:** If a future change reverts the
absolute-resolution logic, the regression test MUST fail with a
deterministic, non-tautological assertion that names the missing
scenarios path.

## 3. Non-Goals

- Changing the loader's "missing directory ≡ empty set" contract
  (spec 037 BS-001). That contract is intentional for fresh
  deploys and ephemeral sidecar containers.
- Rewriting `config/generated/{dev,test}.env` to ship an absolute
  path. The relative value is intentional so the same env file
  works inside and outside the container.
- Removing the `smackerel.sh:913-918` rewrite. It remains a
  defense-in-depth measure for the container path and does not
  conflict with the test-side resolution.
