# Design: BUG-042-005 — Compose contract test missing adversarial coverage for literal-bind / default-fallback bind on prometheus

## Approach

Add a single new persistent in-tree adversarial test function `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` to `internal/deploy/compose_contract_test.go`. The function uses a table-driven sweep with two sub-cases that close the prometheus-side coverage gap in the existing adversarial suite:

1. `prometheus` with literal `127.0.0.1:` bind (spec 020 form).
2. `prometheus` with the forbidden default-fallback bind form `${HOST_BIND_ADDRESS:-127.0.0.1}:`.

The fix is purely additive at the test layer. No change to `assertComposeContract`, no change to the `requiredPrometheusPrefix` constant, no change to `deploy/compose.deploy.yml`, no change to any production runtime code. The existing assertion already rejects both forbidden forms; the new test LOCKS that rejection against future drift in the assertion implementation. The pattern is mechanically identical to BUG-042-003 (ollama coverage) and BUG-042-004 (smackerel-core / smackerel-ml coverage); only the target service and anchor-list change.

## Design Decisions

### DD-1: Single test function with table-driven sub-cases, not two separate test functions

**Decision:** Implement the two coverage gaps as two sub-cases of one `TestComposeContract_AdversarialPrometheusLiteralBindAndFallbackForms` function, table-driven via `cases := []struct{ name, port string }{ ... }`. Do NOT split into two top-level test functions.

**Rationale:** The two sub-cases share identical test infrastructure: build a compose YAML fixture string with prometheus's `ports[0]` set to the forbidden form, call `assertComposeContract`, assert non-nil error, assert error mentions `prometheus`, assert error mentions an anchor term. Splitting into two top-level functions would duplicate this scaffold. The table-driven pattern matches the established convention in this file: `TestComposeContract_AdversarialNetworkModeHostBypass` uses 5 sub-cases, `TestComposeContract_AdversarialOllamaLiteralBind` uses 2 sub-cases, `TestComposeContract_AdversarialDefaultFallbackBind` uses 3 sub-cases. Keeping the same pattern minimises cognitive load and makes future extensions trivial.

**Alternatives rejected:**
- Two top-level functions: rejected because it duplicates the scaffold for no isolation gain (each sub-case is independent in `t.Run` already).
- Three sub-cases (split default-fallback into core-impacted + ml-impacted variants): rejected because prometheus is a single service; the regression target is the prefix check on the prometheus block, not on adjacent services.

### DD-2: Anchor-list assertion on the rejection error message, not exact-string match

**Decision:** Each sub-case asserts the rejection error mentions ANY of `[spec 049, spec 042, fail-loud, ${HOST_BIND_ADDRESS:?, ${HOST_BIND_ADDRESS:-127.0.0.1}, literal 127.0.0.1:]` via a `strings.Contains` loop. Do NOT pin to a single exact substring.

**Rationale:** The `assertComposeContract` rejection message for prometheus is a long sentence that contains all six anchor terms — `"... (spec 049 inherits the spec 042 tailnet-edge bind contract; Prometheus host port MUST use the fail-loud ${HOST_BIND_ADDRESS:?...} SST substitution so compose aborts at start time if HOST_BIND_ADDRESS is unset — no literal 127.0.0.1: prefix, no default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form)"`. Any one of the six terms appearing proves the rejection lands on the prometheus contract surface (spec 049 is the prometheus-specific anchor; the rest are inherited from spec 042). Pinning to a single term would couple the test brittlely to the exact wording of the error message — a future cosmetic re-wording would break the test for no real regression. The anchor-list approach keeps the test honest about WHICH contract is being enforced (anchor terms) while staying flexible to error-message wording changes.

**Alternatives rejected:**
- Pin to a single exact substring (e.g., `"spec 049"`): rejected per the rationale above.
- Pin to a regex pattern: rejected because regex syntax is overkill for `Contains`-style anchor matching and adds a maintenance burden.
- Skip error-message assertion entirely: rejected because that would not prove the rejection lands on the right contract — a stub error from any other code path would silently pass.

### DD-3: Two sub-cases, not three; literal-bind case kept even though no live regression is plausible

**Decision:** Two sub-cases: prometheus literal `127.0.0.1:`, prometheus default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:`. Do NOT add a third sub-case for prometheus with `network_mode: host` (already covered by `TestComposeContract_AdversarialNetworkModeHostBypass` which iterates over all four operator-facing services including prometheus).

**Rationale:** The two sub-cases mirror the BUG-042-003 ollama pattern (literal + default-fallback). The literal-bind case is kept even though "literal 127.0.0.1:" looks similar to "no port published" (which is also rejected by `TestComposeContract_AdversarialInfraHasPorts` for postgres / nats) — the prometheus case is distinct because prometheus IS expected to publish a port (it has profile-gated runtime exposure), so the right contract is "publish via fail-loud SST", not "do not publish at all". The third sub-case (`network_mode: host`) is intentionally OUT of scope here because it is already covered by `TestComposeContract_AdversarialNetworkModeHostBypass`.

**Alternatives rejected:**
- Three sub-cases (add `network_mode: host`): rejected because it duplicates `TestComposeContract_AdversarialNetworkModeHostBypass`.
- One sub-case (only default-fallback, since literal-bind is "obviously wrong"): rejected because the literal form is a real regression risk if someone copies the spec 020 pattern from a stale doc.

### DD-4: RED proof via temporary relaxation of the prefix check, not via test removal

**Decision:** Capture the RED→GREEN proof by temporarily replacing `strings.HasPrefix(p, requiredPrometheusPrefix)` with `strings.Contains(p, "${HOST_BIND_ADDRESS:")` in the prometheus branch of `assertComposeContract` — a too-loose substring check that would accept the default-fallback `${HOST_BIND_ADDRESS:-127.0.0.1}:` form (because it contains the substring `${HOST_BIND_ADDRESS:`) but still reject the literal `127.0.0.1:` form (because the literal does not contain that substring). Keep all the new test code intact. After capturing the FAIL output, restore the strict `HasPrefix` check via `replace_string_in_file`.

**Rationale:** This proves the test would catch a realistic regression (a maintainer accidentally relaxing the strictness of the prefix check). Removing the test entirely would only prove "tests don't exist when removed" — a weaker proof. The chosen relaxation (`Contains` over `HasPrefix` with the substring `${HOST_BIND_ADDRESS:`) is the most plausible accidental relaxation: it matches the fail-loud `:?` form, the forbidden `:-` form, AND any other future variant — exactly the over-permissive failure mode the new test exists to catch. The asymmetric outcome (default-fallback FAILs, literal-bind still PASSes) is a positive cross-check that the relaxation is correctly attributed to the prefix-vs-substring switch and not to a fixture bug.

**Alternatives rejected:**
- `git stash push -p`: rejected because interactive patch staging cannot be scripted reliably and the test surface is too small for stash-based isolation.
- Temporary deletion of the new test code: rejected because it proves "the test was removed" rather than "the assertion was relaxed."

### DD-5: HL-RESCAN-010 attribution in the docstring, not the failure-case error message

**Decision:** Mention `HL-RESCAN-010` in the test function docstring and in the failure-case `t.Fatalf` message. Do NOT mention it in the rejection-message anchor-list assertion (that anchor list is about the contract surface, not the bug ID).

**Rationale:** The failure-case error message ("the contract is tautological — it would NOT catch a regression to the spec 020 literal form or to the default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form for prometheus; HL-RESCAN-010 prometheus literal-bind / default-fallback coverage gap is reintroduced") is the message printed when the test FAILS RED — it points the future maintainer at the bug ID this guard locks. The rejection-message anchor-list assertion is the contract-surface check (proving `assertComposeContract` rejected the right thing); the bug ID belongs in the meta-evidence (docstring + fail message), not in the contract surface itself.

**Alternatives rejected:**
- Pin `HL-RESCAN-010` in the rejection-message anchor list: rejected because that would require modifying `assertComposeContract` to embed the bug ID, which is out of scope (no production-code change wanted).
- Skip HL-RESCAN-010 attribution entirely: rejected because future maintainers benefit from the breadcrumb back to the discovering finding.

### DD-6: No edit to deploy/compose.deploy.yml, no edit to assertComposeContract, no edit to the prefix constants

**Decision:** The fix is bounded to a single test function addition in `internal/deploy/compose_contract_test.go`. No other file is modified. The existing assertion code, the existing `requiredPrometheusPrefix` constant, and the live compose file are all unchanged.

**Rationale:** The defect is a test-coverage gap, not a runtime behavior bug or a contract-text bug. The runtime behavior is correct (the assertion rejects the forbidden forms today). The contract text is correct (the constant is character-for-character matched against the live compose file). The live compose file is correct (prometheus uses the fail-loud form). Editing any of these surfaces would be out of scope for HL-RESCAN-010.

**Alternatives rejected:**
- Add the test plus a comment to `assertComposeContract` referencing HL-RESCAN-010: rejected because it would expand the change boundary unnecessarily.
- Add the test plus an explanatory header to the const block: rejected for the same reason.

## Trade-offs

- The test is a static-file lint that runs on every Go test invocation. A regression would be caught at pre-merge / pre-push, not at deploy time. This is consistent with how the rest of the spec 042 / spec 049 contract is locked — the Go test suite IS the contract enforcement layer.
- The anchor-list assertion (DD-2) trades exact-string assertion strictness for tolerance to future cosmetic error-message edits. A maintainer who completely re-writes the rejection message in `assertComposeContract` to remove all six anchor terms would break this test — but that re-write would be a substantial change that warrants its own review and would naturally surface this dependency.
- The fix lands on the test surface only. A truly malicious maintainer who simultaneously deletes both the test AND the assertion enforcement would obviously bypass the contract — but that pattern is outside the threat model (CR review + the nine other adversarial tests in the suite would surface it).
