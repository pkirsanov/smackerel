// Package deploy contains static-file invariant tests for the deployment
// compose contract enforced by spec 042 (Tailnet-Edge Bind Pattern).
//
// The contract:
//
//  1. The smackerel-core service publishes its host port using the fail-loud
//     prefix "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:".
//     There is NO default fallback — the deploy adapter MUST set HOST_BIND_ADDRESS
//     explicitly in app.env (e.g. 127.0.0.1 for loopback, a tailnet IP for
//     tailnet-edge fronting). Compose fails loud at start time if it is unset
//     or empty. This is Gate G028 (NO-DEFAULTS / fail-loud SST) — the
//     `${VAR:-default}` form is FORBIDDEN.
//  2. The smackerel-ml service publishes its host port using the same fail-loud prefix.
//  3. The postgres service publishes NO host port — DevOps reaches it via
//     `tailscale ssh <host> -- docker exec -it <container> psql ...`
//     (Pattern P1).
//  4. The nats service publishes NO host port — same Pattern P1 access.
//  5. NO service in the contract set (smackerel-core, smackerel-ml, postgres,
//     nats) declares `network_mode: host`. `network_mode: host` is a
//     categorical bypass: it shares the host network namespace and exposes
//     every container port directly on every host NIC, defeating both the
//     HOST_BIND_ADDRESS-substituted port mapping (conditions 1 + 2) and the
//     no-host-port invariant for infra (conditions 3 + 4). BUG-042-002
//     (test round 11, 2026-05-12) closed this bypass after a stochastic
//     sweep probe found that the validator inspected only the `ports:` block.
//
// These invariants live in deploy/compose.deploy.yml. This test parses that
// file with gopkg.in/yaml.v3 and asserts the five conditions hold. Three
// adversarial sub-tests guarantee the contract function would FAIL if
// either invariant regressed (proves the test is not tautological).
//
// References:
//   - specs/042-tailnet-edge-bind-pattern/spec.md
//   - specs/042-tailnet-edge-bind-pattern/design.md
//   - bubbles/skills/bubbles-tailnet-edge-pattern/SKILL.md (canonical pattern)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeDoc is the minimal YAML shape this test needs to assert the
// contract. It intentionally does NOT model every field of compose.deploy.yml
// so that adding unrelated services or fields stays a non-event.
type composeDoc struct {
	Services map[string]struct {
		// Ports is left as []string because compose port entries can be
		// declared as either short-form strings ("HOST:CONT") or long-form
		// objects, and the contract uses short-form throughout. If a future
		// service migrates to long-form ports, this test will fail loudly
		// for that service and the contract assertion can be extended.
		Ports []string `yaml:"ports"`
		// NetworkMode is captured so the contract can mechanically reject
		// `network_mode: host` for any service in the contract set. BUG-042-002
		// closed a categorical bypass: a service that declared
		// `network_mode: host` would share the host network namespace and
		// expose every container port on every host NIC, defeating spec 042's
		// HOST_BIND_ADDRESS-substituted port mapping for backends and the
		// no-host-port invariant for infra. The validator silently ignored
		// `network_mode` before BUG-042-002.
		NetworkMode string `yaml:"network_mode"`
	} `yaml:"services"`
}

// Required port-mapping prefixes — NO DEFAULTS, fail-loud SST per Gate G028.
//
// The `${VAR:-default}` form is FORBIDDEN by copilot-instructions; the
// canonical form is `${VAR:?error}` which makes Docker Compose abort at
// start time with the named error if HOST_BIND_ADDRESS is unset or empty.
// The deploy adapter MUST set HOST_BIND_ADDRESS explicitly in app.env
// (e.g. 127.0.0.1 for loopback, a tailnet IP for tailnet-edge fronting) —
// there is NO implicit fallback to loopback. The literal error message
// inside `:?...` is part of the prefix and MUST match the live compose
// file character-for-character.
const (
	requiredCorePrefix       = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:`
	requiredMLPrefix         = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:`
	requiredPrometheusPrefix = `${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${PROMETHEUS_HOST_PORT}:`
)

// repoRoot returns the repository root by climbing two directories up from
// the directory containing this test file (internal/deploy/ -> repo root).
// Using runtime.Caller makes the path independent of `go test` CWD, which
// makes the test work both from `cd internal/deploy && go test` and from
// `cd /workspace && go test ./...` (the path used by go-unit.sh).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// assertComposeContract returns nil iff the four invariants hold for the
// compose document encoded in yamlBytes. On any violation it returns a
// non-nil error naming the specific service and the specific violation, so
// the adversarial sub-tests can pattern-match the failure mode.
func assertComposeContract(yamlBytes []byte) error {
	var doc composeDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	core, ok := doc.Services["smackerel-core"]
	if !ok {
		return fmt.Errorf("contract violation: services.smackerel-core not found in compose document")
	}
	// BUG-042-002 (test round 11, 2026-05-12): Reject `network_mode: host` for
	// every service in the contract set. `network_mode: host` shares the host
	// network namespace and exposes every container port on every host NIC,
	// which is a categorical bypass of spec 042's HOST_BIND_ADDRESS-substituted
	// port mapping (backends) and no-host-port invariant (infra). Each service
	// is checked independently so the error message names the violating service.
	if core.NetworkMode == "host" {
		return fmt.Errorf("contract violation: services.smackerel-core.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)", core.NetworkMode)
	}
	if len(core.Ports) == 0 {
		return fmt.Errorf("contract violation: services.smackerel-core.ports is empty (expected one entry with prefix %q)", requiredCorePrefix)
	}
	// BUG-042-001 (chaos round 9, 2026-05-12): Iterate over EVERY entry in
	// core.Ports rather than only Ports[0]. The original validator only
	// inspected the first port mapping, allowing an adversarial second
	// entry like "0.0.0.0:8443:8080" to bypass the HOST_BIND_ADDRESS guard
	// even though the first entry was contract-compliant. The contract is
	// "every published host port for this service uses the configurable
	// bind address", so every entry must satisfy the prefix.
	for i, p := range core.Ports {
		if !strings.HasPrefix(p, requiredCorePrefix) {
			return fmt.Errorf("contract violation: services.smackerel-core.ports[%d]=%q does not start with required prefix %q (a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, and any other non-fail-loud bind is forbidden by spec 042 — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form so compose aborts at start time if HOST_BIND_ADDRESS is unset; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredCorePrefix)
		}
	}

	ml, ok := doc.Services["smackerel-ml"]
	if !ok {
		return fmt.Errorf("contract violation: services.smackerel-ml not found in compose document")
	}
	// BUG-042-002 (test round 11, 2026-05-12): Same network_mode: host bypass
	// guard for smackerel-ml. See smackerel-core block above for rationale.
	if ml.NetworkMode == "host" {
		return fmt.Errorf("contract violation: services.smackerel-ml.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes every container port on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)", ml.NetworkMode)
	}
	if len(ml.Ports) == 0 {
		return fmt.Errorf("contract violation: services.smackerel-ml.ports is empty (expected one entry with prefix %q)", requiredMLPrefix)
	}
	// BUG-042-001 (chaos round 9, 2026-05-12): Same multi-ports bypass fix
	// for smackerel-ml. Iterate every entry and require each to use the
	// HOST_BIND_ADDRESS-substituted prefix.
	for i, p := range ml.Ports {
		if !strings.HasPrefix(p, requiredMLPrefix) {
			return fmt.Errorf("contract violation: services.smackerel-ml.ports[%d]=%q does not start with required prefix %q (a literal 127.0.0.1: prefix is the spec 020 form, a default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form is the pre-Gate-G028 form, and any other non-fail-loud bind is forbidden by spec 042 — Gate G028 NO-DEFAULTS requires the fail-loud ${HOST_BIND_ADDRESS:?...} form so compose aborts at start time if HOST_BIND_ADDRESS is unset; BUG-042-001 closes the multi-ports bypass that previously only checked ports[0])", i, p, requiredMLPrefix)
		}
	}

	if pg, ok := doc.Services["postgres"]; ok {
		// BUG-042-002 (test round 11, 2026-05-12): network_mode: host on infra
		// would expose Postgres on every host NIC, defeating Pattern P1.
		if pg.NetworkMode == "host" {
			return fmt.Errorf("contract violation: services.postgres.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes the Postgres container port on every host NIC and defeats Pattern P1: tailscale ssh + docker exec)", pg.NetworkMode)
		}
		if len(pg.Ports) > 0 {
			return fmt.Errorf("contract violation: services.postgres.ports is non-empty (got %v) — postgres must have NO host port mapping per spec 042 (Pattern P1: tailscale ssh + docker exec)", pg.Ports)
		}
	}

	if n, ok := doc.Services["nats"]; ok {
		// BUG-042-002 (test round 11, 2026-05-12): same network_mode: host
		// guard for the nats service. Pattern P1 enforced.
		if n.NetworkMode == "host" {
			return fmt.Errorf("contract violation: services.nats.network_mode=%q — `network_mode: host` is forbidden by spec 042 (BUG-042-002 closes the network_mode bypass; host networking exposes the NATS container port on every host NIC and defeats Pattern P1: tailscale ssh + docker exec)", n.NetworkMode)
		}
		if len(n.Ports) > 0 {
			return fmt.Errorf("contract violation: services.nats.ports is non-empty (got %v) — nats must have NO host port mapping per spec 042 (Pattern P1: tailscale ssh + docker exec)", n.Ports)
		}
	}

	// Spec 049 — Prometheus is profile-gated (off by default) but its
	// service definition still exists in the compose document. When
	// present, its host port MUST inherit the spec 042 fail-loud
	// HOST_BIND_ADDRESS substitution like other operator-facing
	// services (smackerel-core, smackerel-ml). network_mode: host is
	// rejected for the same reason. If the service block is absent
	// (e.g. in a test fixture that scopes down), the check is skipped
	// — mirrors the pattern used for postgres + nats above.
	if prom, ok := doc.Services["prometheus"]; ok {
		if prom.NetworkMode == "host" {
			return fmt.Errorf("contract violation: services.prometheus.network_mode=%q — `network_mode: host` is forbidden by spec 042/049 (host networking exposes Prometheus on every host NIC and defeats the HOST_BIND_ADDRESS-substituted port mapping)", prom.NetworkMode)
		}
		if len(prom.Ports) > 0 {
			for i, p := range prom.Ports {
				if !strings.HasPrefix(p, requiredPrometheusPrefix) {
					return fmt.Errorf("contract violation: services.prometheus.ports[%d]=%q does not start with required prefix %q (spec 049 inherits the spec 042 tailnet-edge bind contract; Prometheus host port MUST use the fail-loud ${HOST_BIND_ADDRESS:?...} SST substitution so compose aborts at start time if HOST_BIND_ADDRESS is unset — no literal 127.0.0.1: prefix, no default-fallback ${HOST_BIND_ADDRESS:-127.0.0.1} form)", i, p, requiredPrometheusPrefix)
				}
			}
		}
	}

	return nil
}

// TestComposeContract_LiveFile is the primary contract assertion. It loads
// the live deploy/compose.deploy.yml from the repo root and proves the
// four invariants hold. This is the test that would FAIL if any future
// edit regresses the contract.
func TestComposeContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertComposeContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 042 tailnet-edge bind contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml satisfies spec 042 (backend ports use fail-loud ${HOST_BIND_ADDRESS:?...}: prefix with NO default fallback per Gate G028; postgres and nats have no host ports)")
}

// TestComposeContract_AdversarialLiteralBind proves the contract function
// catches a regression to the spec 020 hardcoded form. It feeds the
// function a fixture identical in shape to the live file except that the
// smackerel-core port prefix is the literal "127.0.0.1:". The contract
// MUST return a non-nil error mentioning "smackerel-core" and the literal
// prefix being forbidden. This sub-test is the adversarial regression
// guarantee that the live-file contract assertion is not tautological.
func TestComposeContract_AdversarialLiteralBind(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "127.0.0.1:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: literal 127.0.0.1: prefix on smackerel-core was accepted (the contract is tautological — it would NOT catch a regression to the spec 020 form)")
	}
	if !strings.Contains(err.Error(), "smackerel-core") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-core': %v", err)
	}
	if !strings.Contains(err.Error(), "spec 020") {
		t.Fatalf("adversarial contract test failed: error did not mention 'spec 020' (the regression target the test guards against): %v", err)
	}
	t.Logf("adversarial OK: literal 127.0.0.1: prefix on smackerel-core is rejected with: %v", err)
}

// TestComposeContract_AdversarialInfraHasPorts proves the contract function
// catches a regression where postgres re-acquires a host port mapping. It
// feeds the function a fixture where postgres has a ports block. The
// contract MUST return a non-nil error mentioning "postgres" and Pattern
// P1. This sub-test is the adversarial regression guarantee for the infra
// no-host-port invariant.
func TestComposeContract_AdversarialInfraHasPorts(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres:
    ports:
      - "127.0.0.1:5432:5432"
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: postgres ports block was accepted (the contract is tautological — it would NOT catch a regression that re-publishes a host port for postgres)")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Fatalf("adversarial contract test failed: error did not mention 'postgres': %v", err)
	}
	if !strings.Contains(err.Error(), "Pattern P1") {
		t.Fatalf("adversarial contract test failed: error did not mention 'Pattern P1' (the prescribed access path for infra services): %v", err)
	}
	t.Logf("adversarial OK: postgres ports block is rejected with: %v", err)
}

// TestComposeContract_AdversarialMultiPortsBypass proves the contract
// function catches a regression where smackerel-core (or smackerel-ml)
// declares multiple host port mappings and one of them bypasses the
// HOST_BIND_ADDRESS guard. Before BUG-042-001, the validator only
// inspected ports[0]; this test fixture has a contract-compliant entry
// at index 0 followed by an adversarial "0.0.0.0:8443:8080" entry at
// index 1 that would expose the API on every host NIC.
//
// The contract function MUST return a non-nil error mentioning
// "smackerel-core" and the index that violated, proving every entry in
// the ports slice is checked, not just the first.
//
// Discovered: stochastic-quality-sweep round 9 of 20 (seed 20520512),
// trigger=chaos, mapped child mode=chaos-hardening, 2026-05-12.
func TestComposeContract_AdversarialMultiPortsBypass(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
      - "0.0.0.0:8443:8080"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: multi-ports fixture with second entry '0.0.0.0:8443:8080' was accepted (the contract is tautological — it would NOT catch a regression that adds a non-loopback host port mapping after a compliant first entry; BUG-042-001 multi-ports bypass is reintroduced)")
	}
	if !strings.Contains(err.Error(), "smackerel-core") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-core': %v", err)
	}
	if !strings.Contains(err.Error(), "ports[1]") {
		t.Fatalf("adversarial contract test failed: error did not mention the violating index 'ports[1]' (proving every entry is validated, not only ports[0]): %v", err)
	}
	if !strings.Contains(err.Error(), "BUG-042-001") {
		t.Fatalf("adversarial contract test failed: error did not mention BUG-042-001 attribution (the chaos-discovered defect this test guards against): %v", err)
	}
	t.Logf("adversarial OK: multi-ports bypass on smackerel-core is rejected with: %v", err)
}

// TestComposeContract_AdversarialMLMultiPortsBypass mirrors the multi-ports
// bypass guard for smackerel-ml. Same rationale as the core variant — the
// validator must check every entry in the ports slice, not only ports[0].
func TestComposeContract_AdversarialMLMultiPortsBypass(t *testing.T) {
	const fixture = `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
      - "0.0.0.0:9443:8081"
  postgres: {}
  nats: {}
`
	err := assertComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: multi-ports fixture with second entry '0.0.0.0:9443:8081' on smackerel-ml was accepted (BUG-042-001 ml-side bypass is reintroduced)")
	}
	if !strings.Contains(err.Error(), "smackerel-ml") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-ml': %v", err)
	}
	if !strings.Contains(err.Error(), "ports[1]") {
		t.Fatalf("adversarial contract test failed: error did not mention the violating index 'ports[1]': %v", err)
	}
	t.Logf("adversarial OK: multi-ports bypass on smackerel-ml is rejected with: %v", err)
}

// TestComposeContract_AdversarialNetworkModeHostBypass proves the contract
// function catches a regression where any service in the contract set
// declares `network_mode: host`. Host networking shares the host network
// namespace and exposes every container port directly on every host NIC,
// which categorically bypasses spec 042's HOST_BIND_ADDRESS-substituted
// port mapping (backends) and the no-host-port invariant (infra). Before
// BUG-042-002 the validator inspected only `ports:` and silently ignored
// `network_mode`.
//
// The contract function MUST return a non-nil error mentioning the
// offending service and the BUG-042-002 attribution. This test runs four
// table-driven sub-cases — one per service in the contract set — to prove
// the guard is applied uniformly.
//
// Discovered: stochastic-quality-sweep round 11 of 20 (seed 20520512),
// trigger=test, mapped child mode=test-to-doc, 2026-05-12.
func TestComposeContract_AdversarialNetworkModeHostBypass(t *testing.T) {
	cases := []struct {
		name    string
		service string
		fixture string
	}{
		{
			name:    "smackerel-core uses network_mode host",
			service: "smackerel-core",
			fixture: `services:
  smackerel-core:
    network_mode: host
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats: {}
`,
		},
		{
			name:    "smackerel-ml uses network_mode host",
			service: "smackerel-ml",
			fixture: `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    network_mode: host
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats: {}
`,
		},
		{
			name:    "postgres uses network_mode host",
			service: "postgres",
			fixture: `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres:
    network_mode: host
  nats: {}
`,
		},
		{
			name:    "nats uses network_mode host",
			service: "nats",
			fixture: `services:
  smackerel-core:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${CORE_HOST_PORT}:${CORE_CONTAINER_PORT}"
  smackerel-ml:
    ports:
      - "${HOST_BIND_ADDRESS:?HOST_BIND_ADDRESS must be set by deploy adapter}:${ML_HOST_PORT}:${ML_CONTAINER_PORT}"
  postgres: {}
  nats:
    network_mode: host
`,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := assertComposeContract([]byte(tc.fixture))
			if err == nil {
				t.Fatalf("adversarial contract test failed: fixture with %s.network_mode=host was accepted (the contract is tautological — it would NOT catch a regression to host networking; BUG-042-002 network_mode bypass is reintroduced)", tc.service)
			}
			if !strings.Contains(err.Error(), tc.service) {
				t.Fatalf("adversarial contract test failed: error did not mention %q: %v", tc.service, err)
			}
			if !strings.Contains(err.Error(), "network_mode") {
				t.Fatalf("adversarial contract test failed: error did not mention 'network_mode' (the violating field): %v", err)
			}
			if !strings.Contains(err.Error(), "BUG-042-002") {
				t.Fatalf("adversarial contract test failed: error did not mention BUG-042-002 attribution (the test-discovered defect this guard locks): %v", err)
			}
			t.Logf("adversarial OK: network_mode: host on %s is rejected with: %v", tc.service, err)
		})
	}
}
