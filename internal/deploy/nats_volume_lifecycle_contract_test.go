// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Spec 082 SCOPE-082-04 — NATS volume durability contract.
//
// NATS JetStream persists at-least-once IN-FLIGHT capture state (events
// accepted by the capture path but not yet processed by smackerel-core) to
// the `nats-data` named volume. Before SCOPE-082-04 the nats service was
// labelled `com.smackerel.lifecycle: ephemeral`, which marked its volume as
// fair game for `./smackerel.sh clean` flows — a `clean` on a running
// self-hosted stack could have wiped queued capture events that were durably
// accepted but not yet persisted to Postgres.
//
// This contract pins the nats service's lifecycle label to `persistent` so
// the cleanup tooling never targets nats-data for removal on a running
// stack. The adversarial sub-test proves a regression back to `ephemeral`
// is rejected.
//
// Cross-reference:
//   - specs/082-mvp-target-readiness-hardening/spec.md FR-082-004
//   - specs/082-mvp-target-readiness-hardening/scenario-manifest.json SCN-082-D01
//   - deploy/compose.deploy.yml (services.nats.labels)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeLifecycleDoc captures each service's component + lifecycle labels.
type composeLifecycleDoc struct {
	Services map[string]struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"services"`
}

const (
	lifecycleLabelKey      = "com.smackerel.lifecycle"
	natsRequiredLifecycle  = "persistent"
	natsServiceName        = "nats"
	natsForbiddenLifecycle = "ephemeral"
)

// assertNatsVolumeLifecycle returns nil iff the nats service declares
// `com.smackerel.lifecycle: persistent`. Returns a non-nil error naming the
// observed value otherwise.
func assertNatsVolumeLifecycle(yamlBytes []byte) error {
	var doc composeLifecycleDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	svc, ok := doc.Services[natsServiceName]
	if !ok {
		return fmt.Errorf("contract violation: services.%s not found in compose document — SCOPE-082-04 requires the nats service to exist and carry a durable lifecycle label", natsServiceName)
	}
	got, ok := svc.Labels[lifecycleLabelKey]
	if !ok {
		return fmt.Errorf("contract violation: services.%s has no %q label — SCOPE-082-04 requires it to be %q (JetStream holds at-least-once in-flight capture state that MUST survive `clean`)", natsServiceName, lifecycleLabelKey, natsRequiredLifecycle)
	}
	if got != natsRequiredLifecycle {
		return fmt.Errorf("contract violation: services.%s.labels[%q]=%q, expected %q — SCOPE-082-04: nats-data holds queued capture events not yet persisted to Postgres; labelling it %q would let `./smackerel.sh clean` wipe them on a running self-hosted stack", natsServiceName, lifecycleLabelKey, got, natsRequiredLifecycle, got)
	}
	return nil
}

// TestNatsVolumeLifecycle_LiveFile asserts the live deploy compose labels
// the nats service `persistent`.
func TestNatsVolumeLifecycle_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertNatsVolumeLifecycle(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates SCOPE-082-04 nats durability contract: %v", err)
	}
	t.Logf("contract OK: services.nats is labelled %s=%s (SCOPE-082-04 — JetStream in-flight capture state is protected from clean)", lifecycleLabelKey, natsRequiredLifecycle)
}

// TestNatsVolumeLifecycle_AdversarialEphemeralRegression proves the contract
// catches a regression back to the pre-082 `ephemeral` label that would let
// clean wipe queued captures.
func TestNatsVolumeLifecycle_AdversarialEphemeralRegression(t *testing.T) {
	fixture := fmt.Sprintf(`services:
  nats:
    labels:
      com.smackerel.component: nats
      %s: %s
`, lifecycleLabelKey, natsForbiddenLifecycle)

	err := assertNatsVolumeLifecycle([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: nats labelled `ephemeral` was ACCEPTED (a SCOPE-082-04 regression that risks wiping in-flight captures would NOT be caught)")
	}
	if !strings.Contains(err.Error(), natsServiceName) {
		t.Fatalf("adversarial contract test failed: error did not mention %q: %v", natsServiceName, err)
	}
	if !strings.Contains(err.Error(), natsForbiddenLifecycle) {
		t.Fatalf("adversarial contract test failed: error did not report the observed %q value: %v", natsForbiddenLifecycle, err)
	}
	t.Logf("adversarial OK: nats labelled ephemeral is rejected with: %v", err)
}
