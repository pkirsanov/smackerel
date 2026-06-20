# BUG-095-001 Scopes

## Scope 1 — Add truthful spec-068 intent.Compiler provenance reference to the spec-095 downstream router

**Status:** Done

### Gherkin Scenarios

See [spec.md](spec.md) SCN-BUG-095-001-1, SCN-BUG-095-001-2, SCN-BUG-095-001-3.

### Implementation

- Add a TRUTHFUL `intent.Compiler` reference to the `selectRetrievalStrategy`
  doc comment in `internal/assistant/retrieval_strategy_routing.go`, stating the
  router consumes the already-compiled `intent.CompiledIntent` produced upstream
  by the spec-068 `intent.Compiler` and never invokes the compiler itself.
- No runtime / source / control-flow / signature change. No guard / allowlist /
  policy edit.

### Test Plan

| ID | Test | File | Expectation |
|----|------|------|-------------|
| T-BUG-095-001-1 | `TestIntentBypassGuardReportsRouterRouteWithoutCompiledIntent` | `tests/integration/policy/intent_bypass_guard_test.go` | Zero findings under `internal/assistant` (red→green) |
| T-BUG-095-001-2 | Adversarial baseline (pre-existing, step 3 of the same test) | `tests/integration/policy/intent_bypass_guard_test.go` | Fixture WITHOUT compiler ref IS flagged; fixture WITH compiler ref is NOT flagged — proves the guard still catches a real raw-text bypass and does not always-fire |
| T-BUG-095-001-3 | Go unit suite | `./smackerel.sh test unit --go` | All packages green (no regression) |

### Definition of Done

- [x] Truthful `intent.Compiler` provenance reference added to
  `selectRetrievalStrategy` doc comment (red→green)
  - Evidence: report.md "Code Diff Evidence" + "Test Evidence (Red→Green Proof)".
- [x] Guard reports zero findings under `internal/assistant`; CI integration-job
  finding resolved
  - Evidence: report.md "Test Evidence (Red→Green Proof)" → GREEN `ok ... 0.143s`.
- [x] Guard, `AllowedRouteCallers`, `ScanSubdirs`, and all policy files
  UNCHANGED (no allowlist edit, no policy edit, `policy-exception-baseline.json`
  untouched)
  - Evidence: report.md "Audit Evidence — Change Boundary" git diff --name-only.
- [x] No runtime behavior / control flow / signature / routing-logic change —
  comment-only
  - Evidence: report.md "Code Diff Evidence" (single doc-comment hunk).
- [x] Added reference is factually TRUE — no misleading "invokes compiler"
  wording, no dead `var _ intent.Compiler` / fake token
  - Evidence: report.md "Code Diff Evidence" + design.md "Why a comment".
- [x] Adversarial regression guard documented — the pre-existing
  `intent_bypass_guard_test.go` step-3 baseline (fixture WITH vs WITHOUT an
  `intent.Compiler` reference) is the regression guard; no redundant new guard
  test required
  - Evidence: report.md "Adversarial Regression Guard".
- [x] Go unit suite green (no regression); check + lint clean
  - Evidence: report.md "Validation Evidence".
