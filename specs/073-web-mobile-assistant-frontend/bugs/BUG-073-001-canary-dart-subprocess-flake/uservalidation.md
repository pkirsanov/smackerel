# User Validation: BUG-073-001 — Cross-language renderer canary parallel-stability fix

## Checklist

### [Bug Fix] BUG-073-001 Renderer canary parallel-stability
- [x] **What:** Pre-compile Dart CLI to AOT exe in TestMain so the cross-language renderer
  canary no longer races on per-fixture `dart run` startup under parallel unit-lane load.
  - **Steps:**
    1. Run standalone canary: `go test -count=1 -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/`
    2. Run adversarial regression: `go test -count=1 -run TestRenderDescriptorV1_DartPreCompiled ./tests/unit/clients/`
    3. Run 8 concurrent canary invocations and assert all exit 0.
    4. Run full unit lane: `./smackerel.sh test unit --go`
  - **Expected:** All 7 fixture subtests PASS standalone, the adversarial regression PASSES, all 8 concurrent runs exit 0, and the full unit lane passes.
  - **Verify:** terminal exit codes; full output in `report.md` and `scopes.md` DoD evidence blocks.
  - **Evidence:** report.md#test-evidence, scopes.md DoD blocks
  - **Notes:** Bug fix for BUG-073-001
