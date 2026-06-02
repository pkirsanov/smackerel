# Spec: BUG-073-001 — Cross-language renderer canary must be parallel-stable

## Expected Behavior

`TestRenderDescriptorV1_CrossLanguageCanary` in `tests/unit/clients/render_descriptor_canary_test.go`
MUST be deterministic regardless of whether it is invoked:

- Standalone: `go test -count=1 -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/`
- As part of the full unit lane: `./smackerel.sh test unit --go` (which executes
  `go test ./...` with default parallelism across packages).

## Acceptance Criteria

1. **AC-1 (Deterministic standalone):** Standalone invocation passes 100% of attempts.
2. **AC-2 (Deterministic under parallel load):** When the test is run in parallel with
   itself and/or alongside the broader unit lane under heavy CPU contention, it MUST pass
   100% of attempts. Verified by ≥ 8 parallel concurrent invocations completing without
   failure.
3. **AC-3 (No correctness regression):** The seven fixture subtests continue to enforce
   the spec 069 / spec 073 cross-language renderer contract (JS output ≡ Dart output ≡
   per-fixture golden descriptor).
4. **AC-4 (Adversarial guard):** A dedicated regression test fails if the fix is reverted
   (i.e., if the Dart renderer subprocess invocation pattern regresses to the unstable
   per-fixture `dart run` form).
5. **AC-5 (No silent masking):** The fix MUST NOT mask real content failures. JSON-shape
   mismatches, schema violations, or stderr-on-success conditions MUST still fail the test
   loudly (no blanket retry / no swallowed errors).

## Out of Scope

- Changing the canary's correctness contract (TP-073-03 wire format).
- Reworking the Dart CLI's behavior or output format.
- Modifying the spec 073 fixture set.

## Cross-References

- Spec: `specs/073-web-mobile-assistant-frontend/spec.md`
- Test plan: `specs/073-web-mobile-assistant-frontend/test-plan.json` → TP-073-03
- Test file: `tests/unit/clients/render_descriptor_canary_test.go`
