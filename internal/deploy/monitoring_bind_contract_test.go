// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 049 — Monitoring Stack network-bind contract
// (T-049-003).
//
// FR-049-004 / FR-049-005(c): "Monitoring services MUST follow the
// spec 042 tailnet-edge bind pattern. Host port mappings MUST NOT bind
// to 0.0.0.0 or any wildcard. Only the deploy adapter's
// HOST_BIND_ADDRESS substitution decides exposure."
//
// This test walks every service's `ports:` block in both
// `docker-compose.yml` and `deploy/compose.deploy.yml` and asserts NO
// entry starts with `0.0.0.0:` or `[::]:` (the two IPv4/IPv6 wildcard
// bind forms accepted by Docker port-mapping syntax). The check is
// universal — not limited to monitoring services — because the
// invariant is a property of the compose document as a whole. Spec 042
// + spec 049 share the same threat model: an accidental wildcard bind
// exposes the service on every host NIC, including the public
// internet on cloud hosts.
//
// Adversarial sub-tests prove the contract catches both wildcard
// forms.
//
// Cross-reference:
//   - specs/049-monitoring-stack/spec.md FR-049-004 / FR-049-005(c)
//   - specs/042-tailnet-edge-bind-pattern/ (the bind contract spec 049
//     inherits)
//   - internal/deploy/compose_contract_test.go (sibling spec 042 test)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeBindDoc is the minimum YAML shape we need for the bind walk.
type composeBindDoc struct {
	Services map[string]struct {
		Ports []string `yaml:"ports"`
	} `yaml:"services"`
}

// wildcardBindPrefixes lists every literal substring that, if present
// at the start of a published-port mapping, indicates a wildcard bind.
// The Docker port-mapping syntax accepts a number of forms; these
// cover both IPv4 (`0.0.0.0:`) and IPv6 (`[::]:`) wildcards plus a
// rare unqualified-port form (e.g. `"8080:8080"`) which is treated as
// `0.0.0.0:8080:8080` and is therefore equally forbidden.
//
// The unqualified-port form is detected separately because it does
// not start with one of the prefixes; see assertNoWildcardBinds for
// the structural check.
var wildcardBindPrefixes = []string{
	"0.0.0.0:",
	"[::]:",
}

// assertNoWildcardBinds returns nil iff no service in the parsed
// compose document maps a port using a wildcard bind. The caller is
// responsible for providing a label so the error message names the
// source file.
func assertNoWildcardBinds(doc composeBindDoc, label string) error {
	for svc, def := range doc.Services {
		for i, p := range def.Ports {
			for _, prefix := range wildcardBindPrefixes {
				if strings.HasPrefix(p, prefix) {
					return fmt.Errorf("contract violation: %s services.%s.ports[%d]=%q starts with wildcard bind %q — spec 049/spec 042 forbid wildcard binds because they expose the service on every host NIC; use `${HOST_BIND_ADDRESS:?...}:` substitution (deploy compose) or `127.0.0.1:` (dev compose) so exposure is an explicit operator decision", label, svc, i, p, prefix)
				}
			}
			// Detect the unqualified-port form (`"8080:8080"` or
			// `"8080:8080/tcp"`). A correctly-bound port mapping has
			// at least TWO `:` separators (host_addr:host_port:container_port)
			// OR starts with a known good prefix. If the entry has
			// only ONE `:` separator AND does NOT start with a known
			// good prefix, it is an unqualified form.
			separatorCount := strings.Count(p, ":")
			if separatorCount < 2 {
				return fmt.Errorf("contract violation: %s services.%s.ports[%d]=%q has fewer than two `:` separators — this is the unqualified Docker port form which binds to 0.0.0.0 implicitly; explicit `<bind-addr>:<host-port>:<container-port>` is REQUIRED so the bind address is auditable", label, svc, i, p)
			}
		}
	}
	return nil
}

// loadComposeBindDoc loads a compose file and parses just the bind
// shape. Helper so the live-file and adversarial tests share a path.
func loadComposeBindDoc(yamlBytes []byte) (composeBindDoc, error) {
	var doc composeBindDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return doc, fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	return doc, nil
}

// TestMonitoringBindContract_LiveDevCompose asserts the dev compose
// file has no wildcard binds anywhere.
func TestMonitoringBindContract_LiveDevCompose(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read live dev compose %q: %v", path, err)
	}
	doc, err := loadComposeBindDoc(yamlBytes)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := assertNoWildcardBinds(doc, "docker-compose.yml"); err != nil {
		t.Fatalf("live docker-compose.yml violates spec 049 FR-049-004 bind contract: %v", err)
	}
	t.Logf("contract OK: docker-compose.yml has no wildcard binds (every published port is explicitly bound)")
}

// TestMonitoringBindContract_LiveDeployCompose asserts the deploy
// compose file has no wildcard binds anywhere.
func TestMonitoringBindContract_LiveDeployCompose(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read live deploy compose %q: %v", path, err)
	}
	doc, err := loadComposeBindDoc(yamlBytes)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if err := assertNoWildcardBinds(doc, "deploy/compose.deploy.yml"); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 049 FR-049-004 bind contract: %v", err)
	}
	t.Logf("contract OK: deploy/compose.deploy.yml has no wildcard binds (every published port uses the fail-loud HOST_BIND_ADDRESS substitution)")
}

// TestMonitoringBindContract_AdversarialIPv4Wildcard proves the
// contract catches a `0.0.0.0:` bind regression.
func TestMonitoringBindContract_AdversarialIPv4Wildcard(t *testing.T) {
	const fixture = `
services:
  prometheus:
    image: prom/prometheus:v2.55.1
    ports:
      - "0.0.0.0:9090:9090"
`
	doc, err := loadComposeBindDoc([]byte(fixture))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	err = assertNoWildcardBinds(doc, "test-fixture")
	if err == nil {
		t.Fatal("adversarial contract test failed: 0.0.0.0:9090:9090 bind was accepted (contract is tautological — would NOT catch a regression that exposes Prometheus on every host NIC)")
	}
	if !strings.Contains(err.Error(), "0.0.0.0:") {
		t.Fatalf("adversarial contract test failed: error did not name the 0.0.0.0: prefix: %v", err)
	}
	t.Logf("adversarial OK: 0.0.0.0: wildcard bind is rejected with: %v", err)
}

// TestMonitoringBindContract_AdversarialIPv6Wildcard proves the
// contract catches a `[::]:` bind regression — the IPv6 dual-stack
// wildcard form Docker honours on hosts with IPv6 enabled.
func TestMonitoringBindContract_AdversarialIPv6Wildcard(t *testing.T) {
	const fixture = `
services:
  prometheus:
    image: prom/prometheus:v2.55.1
    ports:
      - "[::]:9090:9090"
`
	doc, err := loadComposeBindDoc([]byte(fixture))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	err = assertNoWildcardBinds(doc, "test-fixture")
	if err == nil {
		t.Fatal("adversarial contract test failed: [::]:9090:9090 bind was accepted (contract is tautological — would NOT catch a regression that exposes Prometheus on every IPv6 NIC)")
	}
	if !strings.Contains(err.Error(), "[::]:") {
		t.Fatalf("adversarial contract test failed: error did not name the [::]: prefix: %v", err)
	}
	t.Logf("adversarial OK: [::]: IPv6 wildcard bind is rejected with: %v", err)
}

// TestMonitoringBindContract_AdversarialUnqualifiedPort proves the
// contract catches the `8080:8080` unqualified form which Docker
// implicitly treats as `0.0.0.0:8080:8080`.
func TestMonitoringBindContract_AdversarialUnqualifiedPort(t *testing.T) {
	const fixture = `
services:
  prometheus:
    image: prom/prometheus:v2.55.1
    ports:
      - "9090:9090"
`
	doc, err := loadComposeBindDoc([]byte(fixture))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	err = assertNoWildcardBinds(doc, "test-fixture")
	if err == nil {
		t.Fatal("adversarial contract test failed: unqualified `9090:9090` bind was accepted (contract is tautological — would NOT catch a regression that uses the implicit-wildcard form)")
	}
	if !strings.Contains(err.Error(), "fewer than two") {
		t.Fatalf("adversarial contract test failed: error did not name the unqualified-form problem: %v", err)
	}
	t.Logf("adversarial OK: unqualified bind form is rejected with: %v", err)
}
