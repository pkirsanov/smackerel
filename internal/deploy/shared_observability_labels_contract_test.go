// Spec 101 — shared-observability instrumentation contract (knb spec 014 scope 03).
//
// The shared self-hosted observability stack (one Prometheus/Grafana/Tempo/Loki/
// otel-collector, owned by the knb adapter shared/observability/self-hosted/)
// discovers and scopes each product's containers by the com.bubbles.product +
// com.bubbles.service labels via Prometheus docker_sd. This contract pins those
// two discovery labels onto every smackerel-owned service in BOTH the dev
// compose (docker-compose.yml) and the deploy compose (deploy/compose.deploy.yml).
//
// The invariant: every service that carries a com.smackerel.component label
// (i.e. a smackerel-owned service block) MUST also carry a non-empty
// com.bubbles.product AND com.bubbles.service label. The product label is
// SST-sourced (${METRICS_SCRAPE_LABEL_PRODUCT}); yaml.Unmarshal sees the literal
// placeholder string here, which is non-empty, so the contract holds pre-
// interpolation. The adversarial sub-test proves a service missing either label
// is rejected (the test is not tautological).
//
// Cross-reference:
//   - specs/101-shared-observability-instrumentation/spec.md (SCN-101-A03)
//   - knb/specs/014-shared-host-observability/scopes/03-smackerel-instrumentation/scope.md (T5)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeBubblesLabelsDoc captures each service's labels map (only the fields
// this contract needs; unrelated service fields stay a non-event).
type composeBubblesLabelsDoc struct {
	Services map[string]struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"services"`
}

const (
	bubblesProductLabelKey = "com.bubbles.product"
	bubblesServiceLabelKey = "com.bubbles.service"
	smackerelComponentKey  = "com.smackerel.component"
)

// assertSharedObservabilityLabels returns nil iff every service that carries a
// com.smackerel.component label ALSO carries non-empty com.bubbles.product and
// com.bubbles.service labels. It fails loud (naming the offenders) otherwise,
// and fails if it finds zero smackerel service blocks to check (compose shape
// drift guard).
func assertSharedObservabilityLabels(yamlBytes []byte) error {
	var doc composeBubblesLabelsDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	names := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		names = append(names, name)
	}
	sort.Strings(names)

	var missing []string
	checked := 0
	for _, name := range names {
		svc := doc.Services[name]
		if _, isSmackerel := svc.Labels[smackerelComponentKey]; !isSmackerel {
			continue // not a smackerel-owned service block
		}
		checked++
		if svc.Labels[bubblesProductLabelKey] == "" {
			missing = append(missing, fmt.Sprintf("services.%s missing %q", name, bubblesProductLabelKey))
		}
		if svc.Labels[bubblesServiceLabelKey] == "" {
			missing = append(missing, fmt.Sprintf("services.%s missing %q", name, bubblesServiceLabelKey))
		}
	}
	if checked == 0 {
		return fmt.Errorf("contract violation: found no service carrying a %q label — the parser located no smackerel service blocks (compose shape changed?)", smackerelComponentKey)
	}
	if len(missing) > 0 {
		return fmt.Errorf("contract violation (knb spec 014 scope 03 / spec 101): %d shared-observability discovery-label gap(s): %v", len(missing), missing)
	}
	return nil
}

// TestSharedObservabilityLabels_DevComposeLiveFile asserts the live dev compose
// labels every smackerel service with the com.bubbles.* discovery labels.
func TestSharedObservabilityLabels_DevComposeLiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live dev compose %q: %v", composePath, err)
	}
	if err := assertSharedObservabilityLabels(yamlBytes); err != nil {
		t.Fatalf("live docker-compose.yml violates spec 101 shared-observability label contract: %v", err)
	}
}

// TestSharedObservabilityLabels_DeployComposeLiveFile asserts the live deploy
// compose labels every smackerel service with the com.bubbles.* discovery labels.
func TestSharedObservabilityLabels_DeployComposeLiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live deploy compose %q: %v", composePath, err)
	}
	if err := assertSharedObservabilityLabels(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates spec 101 shared-observability label contract: %v", err)
	}
}

// TestSharedObservabilityLabels_AdversarialMissingLabelRejected proves the
// contract is not tautological: a smackerel service missing com.bubbles.service
// MUST be rejected.
func TestSharedObservabilityLabels_AdversarialMissingLabelRejected(t *testing.T) {
	bad := []byte(`
services:
  smackerel-core:
    labels:
      com.smackerel.component: core
      com.smackerel.lifecycle: ephemeral
      com.bubbles.product: ${METRICS_SCRAPE_LABEL_PRODUCT}
`)
	if err := assertSharedObservabilityLabels(bad); err == nil {
		t.Fatal("expected a smackerel service missing com.bubbles.service to be REJECTED, but the contract accepted it (tautological test)")
	}
}

// TestSharedObservabilityLabels_CompliantSyntheticAccepted proves a fully
// labelled synthetic service passes (RED→GREEN symmetry with the adversarial case).
func TestSharedObservabilityLabels_CompliantSyntheticAccepted(t *testing.T) {
	good := []byte(`
services:
  smackerel-core:
    labels:
      com.smackerel.component: core
      com.smackerel.lifecycle: ephemeral
      com.bubbles.product: ${METRICS_SCRAPE_LABEL_PRODUCT}
      com.bubbles.service: core
`)
	if err := assertSharedObservabilityLabels(good); err != nil {
		t.Fatalf("expected a fully-labelled smackerel service to be ACCEPTED, got: %v", err)
	}
}
