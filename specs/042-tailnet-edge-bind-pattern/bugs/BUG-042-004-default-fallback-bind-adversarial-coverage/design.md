# Design: BUG-042-004 — Compose contract test missing adversarial coverage for default-fallback bind on smackerel-core/ml

## Approach

Add a single new persistent in-tree adversarial test function `TestComposeContract_AdversarialDefaultFallbackBind` to `internal/deploy/compose_contract_test.go`. The function uses a table-driven sweep with three sub-cases that fix three distinct coverage gaps in the existing adversarial suite:

1. `smackerel-core` with the forbidden default-fallback bind form `${HOST_BIND_ADDRESS:-127.0.0.1}:`.
2. `smackerel-ml` with the same forbidden default-fallback bind form.
3. `smackerel-ml` with the literal `127.0.0.1:` spec 020 bind form (already covered for `smackerel-core` by `TestComposeContract_AdversarialLiteralBind` but not for `smackerel-ml`).

The fix is purely additive at the test layer. No change to `assertComposeContract`, no change to the `requiredCorePrefix` / `requiredMLPrefix` constants, no change to `deploy/compose.deploy.yml`, no change to any production runtime code. The existing assertion already rejects all three forms; the new test LOCKS that rejection against future drift in the assertion implementation.

## Design Decisions

### DD-1: Single test function with table-driven sub-cases, not three separate test functions

**Decision:** Implement the three coverage gaps as three sub-cases of one `TestComposeContract_AdversarialDefaultFallbackBind` function, table-driven via `cases := []struct{ name, service, fixture string }{ ... }`. Do NOT split into three top-level test functions.

**Rationale:** The three sub-cases share identical test infrastructure: build a compose YAML fixture string, call `assertComposeContract`, assert non-nil error, assert error mentions the violating service, assert error mentions a regression-target anchor term. Splitting into three top-level functions would triplicate this scaffold. The table-driven pattern matches the established convention in this file: `TestComposeContract_AdversarialNetworkModeHostBypass` uses 5 sub-cases, `TestComposeContract_AdversarialOllamaLiteralBind` uses 2 sub-cases. Following the same pattern keeps cognitive load low and makes future extensions (e.g., adding a `prometheus` sub-case for HL-RESCAN-010) trivial.

**Alternatives rejected:**
- Three top-level functions: rejected because it triplicates the scaffold for no isolation gain (each sub-case is independent in `t.Run` already).
- Two top-level functions (one for default-fallback, one for ml-side literal): rejected because the literal-bind ml-side coverage is mechanically identical to the default-fallback coverage — both prove the prefix check rejects non-conforming forms on the ml branch.

### DD-2: Anchor-list assertion on the rejection error message, not exact-string match

**Decision:** Each sub-case asserts the rejection error mentions ANY of `[spec 020, ${HOST_BIND_ADDRESS:-127.0.0.1}, Gate-G028, fail-loud]` via a `strings.Contains` loop. Do NOT pin to a single exact substring.

**Rationale:** The `assertComposeContract` rejection message is a long sentence containing all four anchor terms — `"a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, ... — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form ..."`. Any one of the four terms appearing proves the rejection lands on the right contract surface. Pinning to a single term would couple the test brittlely to the exact wording of the error message — a future cosmetic re-wording of the error would break the test for no real regression. The anchor-list approach keeps the test honest about WHICH contract is being enforced (anchor terms) while staying flexible to error-message wording changes.

**Alternatives rejected:**
- Pin to a single exact substring (e.g., `"spec 020"`): rejected per the rationale above.
- Pin to a regex pattern: rejected because regex syntax is overkill for `Contains`-style anchor matching and adds a maintenance burden.
- Skip error-message assertion entirely: rejected because that would not prove the rejection lands on the right contract — a stub error from any other code path would silently pass.

### DD-3: Three sub-cases, not four; ml-side default-fallback chosen as the canonical second case

**Decision:** Three sub-cases: smackerel-core default-fallback, smackerel-ml default-fallback, smackerel-ml literal. Do NOT add a fourth sub-case for smackerel-core literal (already covered by the existing `TestComposeContract_AdversarialLiteralBind`).

**Rationale:** The existing `TestComposeContract_AdversarialLiteralBind` already pins the smackerel-core literal-bind rejection. Duplicating it would be tautological (both tests would pass / fail together). The three new sub-cases cover the three genuine gaps: smackerel-core default-fallback (NEW gap), smackerel-ml default-fallback (NEW gap), smackerel-ml literal (NEW gap that exists because the original BUG-042 close-out only covered core-side literal). Three sub-cases is the minimum viable count; four would be over-spec.

**Alternatives rejected:**
- Four sub-cases (add smackerel-core literal): rejected because it duplicates `TestComposeContract_AdversarialLiteralBind`.
- Two sub-cases (drop smackerel-ml literal): rejected because that leaves the ml-side literal-bind regression uncovered, which is one of the three explicit gaps named in the HL-RESCAN-009 finding.

### DD-4: RED proof via temporary relaxation of the prefix check, not via test removal

**Decision:** Capture the RED→GREEN proof by temporarily replacing `strings.HasPrefix(p, requiredCorePrefix)` with `strings.Contains(p, "${HOST_BIND_ADDRESS:")` in the smackerel-core branch of `assertComposeContract` — a too-loose substring check that would accept the default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` form. Keep all the new test code intact. After capturing the FAIL output, restore the strict `HasPrefix` check via `replace_string_in_file`.

**Rationale:** This proves the test would catch a realistic regression (a maintainer accidentally relaxing the strictness of the prefix check). Removing the test entirely would only prove "tests don't exist when removed" — a weaker proof. The chosen relaxation (`Contains` over `HasPrefix` with the substring `${HOST_BIND_ADDRESS:`) is the most plausible accidental relaxation: it matches the fail-loud `:?` form, the forbidden `:-` form, AND any other future variant — exactly the over-permissive failure mode the new test exists to catch.

**Alternatives rejected:**
- `git stash push -p`: rejected because interactive patch staging cannot be scripted reliably and the test surface is too small for stash-based isolation.
- Temporary deletion of the new test code: rejected because it proves "the test was removed" rather than "the assertion was relaxed."

### DD-5: HL-RESCAN-009 attribution in the docstring, not the failure-case error message

**Decision:** Mention `HL-RESCAN-009` in the test function docstring and in the failure-case `t.Fatalf` message. Do NOT mention it in the rejection-message anchor-list assertion (that anchor list is about the contract surface, not the bug ID).

**Rationale:** The failure-case error message ("the contract is tautological — it would NOT catch a regression to ${HOST_BIND_ADDRESS:-127.0.0.1}: default-fallback or literal 127.0.0.1: form; HL-RESCAN-009 default-fallback / literal-bind ml-side coverage gap is reintroduced") is the message printed when the test FAILS RED — it points the future maintainer at the bug ID this guard locks. The rejection-message anchor-list assertion is the contract-surface check (proving `assertComposeContract` rejected the right thing); the bug ID belongs in the meta-evidence (docstring + fail message), not in the contract surface itself.

**Alternatives rejected:**
- Pin `HL-RESCAN-009` in the rejection-message anchor list: rejected because that would require modifying `assertComposeContract` to embed the bug ID, which is out of scope (no production-code change wanted).
- Skip HL-RESCAN-009 attribution entirely: rejected because future maintainers benefit from the breadcrumb back to the discovering finding.

### DD-6: No edit to deploy/compose.deploy.yml, no edit to assertComposeContract, no edit to the prefix constants

**Decision:** The fix is bounded to a single test function addition in `internal/deploy/compose_contract_test.go`. No other file is modified. The existing assertion code, the existing constants, and the live compose file are all unchanged.

**Rationale:** The defect is a test-coverage gap, not a runtime behavior bug or a contract-text bug. The runtime behavior is correct (the assertion rejects the forbidden forms today). The contract text is correct (the constants are character-for-character matches against the live compose file). The live compose file is correct (it uses the fail-loud form for both core and ml). Editing any of these surfaces would be out of scope for HL-RESCAN-009.

**Alternatives rejected:**
- Add the test plus a comment to `assertComposeContract` referencing HL-RESCAN-009: rejected because it would expand the change boundary unnecessarily.
- Add the test plus an explanatory header to the const block: rejected for the same reason.

## Trade-offs

- The test is a static-file lint that runs on every Go test invocation. A regression would be caught at pre-merge / pre-push, not at deploy time. This is consistent with how the rest of the spec 042 contract is locked — the Go test suite IS the contract enforcement layer.
- The anchor-list assertion (DD-2) trades exact-string assertion strictness for tolerance to future cosmetic error-message edits. A maintainer who completely re-writes the rejection message in `assertComposeContract` to remove all four anchor terms would break this test — but that re-write would be a substantial change that warrants its own review and would naturally surface this dependency.
- The fix lands on the test surface only. A truly malicious maintainer who simultaneously deletes both the test AND the assertion enforcement would obviously bypass the contract — but that pattern is outside the threat model (CR review + the eight other adversarial tests in the suite would surface it).
