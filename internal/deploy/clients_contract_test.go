// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy — spec 085 client-binary contract test.
//
// The contract: `deploy/contract.yaml` MUST declare a top-level `clients:`
// block enumerating every native client this project builds. Smackerel ships a
// Flutter mobile assistant (clients/mobile/assistant) targeting Android only
// (android/ present, no clients/mobile/assistant/ios/ app target), so the block
// MUST declare exactly one well-shaped `android` artifact:
//
//	platform: android, variant: "-", kind ⊇ {aab, apk},
//	provenance: cosign-keyless, laneB: false
//
// Why this exists:
// `deploy/contract.yaml::clients` is the authority the knb spec-025 conformance
// gate (scripts/lint/client-binary-conformance.sh) reads to know which native
// platforms this repo ships. With the block present and non-`none`, the gate
// derives the contracted platform set and enforces the fail-closed manifest
// digest + provenance rule (check c) on the published build manifest. The block
// also pins the two-lane contract: Lane B (Play Store) is default-OFF
// (`laneB: false`), gated by the clientReleaseLaneB flag, and Lane A is the
// <deploy-host> self-host delivery performed by the knb self-hosted adapter.
//
// This test locks that shape permanently (mirrors
// external_images_contract_test.go). The adversarial sub-tests prove the check
// would FAIL if the android entry were removed, its provenance downgraded, its
// kind list truncated, its Lane-B flipped ON, or the block falsely declared
// `none: true` while still listing artifacts.
//
// Cross-reference:
//   - specs/085-client-binary-release/ (FR-CBR-001, FR-CBR-013, SCN-085-A01..A04)
//   - deploy/contract.yaml
//   - knb/smackerel/contract.yaml (the knb-side mirror, kept in lockstep)
//   - .github/instructions/bubbles-client-binary-release.instructions.md (canonical schema)
//   - knb/scripts/lint/client-binary-conformance.sh (the gate this contract feeds)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// clientArtifact is the minimal YAML shape of one `clients.artifacts[]` entry
// in deploy/contract.yaml. Only the fields the contract test enforces are read;
// adding unrelated artifact fields stays a non-event.
type clientArtifact struct {
	Platform    string   `yaml:"platform"`
	Variant     string   `yaml:"variant"`
	Kind        []string `yaml:"kind"`
	Ref         string   `yaml:"ref"`
	Provenance  string   `yaml:"provenance"`
	Embeds      []string `yaml:"embeds"`
	LaneB       bool     `yaml:"laneB"`
	LaneBTarget string   `yaml:"laneBTarget"`
}

// clientsContractDoc is the minimal YAML shape needed to read the top-level
// `clients:` block of deploy/contract.yaml.
type clientsContractDoc struct {
	Clients struct {
		None      bool             `yaml:"none"`
		Artifacts []clientArtifact `yaml:"artifacts"`
	} `yaml:"clients"`
}

// kindContains reports whether a kind list contains a given build artifact kind.
func kindContains(kinds []string, want string) bool {
	for _, k := range kinds {
		if strings.EqualFold(strings.TrimSpace(k), want) {
			return true
		}
	}
	return false
}

// assertClientsContract returns nil iff the smackerel `clients:` block invariants
// hold. On any violation it returns a non-nil error naming the specific platform
// and the specific violation, so the adversarial sub-tests can pattern-match the
// failure mode.
func assertClientsContract(doc clientsContractDoc) error {
	c := doc.Clients

	// Smackerel ships a native client, so the block MUST NOT claim `none: true`
	// while listing artifacts — that is a self-contradicting lie the knb gate
	// check (b) also catches against detected source.
	if c.None && len(c.Artifacts) > 0 {
		return fmt.Errorf("contract violation: clients.none=true but clients.artifacts lists %d entry(ies) — a repo that ships a native client MUST declare none: false", len(c.Artifacts))
	}
	if len(c.Artifacts) == 0 {
		return fmt.Errorf("contract violation: deploy/contract.yaml::clients.artifacts is empty — smackerel ships a Flutter Android client (clients/mobile/assistant) that MUST be declared (FR-CBR-001)")
	}

	// No artifact may have an empty platform; no duplicate platform+variant.
	seen := map[string]bool{}
	for i, a := range c.Artifacts {
		if strings.TrimSpace(a.Platform) == "" {
			return fmt.Errorf("contract violation: clients.artifacts[%d] has an empty platform", i)
		}
		key := a.Platform + "/" + a.Variant
		if seen[key] {
			return fmt.Errorf("contract violation: clients.artifacts has duplicate platform/variant %q — each client artifact must be listed once", key)
		}
		seen[key] = true
	}

	// The android artifact MUST be present and well-shaped (FR-CBR-001).
	var android *clientArtifact
	for i := range c.Artifacts {
		if strings.EqualFold(c.Artifacts[i].Platform, "android") {
			android = &c.Artifacts[i]
			break
		}
	}
	if android == nil {
		return fmt.Errorf("contract violation: deploy/contract.yaml::clients.artifacts has no `android` entry — smackerel's native client targets Android (clients/mobile/assistant/android) and MUST be declared (FR-CBR-001)")
	}
	if android.Variant != "-" {
		return fmt.Errorf("contract violation: clients.artifacts[android].variant=%q, expected \"-\" (android is a single-variant platform)", android.Variant)
	}
	if !kindContains(android.Kind, "aab") {
		return fmt.Errorf("contract violation: clients.artifacts[android].kind=%v is missing `aab` — the Play Store / Lane-B artifact (FR-CBR-001)", android.Kind)
	}
	if !kindContains(android.Kind, "apk") {
		return fmt.Errorf("contract violation: clients.artifacts[android].kind=%v is missing `apk` — the sideload / Lane-A artifact (FR-CBR-001)", android.Kind)
	}
	if android.Provenance != "cosign-keyless" {
		return fmt.Errorf("contract violation: clients.artifacts[android].provenance=%q, expected \"cosign-keyless\" — always-on CI provenance is mandatory (knb FR-005; the gate refuses a manifest artifact lacking it)", android.Provenance)
	}
	if android.LaneB {
		return fmt.Errorf("contract violation: clients.artifacts[android].laneB=true — Lane B (Play Store) MUST be default-OFF (false); activation is operator-driven via the clientReleaseLaneB flag, never the contract (FR-CBR-008/009)")
	}

	return nil
}

// TestClientsContract_LiveFiles parses the live deploy/contract.yaml and asserts
// the clients-block contract holds (SCN-085-A01/A02). It also smoke-checks that
// no `ios` artifact is declared today (SCN-085-A04 — ios is RESERVED, not
// declared, because no clients/mobile/assistant/ios/ app target exists).
func TestClientsContract_LiveFiles(t *testing.T) {
	root := repoRoot(t)

	contractPath := filepath.Join(root, "deploy", "contract.yaml")
	contractBytes, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("read %s: %v", contractPath, err)
	}
	var doc clientsContractDoc
	if err := yaml.Unmarshal(contractBytes, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal %s: %v", contractPath, err)
	}

	if err := assertClientsContract(doc); err != nil {
		t.Fatalf("live-file clients contract violation: %v", err)
	}

	// Smoke check (SCN-085-A04): ios is reserved, not declared. When a future
	// spec adds a clients/mobile/assistant/ios/ app target it adds the ios entry
	// here and updates this smoke check — exactly like the prometheus smoke in
	// external_images_contract_test.go.
	for _, a := range doc.Clients.Artifacts {
		if strings.EqualFold(a.Platform, "ios") {
			t.Fatalf("smoke check: clients.artifacts declares an `ios` entry, but no clients/mobile/assistant/ios/ app target exists (SCN-085-A04 — ios is RESERVED for a future spec; do not declare it before the build target lands)")
		}
	}

	// Diagnostic clarity: confirm the android entry resolves to the smackerel
	// client registry (the immutable OCI artifact repo the manifest digest-pins).
	for _, a := range doc.Clients.Artifacts {
		if strings.EqualFold(a.Platform, "android") && !strings.Contains(a.Ref, "smackerel-clients") {
			t.Fatalf("smoke check: clients.artifacts[android].ref=%q does not reference the smackerel-clients OCI artifact repo (ghcr.io/pkirsanov/smackerel-clients)", a.Ref)
		}
	}
}

// TestClientsContract_AdversarialMissingAndroid proves the contract fails if the
// android entry is dropped (the regression the drift lock exists to catch).
func TestClientsContract_AdversarialMissingAndroid(t *testing.T) {
	doc := clientsContractDoc{}
	doc.Clients.None = false
	doc.Clients.Artifacts = []clientArtifact{
		// A non-android entry only — android is MISSING.
		{Platform: "roku", Variant: "scenegraph", Kind: []string{"pkg"}, Provenance: "cosign-keyless"},
	}

	err := assertClientsContract(doc)
	if err == nil {
		t.Fatal("adversarial contract test failed: dropping the android entry was ACCEPTED (the contract is tautological — it would NOT catch the FR-CBR-001 regression)")
	}
	if !strings.Contains(err.Error(), "android") {
		t.Fatalf("adversarial contract test failed: error did not mention 'android': %v", err)
	}
	t.Logf("adversarial OK: missing android rejected with: %v", err)
}

// TestClientsContract_AdversarialWrongProvenance proves the contract fails if the
// android artifact's provenance is downgraded from cosign-keyless.
func TestClientsContract_AdversarialWrongProvenance(t *testing.T) {
	doc := clientsContractDoc{}
	doc.Clients.None = false
	doc.Clients.Artifacts = []clientArtifact{
		{Platform: "android", Variant: "-", Kind: []string{"aab", "apk"}, Provenance: "cosign", LaneB: false},
	}

	err := assertClientsContract(doc)
	if err == nil {
		t.Fatal("adversarial contract test failed: a non-cosign-keyless provenance was ACCEPTED (always-on provenance is unenforced)")
	}
	if !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("adversarial contract test failed: error did not mention 'provenance': %v", err)
	}
	t.Logf("adversarial OK: wrong provenance rejected with: %v", err)
}

// TestClientsContract_AdversarialLaneBOn proves the contract fails if Lane B is
// flipped ON in the contract (activation MUST be flag-driven, never the
// contract).
func TestClientsContract_AdversarialLaneBOn(t *testing.T) {
	doc := clientsContractDoc{}
	doc.Clients.None = false
	doc.Clients.Artifacts = []clientArtifact{
		{Platform: "android", Variant: "-", Kind: []string{"aab", "apk"}, Provenance: "cosign-keyless", LaneB: true},
	}

	err := assertClientsContract(doc)
	if err == nil {
		t.Fatal("adversarial contract test failed: laneB=true was ACCEPTED (Lane B default-OFF is unenforced — FR-CBR-008/009)")
	}
	if !strings.Contains(err.Error(), "laneB") {
		t.Fatalf("adversarial contract test failed: error did not mention 'laneB': %v", err)
	}
	t.Logf("adversarial OK: laneB=true rejected with: %v", err)
}

// TestClientsContract_AdversarialTruncatedKind proves the contract fails if the
// android kind list drops `apk` (Lane-A sideload) or `aab` (Lane-B Play Store).
func TestClientsContract_AdversarialTruncatedKind(t *testing.T) {
	doc := clientsContractDoc{}
	doc.Clients.None = false
	doc.Clients.Artifacts = []clientArtifact{
		{Platform: "android", Variant: "-", Kind: []string{"aab"}, Provenance: "cosign-keyless", LaneB: false},
	}

	err := assertClientsContract(doc)
	if err == nil {
		t.Fatal("adversarial contract test failed: an android entry missing `apk` was ACCEPTED (kind completeness is unenforced)")
	}
	if !strings.Contains(err.Error(), "apk") {
		t.Fatalf("adversarial contract test failed: error did not mention 'apk': %v", err)
	}
	t.Logf("adversarial OK: truncated kind rejected with: %v", err)
}

// TestClientsContract_AdversarialNoneTrueWithArtifacts proves the contract fails
// if the block falsely claims `none: true` while still listing artifacts.
func TestClientsContract_AdversarialNoneTrueWithArtifacts(t *testing.T) {
	doc := clientsContractDoc{}
	doc.Clients.None = true
	doc.Clients.Artifacts = []clientArtifact{
		{Platform: "android", Variant: "-", Kind: []string{"aab", "apk"}, Provenance: "cosign-keyless", LaneB: false},
	}

	err := assertClientsContract(doc)
	if err == nil {
		t.Fatal("adversarial contract test failed: none=true with listed artifacts was ACCEPTED (the self-contradiction is unenforced)")
	}
	if !strings.Contains(err.Error(), "none") {
		t.Fatalf("adversarial contract test failed: error did not mention 'none': %v", err)
	}
	t.Logf("adversarial OK: none=true + artifacts rejected with: %v", err)
}
