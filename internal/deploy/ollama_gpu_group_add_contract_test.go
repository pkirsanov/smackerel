// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Spec 082 SCOPE-082-09 — ROCm host-specific GID routing contract.
//
// The ollama service needs the host's `render` and `video` group GIDs in
// `group_add` to access /dev/dri/renderD128 for GPU inference. Those GIDs
// are HOST-SPECIFIC (a different distro/host assigns different render/video
// GIDs) — exactly the "value that changes when a different operator deploys"
// that the no-env-specific-content + deployment-ownership-boundary policy
// forbids as literals in this GENERIC compose. Before SCOPE-082-09 they were
// hardcoded as `["44","993"]`.
//
// Decision (design.md SCOPE-082-09):
//   - The render/video GIDs are routed to adapter-supplied fail-loud env
//     vars OLLAMA_RENDER_GID / OLLAMA_VIDEO_GID (Gate G028, no silent
//     default). The deploy adapter MUST emit both into app.env or compose
//     aborts at substitution time.
//   - The HSA_OVERRIDE_GFX_VERSION / HCC_AMDGPU_TARGET env values describe
//     the GPU CLASS (Strix Halo / gfx1151), not the host, so they REMAIN as
//     generic literals (documented generic default).
//
// Adversarial sub-test: a regression to a bare numeric-literal GID is
// rejected.
//
// Cross-reference:
//   - specs/082-mvp-target-readiness-hardening/spec.md FR-082-009
//   - specs/082-mvp-target-readiness-hardening/scenario-manifest.json SCN-082-I01
//   - deploy/compose.deploy.yml (services.ollama.group_add / environment)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeOllamaGPUDoc captures the ollama service's group_add list and
// environment map.
type composeOllamaGPUDoc struct {
	Services map[string]struct {
		GroupAdd    []string          `yaml:"group_add"`
		Environment map[string]string `yaml:"environment"`
	} `yaml:"services"`
}

// assertOllamaGPUGroupAddContract returns nil iff:
//  1. Every ollama group_add entry uses the fail-loud `${VAR:?...}` form
//     (no bare numeric host-GID literal).
//  2. Both OLLAMA_RENDER_GID and OLLAMA_VIDEO_GID are referenced.
//  3. The generic gfx env literals (HSA_OVERRIDE_GFX_VERSION,
//     HCC_AMDGPU_TARGET) remain present (the documented generic default).
func assertOllamaGPUGroupAddContract(yamlBytes []byte) error {
	var doc composeOllamaGPUDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	svc, ok := doc.Services["ollama"]
	if !ok {
		return fmt.Errorf("contract violation: services.ollama not found in compose document — SCOPE-082-09 governs the ollama GPU group_add routing")
	}
	if len(svc.GroupAdd) == 0 {
		return fmt.Errorf("contract violation: services.ollama.group_add is empty — SCOPE-082-09 requires render+video host GIDs routed via ${OLLAMA_RENDER_GID:?...}/${OLLAMA_VIDEO_GID:?...}")
	}
	for i, entry := range svc.GroupAdd {
		trimmed := strings.TrimSpace(entry)
		// A bare numeric literal (the pre-082 "44"/"993" form) is the
		// forbidden host-specific value.
		if isAllDigits(trimmed) {
			return fmt.Errorf("contract violation: services.ollama.group_add[%d]=%q is a bare numeric host-GID literal — SCOPE-082-09 forbids host-specific GID literals in the generic compose; route via the fail-loud ${OLLAMA_RENDER_GID:?...}/${OLLAMA_VIDEO_GID:?...} form (Gate G028) so the deploy adapter supplies the real host GID", i, entry)
		}
		if !strings.Contains(trimmed, "${") || !strings.Contains(trimmed, ":?") {
			return fmt.Errorf("contract violation: services.ollama.group_add[%d]=%q does not use the fail-loud ${VAR:?...} SST form — Gate G028 forbids silent defaults; the deploy adapter MUST supply the host GID", i, entry)
		}
		if strings.Contains(trimmed, ":-") {
			return fmt.Errorf("contract violation: services.ollama.group_add[%d]=%q uses the FORBIDDEN ${VAR:-default} fallback form — Gate G028 requires fail-loud ${VAR:?error}", i, entry)
		}
	}
	joined := strings.Join(svc.GroupAdd, " ")
	for _, required := range []string{"OLLAMA_RENDER_GID", "OLLAMA_VIDEO_GID"} {
		if !strings.Contains(joined, required) {
			return fmt.Errorf("contract violation: services.ollama.group_add does not reference %s — SCOPE-082-09 routes BOTH the render and video host GIDs via adapter-supplied env vars", required)
		}
	}
	// The gfx env literals describe the GPU class (generic), not the host —
	// they MUST remain so the accel tier targets the iGPU rather than CPU.
	for _, key := range []string{"HSA_OVERRIDE_GFX_VERSION", "HCC_AMDGPU_TARGET"} {
		if strings.TrimSpace(svc.Environment[key]) == "" {
			return fmt.Errorf("contract violation: services.ollama.environment[%s] is missing/empty — SCOPE-082-09 keeps the gfx-class env literals (generic default) so ROCm claims the iGPU; without it ollama silently falls back to CPU", key)
		}
	}
	return nil
}

// isAllDigits reports whether s (after stripping optional surrounding
// quotes) is a non-empty run of ASCII digits.
func isAllDigits(s string) bool {
	s = strings.Trim(s, `"'`)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// TestOllamaGPUGroupAdd_LiveFile asserts the live compose routes the ollama
// render/video GIDs through adapter-supplied fail-loud env vars while keeping
// the generic gfx env literals.
func TestOllamaGPUGroupAdd_LiveFile(t *testing.T) {
	composePath := filepath.Join(repoRoot(t), "deploy", "compose.deploy.yml")
	yamlBytes, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read live compose file %q: %v", composePath, err)
	}
	if err := assertOllamaGPUGroupAddContract(yamlBytes); err != nil {
		t.Fatalf("live deploy/compose.deploy.yml violates SCOPE-082-09 ROCm GID routing contract: %v", err)
	}
	t.Logf("contract OK: ollama group_add uses ${OLLAMA_RENDER_GID:?...}/${OLLAMA_VIDEO_GID:?...} fail-loud routing; gfx env literals retained (SCOPE-082-09)")
}

// TestOllamaGPUGroupAdd_AdversarialNumericLiteral proves the contract
// catches a regression back to the pre-082 bare numeric host-GID literal.
func TestOllamaGPUGroupAdd_AdversarialNumericLiteral(t *testing.T) {
	const fixture = `services:
  ollama:
    group_add:
      - "44"
      - "993"
    environment:
      HSA_OVERRIDE_GFX_VERSION: "11.5.1"
      HCC_AMDGPU_TARGET: "gfx1151"
`
	err := assertOllamaGPUGroupAddContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: bare numeric group_add literals (\"44\",\"993\") were ACCEPTED (a SCOPE-082-09 regression to host-specific GID literals would NOT be caught)")
	}
	if !strings.Contains(err.Error(), "group_add") {
		t.Fatalf("adversarial contract test failed: error did not mention 'group_add': %v", err)
	}
	if !strings.Contains(err.Error(), "numeric") {
		t.Fatalf("adversarial contract test failed: error did not flag the numeric host-GID literal: %v", err)
	}
	t.Logf("adversarial OK: bare numeric group_add GID is rejected with: %v", err)
}

// TestOllamaGPUGroupAdd_AdversarialMissingGfxEnv proves the contract also
// fails if the generic gfx-class env literal is dropped (which would let
// ROCm silently fall back to CPU on the accel tier).
func TestOllamaGPUGroupAdd_AdversarialMissingGfxEnv(t *testing.T) {
	const fixture = `services:
  ollama:
    group_add:
      - "${OLLAMA_RENDER_GID:?OLLAMA_RENDER_GID must be set by deploy adapter}"
      - "${OLLAMA_VIDEO_GID:?OLLAMA_VIDEO_GID must be set by deploy adapter}"
    environment:
      HCC_AMDGPU_TARGET: "gfx1151"
`
	err := assertOllamaGPUGroupAddContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: missing HSA_OVERRIDE_GFX_VERSION was ACCEPTED (a regression dropping the gfx-class pin would silently fall back to CPU)")
	}
	if !strings.Contains(err.Error(), "HSA_OVERRIDE_GFX_VERSION") {
		t.Fatalf("adversarial contract test failed: error did not mention 'HSA_OVERRIDE_GFX_VERSION': %v", err)
	}
	t.Logf("adversarial OK: missing gfx env literal is rejected with: %v", err)
}
