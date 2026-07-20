// Package config — contract test for `config/release-trains.yaml` and the
// per-train `config/feature-flags.<train>.yaml` bundles.
//
// The release-train surface has TWO validation layers today:
//
//  1. `bash .github/bubbles/scripts/release-train-guard.sh` (yq-based,
//     pre-push hook) — but it requires yq on the host and isn't run by
//     `./smackerel.sh test`.
//  2. (none) — `go test ./...` does not exercise these files.
//
// A regression that (a) drops the `flags_bundle:` field from a train,
// (b) renames `target_slot:` to something invalid, (c) points
// `flags_bundle:` at a missing file, or (d) sets the bundle's `train:`
// to a value that doesn't match the train id in release-trains.yaml
// would slip past `./smackerel.sh test` and break:
//   - `bubbles.train` cut/promote/rollback/retire operations
//   - `release-train-guard.sh` (would then fail at pre-push, but only
//     for operators who have yq)
//   - the per-train flag bundles consumed by deploy adapters
//
// This Go contract test runs in `./smackerel.sh test unit --go` (no yq
// dependency, no docker), and asserts the structural invariants the
// shell guard already enforces. Adversarial sub-tests prove each
// invariant catches its target failure mode.
//
// References:
//   - .github/bubbles/scripts/release-train-guard.sh (yq-based equivalent)
//   - .github/instructions/bubbles-release-trains.instructions.md
//   - .github/skills/bubbles-release-train-model/SKILL.md
//   - .github/skills/bubbles-config-bundle-per-train/SKILL.md
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// trainsDoc is the minimal YAML shape this test needs.
type trainsDoc struct {
	Version  int           `yaml:"version"`
	Defaults trainDefaults `yaml:"defaults"`
	Trains   []trainDecl   `yaml:"trains"`
}

type trainDefaults struct {
	Retention       string `yaml:"retention"`
	PII             string `yaml:"pii"`
	OffsiteRequired *bool  `yaml:"offsite_required"`
}

type trainDecl struct {
	ID          string `yaml:"id"`
	Phase       string `yaml:"phase"`
	TargetSlot  string `yaml:"target_slot"`
	FlagsBundle string `yaml:"flags_bundle"`
	Description string `yaml:"description"`
	Retention   string `yaml:"retention"`
	PII         string `yaml:"pii"`
}

type bundleDoc struct {
	Version  int            `yaml:"version"`
	Train    string         `yaml:"train"`
	Flags    map[string]any `yaml:"flags"`
	Metadata map[string]any `yaml:"metadata"`
}

var validPhases = map[string]bool{
	"active":     true,
	"maintained": true,
	"frozen":     true,
	"retired":    true,
}

var validTargetSlots = map[string]bool{
	"prod":        true,
	"staging":     true,
	"self-hosted": true,
	"none":        true,
}

func releaseTrainsRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// assertTrainsContract validates release-trains.yaml structure WITHOUT
// resolving bundle file paths (so adversarial tests can pass synthetic
// YAML without writing files to disk). The live-file test additionally
// resolves bundles via assertBundleResolution.
func assertTrainsContract(yamlBytes []byte) error {
	var doc trainsDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	if doc.Version != 1 {
		return fmt.Errorf("contract violation: version=%d (expected 1)", doc.Version)
	}
	if len(doc.Trains) == 0 {
		return fmt.Errorf("contract violation: no trains declared (expected >= 1)")
	}

	// Build defaults fallback set so per-train retention/pii can omit them.
	defaultRetention := strings.TrimSpace(doc.Defaults.Retention)
	defaultPII := strings.TrimSpace(doc.Defaults.PII)

	seenIDs := make(map[string]bool, len(doc.Trains))
	for i, tr := range doc.Trains {
		where := fmt.Sprintf("trains[%d]", i)
		if strings.TrimSpace(tr.ID) == "" {
			return fmt.Errorf("contract violation: %s.id is empty", where)
		}
		if seenIDs[tr.ID] {
			return fmt.Errorf("contract violation: duplicate train id %q", tr.ID)
		}
		seenIDs[tr.ID] = true
		where = fmt.Sprintf("trains[%d] (id=%q)", i, tr.ID)

		if !validPhases[tr.Phase] {
			return fmt.Errorf("contract violation: %s.phase=%q (expected one of active|maintained|frozen|retired)", where, tr.Phase)
		}
		if !validTargetSlots[tr.TargetSlot] {
			return fmt.Errorf("contract violation: %s.target_slot=%q (expected one of prod|staging|self-hosted|none)", where, tr.TargetSlot)
		}
		if strings.TrimSpace(tr.FlagsBundle) == "" {
			return fmt.Errorf("contract violation: %s.flags_bundle is empty", where)
		}

		retention := strings.TrimSpace(tr.Retention)
		if retention == "" {
			retention = defaultRetention
		}
		if retention == "" {
			return fmt.Errorf("contract violation: %s missing retention (G118) — declare per-train or via defaults.retention", where)
		}

		pii := strings.TrimSpace(tr.PII)
		if pii == "" {
			pii = defaultPII
		}
		if pii == "" {
			return fmt.Errorf("contract violation: %s missing pii classification (G120) — declare per-train or via defaults.pii", where)
		}
	}

	return nil
}

// assertBundleResolution checks that every train's flags_bundle path
// resolves to a real file on disk and that the bundle's train: field
// matches the owning train's id.
func assertBundleResolution(repoRoot string, yamlBytes []byte) error {
	var doc trainsDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	for _, tr := range doc.Trains {
		bundlePath := filepath.Join(repoRoot, tr.FlagsBundle)
		b, err := os.ReadFile(bundlePath)
		if err != nil {
			return fmt.Errorf("contract violation: train %q flags_bundle %q does not resolve to a readable file: %v", tr.ID, tr.FlagsBundle, err)
		}
		var bdl bundleDoc
		if err := yaml.Unmarshal(b, &bdl); err != nil {
			return fmt.Errorf("contract violation: train %q flags_bundle %q is not valid YAML: %v", tr.ID, tr.FlagsBundle, err)
		}
		if bdl.Version != 1 {
			return fmt.Errorf("contract violation: train %q bundle %q has version=%d (expected 1)", tr.ID, tr.FlagsBundle, bdl.Version)
		}
		if bdl.Train != tr.ID {
			return fmt.Errorf("contract violation: train %q bundle %q declares train=%q (must match owning train id)", tr.ID, tr.FlagsBundle, bdl.Train)
		}
	}
	return nil
}

// TestReleaseTrainsContract_LiveFile parses the live release-trains.yaml
// and asserts both structural + bundle-resolution invariants.
func TestReleaseTrainsContract_LiveFile(t *testing.T) {
	root := releaseTrainsRepoRoot(t)
	trainsPath := filepath.Join(root, "config", "release-trains.yaml")
	b, err := os.ReadFile(trainsPath)
	if err != nil {
		t.Fatalf("read %s: %v", trainsPath, err)
	}
	if err := assertTrainsContract(b); err != nil {
		t.Fatalf("live release-trains.yaml violates contract: %v", err)
	}
	if err := assertBundleResolution(root, b); err != nil {
		t.Fatalf("live release-trains.yaml bundle resolution violates contract: %v", err)
	}
}

// TestReleaseTrainsContract_AdversarialMissingFlagsBundle proves the
// contract test would FAIL if a train declaration silently dropped its
// flags_bundle field — the most common silent-break failure mode.
func TestReleaseTrainsContract_AdversarialMissingFlagsBundle(t *testing.T) {
	bad := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: self-hosted
  description: "no flags_bundle"
`)
	err := assertTrainsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted train without flags_bundle — would break bubbles.train + deploy adapters")
	}
	if !strings.Contains(err.Error(), "flags_bundle is empty") {
		t.Fatalf("expected 'flags_bundle is empty' message, got: %v", err)
	}
}

// TestReleaseTrainsContract_AdversarialInvalidTargetSlot proves the
// contract test would FAIL if a target_slot regressed to a non-canonical
// value (e.g., a typo like "selfhosted" or an invented "dev"). The shell
// guard rejects this at pre-push but only for operators with yq.
func TestReleaseTrainsContract_AdversarialInvalidTargetSlot(t *testing.T) {
	bad := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: selfhosted
  flags_bundle: config/feature-flags.mvp.yaml
`)
	err := assertTrainsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted target_slot=selfhosted (typo) — would break bubbles.train slot routing")
	}
	if !strings.Contains(err.Error(), "target_slot") {
		t.Fatalf("expected target_slot rejection message, got: %v", err)
	}
}

// TestReleaseTrainsContract_AdversarialInvalidPhase proves the contract
// test would FAIL if a phase regressed to an invented status.
func TestReleaseTrainsContract_AdversarialInvalidPhase(t *testing.T) {
	bad := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: stable
  target_slot: self-hosted
  flags_bundle: config/feature-flags.mvp.yaml
`)
	err := assertTrainsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted phase=stable (invented) — would break bubbles.train lifecycle commands")
	}
	if !strings.Contains(err.Error(), "phase") {
		t.Fatalf("expected phase rejection message, got: %v", err)
	}
}

// TestReleaseTrainsContract_AdversarialDuplicateTrainID proves the
// contract test would FAIL if two trains accidentally collide on id —
// would cause non-deterministic yq queries in the shell guard.
func TestReleaseTrainsContract_AdversarialDuplicateTrainID(t *testing.T) {
	bad := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: self-hosted
  flags_bundle: config/feature-flags.mvp.yaml
- id: mvp
  phase: maintained
  target_slot: staging
  flags_bundle: config/feature-flags.mvp.yaml
`)
	err := assertTrainsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted duplicate train id — would break yq selectors in release-train-guard.sh")
	}
	if !strings.Contains(err.Error(), "duplicate train id") {
		t.Fatalf("expected duplicate-id rejection message, got: %v", err)
	}
}

// TestReleaseTrainsContract_AdversarialMissingRetention proves the
// contract test would FAIL if a train AND defaults.retention both
// omitted retention (Gate G118 — backup retention policy).
func TestReleaseTrainsContract_AdversarialMissingRetention(t *testing.T) {
	bad := []byte(`version: 1
defaults:
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: self-hosted
  flags_bundle: config/feature-flags.mvp.yaml
`)
	err := assertTrainsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted train without retention — would violate Gate G118 backup retention policy")
	}
	if !strings.Contains(err.Error(), "retention") {
		t.Fatalf("expected retention rejection message, got: %v", err)
	}
}

// TestReleaseTrainsBundleContract_AdversarialMismatchedTrainID proves the
// bundle-resolution test would FAIL if a bundle's train: field were
// changed to mismatch its owning train id (e.g., copy/paste of mvp.yaml
// into next.yaml without updating the `train:` field).
func TestReleaseTrainsBundleContract_AdversarialMismatchedTrainID(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	bundlePath := filepath.Join(root, "config", "feature-flags.mvp.yaml")
	bundleContent := []byte(`version: 1
train: next
flags: {}
metadata: {}
`)
	if err := os.WriteFile(bundlePath, bundleContent, 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}
	trains := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: self-hosted
  flags_bundle: config/feature-flags.mvp.yaml
`)
	err := assertBundleResolution(root, trains)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted bundle whose train: did not match owning train id — would silently mis-route flags")
	}
	if !strings.Contains(err.Error(), "declares train=") {
		t.Fatalf("expected train-mismatch rejection message, got: %v", err)
	}
}

// TestReleaseTrainsBundleContract_AdversarialMissingBundleFile proves the
// bundle-resolution test would FAIL if a train's flags_bundle path
// pointed at a non-existent file.
func TestReleaseTrainsBundleContract_AdversarialMissingBundleFile(t *testing.T) {
	root := t.TempDir()
	trains := []byte(`version: 1
defaults:
  retention: "7d"
  pii: "encrypted-only"
trains:
- id: mvp
  phase: active
  target_slot: self-hosted
  flags_bundle: config/feature-flags.does-not-exist.yaml
`)
	err := assertBundleResolution(root, trains)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted train pointing at missing bundle file — would silently break deploy adapter flag injection")
	}
	if !strings.Contains(err.Error(), "does not resolve to a readable file") {
		t.Fatalf("expected file-resolution rejection message, got: %v", err)
	}
}
