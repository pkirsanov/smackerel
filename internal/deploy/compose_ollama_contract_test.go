// Spec 043 Scope 1 — T1-03 + T1-04 compose contract for the ollama service.
//
// docker-compose.yml hosts the ollama service for both dev and test stacks
// (per-env isolation comes from the compose project name and the
// OLLAMA_VOLUME_NAME env var). The contract this test enforces:
//
//  1. The `ollama` service exists in `docker-compose.yml`.
//  2. The service uses `image: ${OLLAMA_IMAGE}` (the image is hoisted to
//     the SST per design.md §3 OQ-D1, NOT a literal `ollama/ollama:<tag>`).
//  3. The service is gated behind `profiles: [ollama]` so dev stays
//     opt-in (FR-OLLAMA-007) and test stays auto-enabled via
//     ENABLE_OLLAMA=true wiring.
//  4. The named volume `ollama-data` resolves to `${OLLAMA_VOLUME_NAME}`
//     so dev and test cannot collide (SCN-OLLAMA-005 isolation).
//
// Two adversarial sub-tests prove the contract function would FAIL if
// either the image hoist regressed to a literal tag or if the volume name
// regressed to a hardcoded string.
//
// References:
//   - specs/043-ollama-test-infrastructure/spec.md (FR-OLLAMA-006, FR-OLLAMA-008)
//   - specs/043-ollama-test-infrastructure/design.md §3 (configuration plan)
//   - specs/043-ollama-test-infrastructure/scopes.md (Scope 1, T1-03/T1-04)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// ollamaComposeDoc is the minimal YAML shape the ollama contract needs.
type ollamaComposeDoc struct {
	Services map[string]struct {
		Image    string   `yaml:"image"`
		Profiles []string `yaml:"profiles"`
		Volumes  []string `yaml:"volumes"`
	} `yaml:"services"`
	Volumes map[string]struct {
		Name string `yaml:"name"`
	} `yaml:"volumes"`
}

const (
	requiredOllamaImage         = "${OLLAMA_IMAGE}"
	requiredOllamaProfile       = "ollama"
	requiredOllamaVolumeRefName = "${OLLAMA_VOLUME_NAME}"
)

// assertOllamaComposeContract returns nil iff the four invariants hold.
// Returns a non-nil error naming the specific service and the specific
// violation so the adversarial sub-tests can pattern-match the failure.
func assertOllamaComposeContract(yamlBytes []byte) error {
	var doc ollamaComposeDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	ollama, ok := doc.Services["ollama"]
	if !ok {
		return fmt.Errorf("contract violation: services.ollama not found in compose document")
	}

	if ollama.Image != requiredOllamaImage {
		return fmt.Errorf("contract violation: services.ollama.image=%q is not %q (spec 043 hoists the Ollama image to the SST; literal ollama/ollama:<tag> is forbidden in compose)",
			ollama.Image, requiredOllamaImage)
	}

	hasProfile := false
	for _, p := range ollama.Profiles {
		if p == requiredOllamaProfile {
			hasProfile = true
			break
		}
	}
	if !hasProfile {
		return fmt.Errorf("contract violation: services.ollama.profiles=%v does not include %q (FR-OLLAMA-007 requires profile-gating so dev stays opt-in)",
			ollama.Profiles, requiredOllamaProfile)
	}

	hasOllamaDataMount := false
	for _, v := range ollama.Volumes {
		if strings.HasPrefix(v, "ollama-data:") {
			hasOllamaDataMount = true
			break
		}
	}
	if !hasOllamaDataMount {
		return fmt.Errorf("contract violation: services.ollama.volumes=%v does not mount the ollama-data named volume (test isolation per SCN-OLLAMA-005 depends on the named-volume indirection)",
			ollama.Volumes)
	}

	vol, ok := doc.Volumes["ollama-data"]
	if !ok {
		return fmt.Errorf("contract violation: top-level volumes.ollama-data not declared (cannot honor per-env OLLAMA_VOLUME_NAME indirection without it)")
	}
	if vol.Name != requiredOllamaVolumeRefName {
		return fmt.Errorf("contract violation: volumes.ollama-data.name=%q is not %q (per-env isolation requires OLLAMA_VOLUME_NAME indirection — hardcoding the name collapses dev and test onto the same volume)",
			vol.Name, requiredOllamaVolumeRefName)
	}

	return nil
}

// TestOllamaComposeContract_LiveFile is the primary contract assertion.
// It loads docker-compose.yml from the repo root and proves the four
// invariants hold. Would FAIL if any future edit regresses the contract.
func TestOllamaComposeContract_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "docker-compose.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertOllamaComposeContract(yamlBytes); err != nil {
		t.Fatalf("live docker-compose.yml violates spec 043 ollama-service contract: %v", err)
	}
	t.Logf("contract OK: docker-compose.yml ollama service uses ${OLLAMA_IMAGE}, has profile gate, mounts ollama-data which resolves via ${OLLAMA_VOLUME_NAME}")
}

// TestOllamaComposeContract_AdversarialLiteralImage proves the contract
// catches a regression where the image is hardcoded back to a literal tag.
func TestOllamaComposeContract_AdversarialLiteralImage(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ollama/ollama:0.6
    profiles: [ollama]
    volumes:
      - ollama-data:/root/.ollama
volumes:
  ollama-data:
    name: ${OLLAMA_VOLUME_NAME}
`
	err := assertOllamaComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: literal image tag was accepted (the contract is tautological — it would NOT catch a regression that hardcodes ollama/ollama:<tag> back into compose)")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Fatalf("adversarial contract test failed: error did not mention 'image': %v", err)
	}
	if !strings.Contains(err.Error(), "spec 043") {
		t.Fatalf("adversarial contract test failed: error did not mention 'spec 043': %v", err)
	}
	t.Logf("adversarial OK: literal image tag rejected with: %v", err)
}

// TestOllamaComposeContract_AdversarialHardcodedVolumeName proves the
// contract catches a regression where the volume name is hardcoded
// (collapsing dev and test onto the same named volume — breaking test
// isolation per SCN-OLLAMA-005).
func TestOllamaComposeContract_AdversarialHardcodedVolumeName(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ${OLLAMA_IMAGE}
    profiles: [ollama]
    volumes:
      - ollama-data:/root/.ollama
volumes:
  ollama-data:
    name: smackerel-ollama-data
`
	err := assertOllamaComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: hardcoded volume name was accepted (the contract is tautological — it would NOT catch a regression that breaks per-env volume isolation)")
	}
	if !strings.Contains(err.Error(), "ollama-data") {
		t.Fatalf("adversarial contract test failed: error did not mention 'ollama-data': %v", err)
	}
	if !strings.Contains(err.Error(), "OLLAMA_VOLUME_NAME") {
		t.Fatalf("adversarial contract test failed: error did not mention 'OLLAMA_VOLUME_NAME' (the indirection point that maintains dev/test isolation): %v", err)
	}
	t.Logf("adversarial OK: hardcoded volume name rejected with: %v", err)
}

// TestOllamaComposeContract_AdversarialMissingProfile proves the contract
// catches a regression where the profile gate is removed (which would
// auto-start ollama in dev, violating FR-OLLAMA-007).
func TestOllamaComposeContract_AdversarialMissingProfile(t *testing.T) {
	const fixture = `services:
  ollama:
    image: ${OLLAMA_IMAGE}
    volumes:
      - ollama-data:/root/.ollama
volumes:
  ollama-data:
    name: ${OLLAMA_VOLUME_NAME}
`
	err := assertOllamaComposeContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: missing profile gate was accepted (the contract is tautological — it would NOT catch a regression that auto-starts ollama in dev)")
	}
	if !strings.Contains(err.Error(), "profile") {
		t.Fatalf("adversarial contract test failed: error did not mention 'profile': %v", err)
	}
	if !strings.Contains(err.Error(), "FR-OLLAMA-007") {
		t.Fatalf("adversarial contract test failed: error did not mention 'FR-OLLAMA-007': %v", err)
	}
	t.Logf("adversarial OK: missing profile gate rejected with: %v", err)
}
