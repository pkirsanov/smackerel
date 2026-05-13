# Design: BUG-042-001 — Compose contract validator multi-ports bypass

## Current Truth (codebase reality before fix)

- `assertComposeContract` lives at [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) lines ~73-117 (in-test validator, not exported because the contract surface is the static compose file, not a runtime API).
- The validator parses `deploy/compose.deploy.yml` into a minimal `composeDoc` struct: `Services map[string]struct { Ports []string }`.
- It checks four invariants:
  1. `services.smackerel-core.Ports[0]` starts with `${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:`.
  2. `services.smackerel-ml.Ports[0]` starts with `${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:`.
  3. `services.postgres.Ports` is empty (if postgres exists).
  4. `services.nats.Ports` is empty (if nats exists).
- Invariants 3 and 4 use `len(...) > 0` and therefore catch any port mapping for postgres/nats. Invariants 1 and 2 use `[0]` indexing and therefore only catch single-port regressions or regressions on the first entry.
- The live `deploy/compose.deploy.yml` declares exactly one port mapping for each backend service (`${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}` and `${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}`), so no live exposure exists today. The defect is in the GUARD, not in the live state.
- The chaos round 9 probe at `/tmp/smackerel-chaos-round9/main.go` empirically confirmed the bypass: a fixture with `core.ports = ["${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}", "0.0.0.0:8443:8080"]` is silently accepted by the unmodified validator.

## Design Decision

Use the same `len(...) > 0` + iterate-and-check pattern that the postgres/nats invariants already use, but inverted: instead of "any entry is a violation" the backend services need "every entry must satisfy the prefix". Concretely: replace `if !strings.HasPrefix(core.Ports[0], requiredCorePrefix)` with `for i, p := range core.Ports { if !strings.HasPrefix(p, requiredCorePrefix) { return ... ports[%d]=%q ... } }`. Same change for `ml.Ports`.

### Why iterate rather than enforce `len == 1`

Three options were considered:

| Option | Description | Rejected because |
|---|---|---|
| A | Require `len(core.Ports) == 1` AND prefix match on `[0]`. Strictest interpretation of current intent. | Too strict — a future spec might legitimately add a second backend port (e.g., debug pprof endpoint, websocket server) and would have to amend the contract test even when using a compliant prefix. |
| B | Iterate every entry and require all to satisfy the prefix. | **Chosen.** Matches the spec 042 INTENT ("every published host port for these services uses the configurable bind address"), allows future single-spec port additions without contract amendment, blocks the actual security-relevant regression class. |
| C | Pattern-match on forbidden substrings like `0.0.0.0:`. | Too narrow — would miss other adversarial bind addresses (e.g., specific public IP, `[::]:`). Option B subsumes this by construction. |

Option B is the smallest, most general fix that closes the bypass without over-constraining future legitimate edits.

### Error message contract

The new error message names:
1. The service (`smackerel-core` or `smackerel-ml`).
2. The violating index (`ports[%d]`) — proves the iterator covers every entry, not only `[0]`.
3. The violating value (`%q`) — gives a future operator the literal offending string.
4. The required prefix (`%q`) — explains what was expected.
5. The forbidden-pattern explanation: "literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042".
6. The BUG-042-001 attribution — gives a future agent the regression context.

The new adversarial tests assert on (1), (2), and (6) — three independent properties that together prove the test is non-tautological:
- (1) proves the right service was identified.
- (2) proves the validator iterated past `[0]` to reach the violating entry.
- (6) proves the regression contract is wired to this specific bug attribution.

## Code change diff

```diff
--- a/internal/deploy/compose_contract_test.go
+++ b/internal/deploy/compose_contract_test.go
@@ -82,8 +82,18 @@ func assertComposeContract(yamlBytes []byte) error {
 	if len(core.Ports) == 0 {
 		return fmt.Errorf("contract violation: services.smackerel-core.ports is empty (expected one entry with prefix %q)", requiredCorePrefix)
 	}
-	if !strings.HasPrefix(core.Ports[0], requiredCorePrefix) {
-		return fmt.Errorf("contract violation: services.smackerel-core.ports[0]=%q does not start with required prefix %q (literal 127.0.0.1: prefix is the spec 020 form and is forbidden by spec 042)", core.Ports[0], requiredCorePrefix)
+	// BUG-042-001 (chaos round 9, 2026-05-12): Iterate over EVERY entry in
+	// core.Ports rather than only Ports[0]. The original validator only
+	// inspected the first port mapping, allowing an adversarial second
+	// entry like "0.0.0.0:8443:8080" to bypass the HOST_BIND_ADDRESS guard
+	// even though the first entry was contract-compliant. The contract is
+	// "every published host port for this service uses the configurable
+	// bind address", so every entry must satisfy the prefix.
+	for i, p := range core.Ports {
+		if !strings.HasPrefix(p, requiredCorePrefix) {
+			return fmt.Errorf("contract violation: services.smackerel-core.ports[%d]=%q does not start with required prefix %q (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredCorePrefix)
+		}
 	}
 
 	ml, ok := doc.Services["smackerel-ml"]
@@ -93,8 +103,15 @@ func assertComposeContract(yamlBytes []byte) error {
 	if len(ml.Ports) == 0 {
 		return fmt.Errorf("contract violation: services.smackerel-ml.ports is empty (expected one entry with prefix %q)", requiredMLPrefix)
 	}
-	if !strings.HasPrefix(ml.Ports[0], requiredMLPrefix) {
-		return fmt.Errorf("contract violation: services.smackerel-ml.ports[0]=%q does not start with required prefix %q (literal 127.0.0.1: prefix is the spec 020 form and is forbidden by spec 042)", ml.Ports[0], requiredMLPrefix)
+	// BUG-042-001 (chaos round 9, 2026-05-12): Same multi-ports bypass fix
+	// for smackerel-ml. Iterate every entry and require each to use the
+	// HOST_BIND_ADDRESS-substituted prefix.
+	for i, p := range ml.Ports {
+		if !strings.HasPrefix(p, requiredMLPrefix) {
+			return fmt.Errorf("contract violation: services.smackerel-ml.ports[%d]=%q does not start with required prefix %q (literal 127.0.0.1: prefix or any non-${HOST_BIND_ADDRESS:-127.0.0.1} bind is the spec 020 form and is forbidden by spec 042; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredMLPrefix)
+		}
 	}
 
 	if pg, ok := doc.Services["postgres"]; ok {
```

Plus 2 new adversarial sub-tests appended to the bottom of the same file (TestComposeContract_AdversarialMultiPortsBypass and TestComposeContract_AdversarialMLMultiPortsBypass).

## Scope

- IN SCOPE: `internal/deploy/compose_contract_test.go` — validator function + 2 new adversarial sub-tests.
- OUT OF SCOPE: `deploy/compose.deploy.yml` — already compliant, no changes needed.
- OUT OF SCOPE: `.github/copilot-instructions.md` — the existing "Tailnet-Edge Bind Pattern" section already says "Forbidden — `literal 127.0.0.1: in deploy/compose.deploy.yml is forbidden`"; the multi-ports bypass scenario is implicit in that contract and the new test enforces it mechanically.
- OUT OF SCOPE: `docs/Operations.md` — operational guidance unchanged.
- OUT OF SCOPE: spec 042 `spec.md` / `design.md` / `scopes.md` — the parent spec's intent already covers this case ("the smackerel-core and smackerel-ml ports entries use the prefix `${HOST_BIND_ADDRESS:-127.0.0.1}:`"). The fix tightens the test's enforcement to match the documented intent rather than weakening it.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Live `deploy/compose.deploy.yml` regresses on the change | Very low | High | `TestComposeContract_LiveFile` re-runs against the unmodified live file; if it fails, the validator change is incompatible and the diff would be rejected at the green-proof step. |
| Future spec adds a legitimate second backend port and the test blocks it | Low | Medium | The contract is satisfiable with multiple ports as long as each uses the `${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:` (or ml) prefix. Future specs can add `${HOST_BIND_ADDRESS:-127.0.0.1}:${SECOND_PORT}:8443` without amending this test. |
| Cross-package regression in `internal/config` or `internal/api` | Very low | High | The fix is in a `_test.go` file in `internal/deploy`; nothing imports it. Cross-package smoke run confirms no regression. |
| Validator behavior diverges from documentation | Low | Low | Code comments + this design doc + the new tests' explanatory log lines all reference BUG-042-001, providing future agents the trace context. |
