// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — BUG-049-001 external-image contract test.
//
// The contract: every service in `deploy/compose.deploy.yml` that is NOT
// built by this project (i.e. not `smackerel-core` and not `smackerel-ml`)
// MUST be enumerated by `name` in `deploy/contract.yaml::externalImages`.
// In addition, every externalImages entry whose corresponding compose
// service declares a literal image (no `${...}` substitution) MUST match
// the externalImages entry's `image` value byte-for-byte.
//
// Why this exists:
// `deploy/contract.yaml::externalImages` is the operator-facing summary of
// third-party, pinned-for-reproducibility images. Deploy-adapter overlays
// consume it to know which images to pre-pull (offline cache, air-gapped
// mirror, signature-verification audit-trail). Before BUG-049-001 the list
// drifted to omit `prom/prometheus:v2.55.1` (added to compose by spec 049
// as a profile-gated service). Adapter overlays would silently miss the
// image when an operator first enabled `--profile monitoring`.
//
// This test closes that drift permanently. Adversarial sub-tests prove
// the check would fail if any third-party service in compose were dropped
// from the contract.
//
// Cross-reference:
//   - specs/049-monitoring-stack/bugs/BUG-049-001-prometheus-external-image-contract-drift/
//   - deploy/contract.yaml
//   - deploy/compose.deploy.yml
//   - internal/deploy/compose_contract_test.go (sibling spec 042 contract)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeImageDoc is the minimal YAML shape needed to enumerate every
// service's declared image. We only read `image` (no ports, env, deploy,
// etc.) so adding unrelated compose fields stays a non-event.
type composeImageDoc struct {
	Services map[string]struct {
		Image string `yaml:"image"`
	} `yaml:"services"`
}

// externalImagesDoc is the minimal YAML shape needed to enumerate every
// externalImages entry. The optional `profile:` field is captured so
// future regressions that drop the field can be diagnosed in the failure
// message, but the field is informational and not enforced here.
type externalImagesDoc struct {
	ExternalImages []struct {
		Name    string `yaml:"name"`
		Image   string `yaml:"image"`
		Profile string `yaml:"profile,omitempty"`
	} `yaml:"externalImages"`
}

// projectBuiltServices enumerates the compose services whose images are
// produced by THIS project's CI build job. Their image strings come from
// the `images:` block in `deploy/contract.yaml` (cosign-verified, etc.)
// and MUST NOT appear in `externalImages`. Any future project-built
// service added to compose must be appended here.
var projectBuiltServices = map[string]bool{
	"smackerel-core": true,
	"smackerel-ml":   true,
}

// assertExternalImagesContract returns nil iff both invariants hold for
// the parsed compose document and the parsed externalImages list. The
// error names the specific service or image so adversarial sub-tests can
// pattern-match the failure mode.
func assertExternalImagesContract(compose composeImageDoc, contract externalImagesDoc) error {
	// Compute the set of non-built compose services (the "should be in
	// externalImages" set).
	composeExternalServices := map[string]string{} // name -> declared image
	for svc, def := range compose.Services {
		if projectBuiltServices[svc] {
			// Project-built; must NOT appear in externalImages. Skip.
			continue
		}
		if strings.TrimSpace(def.Image) == "" {
			return fmt.Errorf("contract violation: services.%s has empty image — every compose service must declare an image", svc)
		}
		composeExternalServices[svc] = def.Image
	}

	// Compute the set of externalImages names (the "advertised" set).
	contractByName := map[string]string{} // name -> image
	for _, e := range contract.ExternalImages {
		if strings.TrimSpace(e.Name) == "" {
			return fmt.Errorf("contract violation: deploy/contract.yaml::externalImages entry has empty name (image=%q)", e.Image)
		}
		if strings.TrimSpace(e.Image) == "" {
			return fmt.Errorf("contract violation: deploy/contract.yaml::externalImages entry name=%q has empty image", e.Name)
		}
		if _, dup := contractByName[e.Name]; dup {
			return fmt.Errorf("contract violation: deploy/contract.yaml::externalImages has duplicate name=%q — each external image must be listed once", e.Name)
		}
		contractByName[e.Name] = e.Image
	}

	// Check 1 — every non-built compose service must be advertised.
	missing := []string{}
	for svc := range composeExternalServices {
		if _, ok := contractByName[svc]; !ok {
			missing = append(missing, svc)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		return fmt.Errorf("contract violation: deploy/contract.yaml::externalImages is missing entries for non-built compose services %v — every non-built service in deploy/compose.deploy.yml MUST be enumerated by name in externalImages so adapter overlays can pre-pull/cache them (BUG-049-001)", missing)
	}

	// Check 2 — every advertised externalImages entry must correspond to a
	// real non-built compose service. A stale entry is just as bad as a
	// missing entry because it leads adapter overlays to pre-pull images
	// the runtime stack does not actually use.
	stale := []string{}
	for name := range contractByName {
		if _, ok := composeExternalServices[name]; !ok {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		return fmt.Errorf("contract violation: deploy/contract.yaml::externalImages has stale entries %v — these names do not correspond to any non-built service in deploy/compose.deploy.yml (BUG-049-001)", stale)
	}

	// Check 3 — for services whose compose `image` field is a literal
	// (no ${...} substitution), the externalImages entry MUST match
	// byte-for-byte. For SST-substituted images (e.g. ${PROMETHEUS_IMAGE})
	// we do NOT resolve the variable here; the spec 045/049 contract
	// tests already verify the substitution path. The presence-of-name
	// check above is sufficient to lock the drift this test exists to
	// prevent.
	for svc, composeImage := range composeExternalServices {
		if strings.Contains(composeImage, "${") {
			// SST-substituted — name presence already enforced.
			continue
		}
		contractImage := contractByName[svc]
		if composeImage != contractImage {
			return fmt.Errorf("contract violation: services.%s declares literal image %q but deploy/contract.yaml::externalImages[name=%s].image is %q — pinned literal images MUST match byte-for-byte (BUG-049-001)", svc, composeImage, svc, contractImage)
		}
	}

	return nil
}

// TestExternalImagesContract_LiveFiles parses the live
// deploy/compose.deploy.yml and deploy/contract.yaml and asserts the
// contract holds.
func TestExternalImagesContract_LiveFiles(t *testing.T) {
	root := repoRoot(t)

	composePath := filepath.Join(root, "deploy", "compose.deploy.yml")
	composeBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read %s: %v", composePath, err)
	}
	var compose composeImageDoc
	if err := yaml.Unmarshal(composeBytes, &compose); err != nil {
		t.Fatalf("yaml.Unmarshal %s: %v", composePath, err)
	}

	contractPath := filepath.Join(root, "deploy", "contract.yaml")
	contractBytes, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s: %v", contractPath, err)
	}
	var contract externalImagesDoc
	if err := yaml.Unmarshal(contractBytes, &contract); err != nil {
		t.Fatalf("yaml.Unmarshal %s: %v", contractPath, err)
	}

	if err := assertExternalImagesContract(compose, contract); err != nil {
		t.Fatalf("live-file contract violation: %v", err)
	}

	// Smoke check: the contract should advertise prometheus (the entry
	// BUG-049-001 added). If a future edit drops it, Check 1 above fires
	// first, but log the presence here for diagnostic clarity.
	foundProm := false
	for _, e := range contract.ExternalImages {
		if e.Name == "prometheus" {
			foundProm = true
			if e.Image != "prom/prometheus:v2.55.1" {
				t.Fatalf("smoke check: externalImages[name=prometheus].image=%q, expected %q (BUG-049-001 added this pin; any change requires a design discussion + adversarial regression test re-run per the pattern set by config/smackerel.yaml::monitoring.prometheus.image)", e.Image, "prom/prometheus:v2.55.1")
			}
			if e.Profile != "monitoring" {
				t.Fatalf("smoke check: externalImages[name=prometheus].profile=%q, expected \"monitoring\" (BUG-049-001 set this gating)", e.Profile)
			}
		}
	}
	if !foundProm {
		t.Fatal("smoke check: externalImages does not contain a `prometheus` entry — BUG-049-001 required it")
	}
}

// TestExternalImagesContract_AdversarialMissingPrometheus proves the
// contract fails if the prometheus entry is dropped from externalImages
// (the exact regression BUG-049-001 fixed).
func TestExternalImagesContract_AdversarialMissingPrometheus(t *testing.T) {
	compose := composeImageDoc{Services: map[string]struct {
		Image string `yaml:"image"`
	}{
		"smackerel-core": {Image: "${SMACKEREL_CORE_IMAGE}"},
		"smackerel-ml":   {Image: "${SMACKEREL_ML_IMAGE}"},
		"postgres":       {Image: "pgvector/pgvector:pg16"},
		"nats":           {Image: "nats:2.10-alpine"},
		"ollama":         {Image: "ollama/ollama:0.23.2"},
		"prometheus":     {Image: "${PROMETHEUS_IMAGE}"},
	}}
	contract := externalImagesDoc{ExternalImages: []struct {
		Name    string `yaml:"name"`
		Image   string `yaml:"image"`
		Profile string `yaml:"profile,omitempty"`
	}{
		{Name: "postgres", Image: "pgvector/pgvector:pg16"},
		{Name: "nats", Image: "nats:2.10-alpine"},
		{Name: "ollama", Image: "ollama/ollama:0.23.2"},
		// prometheus intentionally OMITTED — this is the BUG-049-001 regression.
	}}

	err := assertExternalImagesContract(compose, contract)
	if err == nil {
		t.Fatal("adversarial contract test failed: dropping prometheus from externalImages was ACCEPTED (the contract is tautological — it would NOT catch the BUG-049-001 regression)")
	}
	if !strings.Contains(err.Error(), "prometheus") {
		t.Fatalf("adversarial contract test failed: error did not mention 'prometheus': %v", err)
	}
	t.Logf("adversarial OK: missing prometheus rejected with: %v", err)
}

// TestExternalImagesContract_AdversarialMissingNats proves the contract
// fails for any non-built service drop (not just prometheus) — checks
// the assertion is generic, not hardcoded.
func TestExternalImagesContract_AdversarialMissingNats(t *testing.T) {
	compose := composeImageDoc{Services: map[string]struct {
		Image string `yaml:"image"`
	}{
		"smackerel-core": {Image: "${SMACKEREL_CORE_IMAGE}"},
		"smackerel-ml":   {Image: "${SMACKEREL_ML_IMAGE}"},
		"postgres":       {Image: "pgvector/pgvector:pg16"},
		"nats":           {Image: "nats:2.10-alpine"},
		"ollama":         {Image: "ollama/ollama:0.23.2"},
	}}
	contract := externalImagesDoc{ExternalImages: []struct {
		Name    string `yaml:"name"`
		Image   string `yaml:"image"`
		Profile string `yaml:"profile,omitempty"`
	}{
		{Name: "postgres", Image: "pgvector/pgvector:pg16"},
		// nats intentionally OMITTED.
		{Name: "ollama", Image: "ollama/ollama:0.23.2"},
	}}

	err := assertExternalImagesContract(compose, contract)
	if err == nil {
		t.Fatal("adversarial contract test failed: dropping nats from externalImages was ACCEPTED (the BUG-049-001 contract is hardcoded to prometheus only — it would NOT catch a generic drift)")
	}
	if !strings.Contains(err.Error(), "nats") {
		t.Fatalf("adversarial contract test failed: error did not mention 'nats': %v", err)
	}
	t.Logf("adversarial OK: missing nats rejected with: %v", err)
}

// TestExternalImagesContract_AdversarialStaleEntry proves the contract
// also fails if externalImages lists a name that no longer exists as a
// non-built service in compose — stale entries mislead adapter overlays
// into pre-pulling images the runtime no longer uses.
func TestExternalImagesContract_AdversarialStaleEntry(t *testing.T) {
	compose := composeImageDoc{Services: map[string]struct {
		Image string `yaml:"image"`
	}{
		"smackerel-core": {Image: "${SMACKEREL_CORE_IMAGE}"},
		"smackerel-ml":   {Image: "${SMACKEREL_ML_IMAGE}"},
		"postgres":       {Image: "pgvector/pgvector:pg16"},
		"nats":           {Image: "nats:2.10-alpine"},
		"ollama":         {Image: "ollama/ollama:0.23.2"},
	}}
	contract := externalImagesDoc{ExternalImages: []struct {
		Name    string `yaml:"name"`
		Image   string `yaml:"image"`
		Profile string `yaml:"profile,omitempty"`
	}{
		{Name: "postgres", Image: "pgvector/pgvector:pg16"},
		{Name: "nats", Image: "nats:2.10-alpine"},
		{Name: "ollama", Image: "ollama/ollama:0.23.2"},
		// Stale entry — `redis` is not in compose but is advertised.
		{Name: "redis", Image: "redis:7-alpine"},
	}}

	err := assertExternalImagesContract(compose, contract)
	if err == nil {
		t.Fatal("adversarial contract test failed: stale `redis` entry was ACCEPTED (the contract does not catch advertised images the runtime no longer uses)")
	}
	if !strings.Contains(err.Error(), "redis") {
		t.Fatalf("adversarial contract test failed: error did not mention 'redis': %v", err)
	}
	t.Logf("adversarial OK: stale redis rejected with: %v", err)
}

// TestExternalImagesContract_AdversarialLiteralImageMismatch proves the
// contract fails if a literal image in compose drifts from the
// externalImages pin (e.g. a contributor bumps nats in compose but
// forgets to bump deploy/contract.yaml).
func TestExternalImagesContract_AdversarialLiteralImageMismatch(t *testing.T) {
	compose := composeImageDoc{Services: map[string]struct {
		Image string `yaml:"image"`
	}{
		"smackerel-core": {Image: "${SMACKEREL_CORE_IMAGE}"},
		"smackerel-ml":   {Image: "${SMACKEREL_ML_IMAGE}"},
		"postgres":       {Image: "pgvector/pgvector:pg16"},
		// nats bumped in compose...
		"nats":   {Image: "nats:2.11-alpine"},
		"ollama": {Image: "ollama/ollama:0.23.2"},
	}}
	contract := externalImagesDoc{ExternalImages: []struct {
		Name    string `yaml:"name"`
		Image   string `yaml:"image"`
		Profile string `yaml:"profile,omitempty"`
	}{
		{Name: "postgres", Image: "pgvector/pgvector:pg16"},
		// ...but contract still pins the old tag.
		{Name: "nats", Image: "nats:2.10-alpine"},
		{Name: "ollama", Image: "ollama/ollama:0.23.2"},
	}}

	err := assertExternalImagesContract(compose, contract)
	if err == nil {
		t.Fatal("adversarial contract test failed: literal image mismatch (nats:2.11-alpine vs nats:2.10-alpine) was ACCEPTED (the contract does not catch tag drift between compose and the externalImages pin)")
	}
	if !strings.Contains(err.Error(), "nats") || !strings.Contains(err.Error(), "2.11") {
		t.Fatalf("adversarial contract test failed: error did not mention 'nats' and '2.11': %v", err)
	}
	t.Logf("adversarial OK: literal image mismatch rejected with: %v", err)
}
