# Design: BUG-042-003 — Ollama service exempt from spec 042 compose contract enforcement

## Approach

Extend the existing `assertComposeContract` function in `internal/deploy/compose_contract_test.go` with an ollama-branch enforcement block, modeled exactly on the prometheus branch (the closest sibling: also profile-gated, also operator-facing, also subject to the spec 042 fail-loud SST contract). Add two adversarial test functions/sub-cases that prove the new enforcement would catch a regression. Leave the live `deploy/compose.deploy.yml` unchanged — it already complies; the fix only adds the static lock against future drift.

## Design Decisions

### DD-1: Mirror the prometheus branch shape, not the smackerel-core branch shape

**Decision:** Implement the ollama enforcement using the optional-service pattern (`if oll, ok := doc.Services["ollama"]; ok { ... }`), same as prometheus, postgres, and nats. Do NOT use the required-service pattern that smackerel-core and smackerel-ml use.

**Rationale:** ollama is profile-gated (`profiles: [ollama]`) — it appears in `deploy/compose.deploy.yml` but only runs when the operator activates the `ollama` profile. The contract test uses small fixtures that often omit ollama entirely (e.g. the existing `TestComposeContract_AdversarialLiteralBind` fixture has only `smackerel-core`, `smackerel-ml`, `postgres`, `nats`). A required-service pattern would force every existing test fixture to add an `ollama: {}` line just to keep passing — that would bloat the test surface for no contract-coverage gain. The optional pattern matches prometheus exactly and keeps existing fixtures intact.

**Alternatives rejected:**
- Required-service pattern (forced-presence): rejected because it explodes the fixture surface and over-couples the contract to an optional service.
- Profile-aware pattern (only enforce when `profiles: [ollama]` declared): rejected because it adds parsing complexity and the existing prometheus branch (also profile-gated) does NOT do this — keeping the patterns identical reduces cognitive load.

### DD-2: Two attribution strings (BUG-042-002 OR BUG-042-003) accepted in the network_mode sub-test

**Decision:** The `TestComposeContract_AdversarialNetworkModeHostBypass` table-driven sweep checks for `BUG-042-002` OR `BUG-042-003` in the error message, not just one. The four pre-existing sub-cases (smackerel-core, smackerel-ml, postgres, nats) each emit `BUG-042-002` attribution; the new `ollama` sub-case emits `BUG-042-003` attribution because the ollama branch was added later.

**Rationale:** The original BUG-042-002 fix added the network_mode check uniformly across smackerel-core, smackerel-ml, postgres, nats. The ollama branch is a BUG-042-003 addition — it inherits the same defect class but the attribution string in the error message correctly names the closing bug. The adversarial assertion accepts either attribution to keep the test honest about which bug the regression would resurrect.

**Alternatives rejected:**
- Force every error message to mention both BUG-042-002 AND BUG-042-003: rejected because it conflates two separate close-outs in the same string.
- Force the ollama error to mention only BUG-042-002: rejected because that misattributes the closing bug.

### DD-3: requiredOllamaPrefix is a package-level constant

**Decision:** Add `requiredOllamaPrefix` to the `const ( ... )` block alongside `requiredCorePrefix`, `requiredMLPrefix`, `requiredPrometheusPrefix`. Do NOT inline the literal in the assertion function.

**Rationale:** Constancy parity with the other three required prefixes. The constant is the single source of truth for the literal string the live compose file MUST match. Future maintainers extending the contract for new services follow the same pattern (e.g. if grafana ever gets enforced, add `requiredGrafanaPrefix`). Inlining the literal would create string-duplication risk where a future fix could update one site but miss the other.

**Alternatives rejected:**
- Inline string literal in the `if oll.NetworkMode == "host"` / prefix-check block: rejected per the rationale above.
- Compute the prefix dynamically from a base + variable name: rejected because the literal substring `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:` is the contract — it MUST match character-for-character including the error message text inside `:?...`. Computing it dynamically would invite drift.

### DD-4: RED proof via temporary block removal, not git stash

**Decision:** Capture the RED→GREEN proof by temporarily replacing the ollama enforcement block in `assertComposeContract` with a no-op (`_ = requiredOllamaPrefix`) — keeping all the new test code intact — then re-running the contract suite. After capturing the FAIL output, restore the enforcement block via `replace_string_in_file`.

**Rationale:** This isolates the proof to the new ollama enforcement code only. A `git stash` of the entire test file would also remove the new tests, so the RED state would show "tests don't exist" rather than "tests exist and FAIL". The latter is what the adversarial test contract requires.

**Alternatives rejected:**
- `git stash push -p`: rejected because interactive patch staging cannot be scripted by the tool surface and is error-prone.
- Whole-file stash + git apply diff: rejected because the diff would conflict with the parallel session's edits to other files in the working tree.

### DD-5: No edit to deploy/compose.deploy.yml

**Decision:** The live `deploy/compose.deploy.yml` is unchanged. The fix is purely additive at the contract-test layer.

**Rationale:** The live file already complies with the contract this fix locks (line 243 uses the fail-loud SST form). Editing the live file would either be a no-op (if we re-write the same line) or a real change (if we change the form) — both are out of scope for HL-RESCAN-005, which is a coverage gap on the test side, not a regression on the live side.

**Alternatives rejected:**
- Add explanatory comment to deploy/compose.deploy.yml line 243 referencing BUG-042-003: rejected because the comment block above the line already explains the spec 042 SST policy; adding a BUG attribution to every spec line would clutter the file.

## Trade-offs

- The contract is locked at the test layer, not at the docker-compose validation layer. This means a regression would be caught at pre-merge / pre-push (where the Go unit tests run) but NOT at `docker compose config` time (which would only catch syntax-level issues, not policy issues). This is consistent with how the other contract enforcement (smackerel-core, smackerel-ml, prometheus) is locked — the Go test suite IS the contract enforcement layer.
- The optional-service pattern means a fixture that drops the ollama service entirely (e.g. a test that scopes down to just smackerel-core) silently skips the ollama check. This is intentional and matches the prometheus pattern; the LiveFile test always exercises the live compose file, which DOES include ollama, so the live-file path is always covered.
