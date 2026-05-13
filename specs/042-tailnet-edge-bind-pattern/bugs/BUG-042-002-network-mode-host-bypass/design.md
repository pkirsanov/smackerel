# Design: BUG-042-002 — Compose contract validator silently accepts `network_mode: host`

## Current Truth (codebase reality before fix)

- `assertComposeContract` lives at [internal/deploy/compose_contract_test.go](../../../../internal/deploy/compose_contract_test.go) lines ~85-150 (in-test validator, not exported because the contract surface is the static compose file, not a runtime API).
- The validator parses `deploy/compose.deploy.yml` into a minimal `composeDoc` struct: `Services map[string]struct { Ports []string }`.
- It checks four invariants — all on the `ports:` block:
  1. Every entry of `services.smackerel-core.Ports` starts with `${HOST_BIND_ADDRESS:-127.0.0.1}:${CORE_HOST_PORT}:` (BUG-042-001 closed multi-ports bypass).
  2. Every entry of `services.smackerel-ml.Ports` starts with `${HOST_BIND_ADDRESS:-127.0.0.1}:${ML_HOST_PORT}:` (BUG-042-001 closed multi-ports bypass).
  3. `services.postgres.Ports` is empty (if postgres exists).
  4. `services.nats.Ports` is empty (if nats exists).
- The validator NEVER reads `network_mode`. The struct does not even declare the field, so `yaml.Unmarshal` discards it silently.
- The live `deploy/compose.deploy.yml` does not declare `network_mode: host` for any service, so no live exposure exists today. The defect is in the GUARD, not in the live state.
- The test-round 11 probe at `/tmp/probe042/main.go` empirically confirmed the bypass: a fixture with `core.network_mode: host` and a compliant `core.ports` block is silently accepted by the unmodified validator.

## Why this is a real defect, not a hypothetical

`network_mode: host` is the canonical Docker escape-hatch for services that need direct host networking — a future operator (or an LLM agent assisting an operator) faced with a host-loopback connection issue could plausibly reach for it without understanding the spec 042 contract. The chaos-style probe found no instance of it in the live file, but the contract test exists precisely to catch FUTURE regressions of the spec, not to audit the current state. The same logic that justified BUG-042-001 (the multi-ports bypass — also not present in the live file) applies here.

The bypass is also strictly broader than the BUG-042-001 multi-ports case: a malicious or careless `network_mode: host` exposes EVERY container port on EVERY host NIC, including infra services like postgres (5432) and nats (4222) that the spec 042 design specifically wanted off the host network entirely.

## Design Decision

Add `NetworkMode string yaml:"network_mode"` to the `composeDoc` service struct. Inside `assertComposeContract`, after the service is looked up but before the `ports:` checks, reject `network_mode: host` for each of the four services in the contract set. Each service gets its own check so the error message names the violating service.

### Why per-service checks and not a single generic loop

Three options were considered:

| Option | Description | Rejected because |
|---|---|---|
| A | Single loop: `for name, svc := range doc.Services { if svc.NetworkMode == "host" { return error } }`. | Too strict — the validator deliberately ignores out-of-scope services like `ollama` (profile-gated). A generic loop would reject `ollama.network_mode: host` even though spec 042 explicitly excludes ollama. |
| B | Per-service check, mirroring the existing per-service `ports:` checks. | **Chosen.** Matches the existing validator structure (each service has its own block of invariants), allows the contract set to evolve independently per service, gives precise error messages. |
| C | Reject any non-empty `network_mode` (not just `"host"`) on contract-set services. | Too strict — `network_mode: bridge` (the default) is benign, `network_mode: none` is the safest possible mode, `network_mode: service:<name>` is a legitimate sidecar pattern. Only `host` is the bypass. |

Option B is the smallest, most general fix that closes the bypass without over-constraining future legitimate edits.

### Why one test function with four table-driven sub-cases (not four separate test functions)

The four checks share identical assertion structure — only the service name and fixture differ. A table-driven test compresses the per-service variant from ~20 lines × 4 = 80 lines to a single ~30-line function. This matches the rule-of-three threshold (4 cases, all structurally identical, all asserting the same three properties). The `TestComposeContract_AdversarialMultiPortsBypass` / `MLMultiPortsBypass` BUG-042-001 pair could also have been table-driven; their split into two separate functions was an early-bug stylistic choice, not a contract requirement.

### Error message contract

The new error messages name:
1. The service (`smackerel-core`, `smackerel-ml`, `postgres`, or `nats`).
2. The violating field (`network_mode`).
3. The violating value (`"host"`).
4. The forbidden-pattern explanation: "`network_mode: host` is forbidden by spec 042; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping" (or the Pattern P1 variant for infra).
5. The BUG-042-002 attribution — gives a future agent the regression context.

The new adversarial test asserts on (1), (2), and (5) — three independent properties that together prove the test is non-tautological:
- (1) proves the right service was identified.
- (2) proves the validator inspected the `network_mode` field, not just the `ports:` block.
- (5) proves the regression contract is wired to this specific bug attribution.

## Code change diff

```diff
--- a/internal/deploy/compose_contract_test.go
+++ b/internal/deploy/compose_contract_test.go
@@ struct: add NetworkMode field
 type composeDoc struct {
 	Services map[string]struct {
 		Ports []string `yaml:"ports"`
+		NetworkMode string `yaml:"network_mode"`
 	} `yaml:"services"`
 }

@@ assertComposeContract: smackerel-core block
 	core, ok := doc.Services["smackerel-core"]
 	if !ok {
 		return fmt.Errorf("contract violation: services.smackerel-core not found in compose document")
 	}
+	if core.NetworkMode == "host" {
+		return fmt.Errorf("contract violation: services.smackerel-core.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)", core.NetworkMode)
+	}
 	if len(core.Ports) == 0 { ... }

@@ assertComposeContract: smackerel-ml block
 	ml, ok := doc.Services["smackerel-ml"]
 	if !ok { ... }
+	if ml.NetworkMode == "host" {
+		return fmt.Errorf("contract violation: services.smackerel-ml.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; ...)", ml.NetworkMode)
+	}
 	if len(ml.Ports) == 0 { ... }

@@ assertComposeContract: postgres block
 	if pg, ok := doc.Services["postgres"]; ok {
+		if pg.NetworkMode == "host" {
+			return fmt.Errorf("... services.postgres.network_mode=%q ... defeats Pattern P1: tailscale ssh + docker exec)", pg.NetworkMode)
+		}
 		if len(pg.Ports) > 0 { ... }
 	}

@@ assertComposeContract: nats block
 	if n, ok := doc.Services["nats"]; ok {
+		if n.NetworkMode == "host" {
+			return fmt.Errorf("... services.nats.network_mode=%q ... defeats Pattern P1: tailscale ssh + docker exec)", n.NetworkMode)
+		}
 		if len(n.Ports) > 0 { ... }
 	}

@@ new adversarial test (table-driven, 4 sub-cases)
+func TestComposeContract_AdversarialNetworkModeHostBypass(t *testing.T) {
+	cases := []struct{ name, service, fixture string }{
+		{ name: "smackerel-core uses network_mode host", service: "smackerel-core", fixture: ... },
+		{ name: "smackerel-ml uses network_mode host",   service: "smackerel-ml",   fixture: ... },
+		{ name: "postgres uses network_mode host",       service: "postgres",       fixture: ... },
+		{ name: "nats uses network_mode host",           service: "nats",           fixture: ... },
+	}
+	for _, tc := range cases {
+		tc := tc
+		t.Run(tc.name, func(t *testing.T) {
+			err := assertComposeContract([]byte(tc.fixture))
+			if err == nil { t.Fatalf(...BUG-042-002 network_mode bypass is reintroduced...) }
+			if !strings.Contains(err.Error(), tc.service)    { t.Fatalf(...) }
+			if !strings.Contains(err.Error(), "network_mode") { t.Fatalf(...) }
+			if !strings.Contains(err.Error(), "BUG-042-002") { t.Fatalf(...) }
+			t.Logf("adversarial OK: ...")
+		})
+	}
+}
```

## Open Questions Resolved During Implementation

- **OQ-A**: Should `network_mode: host` be rejected for all services or only contract-set services? → Resolved: only contract-set services (the four named in spec 042). Generic enforcement would conflict with the explicit out-of-scope decision for `ollama` (profile-gated).
- **OQ-B**: Should we also reject other `network_mode` values like `container:<name>`? → Resolved: no. Only `"host"` is the categorical bypass. Other modes are either default-safe (`bridge`, `none`) or are legitimate orchestration patterns (`service:<name>`, `container:<name>`) that don't share the host network namespace.
- **OQ-C**: Should we add `network_mode: host` text to `.github/copilot-instructions.md` Tailnet-Edge Bind Pattern section? → Resolved: not in this bug. The mechanical guard in the contract test is sufficient — the copilot-instructions section already states the spec 042 contract abstractly ("backends bind via HOST_BIND_ADDRESS, infra has no host port"); `network_mode: host` is one mechanism that violates both invariants and is now mechanically blocked. A future docs-only spec can amend the copilot-instructions text if a future agent demonstrates confusion. Recording as future-tracked but not in scope here.
