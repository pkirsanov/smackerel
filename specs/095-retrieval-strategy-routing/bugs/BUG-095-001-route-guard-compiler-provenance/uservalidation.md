# BUG-095-001 User Validation

- [x] The spec-068 route-bypass guard test passes again (zero findings under `internal/assistant`), so the `CI` workflow's `integration` job is green for this finding.
- [x] The fix is a single truthful doc comment — no runtime behavior, control flow, signatures, or routing logic changed; the spec-095 retrieval-strategy router behaves exactly as before.
- [x] The guard, its `AllowedRouteCallers` allowlist, `ScanSubdirs`, the guard test, and `policy-exception-baseline.json` are all unchanged — no cross-spec policy edit and no allowlist blinding.
- [x] The guard still catches a real raw-text bypass: the pre-existing `intent_bypass_guard_test.go` adversarial baseline (fixture WITH vs WITHOUT an `intent.Compiler` reference) is the regression guard, so this fix cannot mask a future genuine bypass.
