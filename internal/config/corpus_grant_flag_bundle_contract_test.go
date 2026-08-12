// Package config — TP-05-01 contract test for the spec 108 release-train flag
// `corpusGrantEnforcement`.
//
// Spec 108 ships a two-stage OBSERVE→ENFORCE rollout. The flag it introduces
// must satisfy three separate obligations that no existing test covered:
//
//   - R-108-FL2 — the flag is DECLARED in every train bundle. A flag that is
//     silently dropped from one bundle leaves that train resolving nothing.
//   - R-108-FL3 — the flag ships default-OFF (`false`) in EVERY train,
//     including the owning train `next`. A default-ON `next` would arrive
//     already enforcing, destroying the observation window Scope 04 exists to
//     produce and denying callers that have not yet received the `corpus:read`
//     token rotation `docs/smackerel.md` §17.2 requires.
//   - R-108-FL4 — the `mvp` bundle carries the metadata block naming
//     `owning_spec`, `introduced_in_train`, and `introduced_at`.
//
// It also pins the corrected premise. Scope 05 was originally planned on the
// assertion that release-train policy "requires default-ON in exactly one
// owning train". That premise is false: `release-train-guard.sh` Check 8 skips
// the owning train before reaching the G111 check, so G111 can only reject
// default-ON on a NON-owning train — it never requires ON anywhere. An all-OFF
// dormant flag is conformant. Adversarial case (c) below is what makes that
// concrete: a rule demanding ON somewhere would fail it.
//
// The invariant is a pure function over parsed bundle bytes so the adversarial
// fixtures are string literals rather than mutations of real config on disk.
// This mirrors internal/deploy/eval_lane_contract_test.go.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	corpusFlagName          = "corpusGrantEnforcement"
	corpusFlagOwningTrain   = "next"
	corpusFlagMetadataTrain = "mvp"
	corpusFlagOwningSpec    = "specs/108-corpus-grant-enforcement/"

	corpusFlagBundleGlob = "config/feature-flags.*.yaml"
	corpusFlagBundleFmt  = "config/feature-flags.%s.yaml"
)

// corpusFlagBundleDoc is the minimal bundle shape this contract needs.
// Flags are decoded as yaml.Node rather than bool so a non-boolean value
// (`"false"`, `1`, `yes`) is reported as a vocabulary violation instead of
// aborting the whole unmarshal with an opaque type error.
type corpusFlagBundleDoc struct {
	Version  int                       `yaml:"version"`
	Train    string                    `yaml:"train"`
	Flags    map[string]yaml.Node      `yaml:"flags"`
	Metadata map[string]corpusFlagMeta `yaml:"metadata"`
}

type corpusFlagMeta struct {
	OwningSpec        string `yaml:"owning_spec"`
	IntroducedInTrain string `yaml:"introduced_in_train"`
	IntroducedAt      string `yaml:"introduced_at"`
}

// assertFlagBundleContract is the pure invariant. `bundles` maps a train id to
// that train's raw bundle YAML. Pure so adversarial fixtures need no
// filesystem and the real config is never mutated.
func assertFlagBundleContract(bundles map[string][]byte) error {
	if len(bundles) == 0 {
		return fmt.Errorf("contract violation: no train bundles supplied, so %q cannot be shown to be declared anywhere", corpusFlagName)
	}
	for _, required := range []string{corpusFlagOwningTrain, corpusFlagMetadataTrain} {
		if _, ok := bundles[required]; !ok {
			return fmt.Errorf("contract violation: bundle for train %q is absent from the supplied set (R-108-FL2 requires %q declared in every train bundle)", required, corpusFlagName)
		}
	}

	trains := make([]string, 0, len(bundles))
	for train := range bundles {
		trains = append(trains, train)
	}
	sort.Strings(trains)

	for _, train := range trains {
		var doc corpusFlagBundleDoc
		if err := yaml.Unmarshal(bundles[train], &doc); err != nil {
			return fmt.Errorf("contract violation: train %q bundle is not valid YAML: %w", train, err)
		}
		if doc.Version != 1 {
			return fmt.Errorf("contract violation: train %q bundle has version=%d (expected 1)", train, doc.Version)
		}
		// Without this the (b) fixture could be mislabelled: a file keyed
		// "mvp" that actually declares `train: next` would be judged against
		// the owning-train rule and the G111 case would never be exercised.
		if doc.Train != train {
			return fmt.Errorf("contract violation: bundle supplied for train %q declares train=%q; the bundle's own train id must match", train, doc.Train)
		}

		node, declared := doc.Flags[corpusFlagName]
		if !declared {
			return fmt.Errorf("contract violation (R-108-FL2): train %q does not declare flag %q; a flag dropped from one bundle leaves that train resolving nothing", train, corpusFlagName)
		}
		if node.Tag != "!!bool" {
			return fmt.Errorf("contract violation: train %q declares %q as %s (%q); the flag vocabulary is the two booleans true|false", train, corpusFlagName, node.Tag, node.Value)
		}
		if node.Value != "false" {
			if train != corpusFlagOwningTrain {
				return fmt.Errorf("contract violation (G111): train %q is NOT the owning train %q yet declares %q default-ON; release-train-guard.sh Check 8 skips the owning train and raises G111 for exactly this case", train, corpusFlagOwningTrain, corpusFlagName)
			}
			return fmt.Errorf("contract violation (R-108-FL3): owning train %q declares %q default-ON, but the flag ships default-OFF in EVERY train; a default-ON owning train arrives already enforcing and destroys the OBSERVE window. bubbles.train owns the later flip", train, corpusFlagName)
		}
	}

	var meta corpusFlagBundleDoc
	if err := yaml.Unmarshal(bundles[corpusFlagMetadataTrain], &meta); err != nil {
		return fmt.Errorf("contract violation: train %q bundle is not valid YAML: %w", corpusFlagMetadataTrain, err)
	}
	entry, ok := meta.Metadata[corpusFlagName]
	if !ok {
		return fmt.Errorf("contract violation (R-108-FL4): train %q carries no metadata block for %q; the flag's owning spec, owning train, and introduction date would be unrecorded", corpusFlagMetadataTrain, corpusFlagName)
	}
	if entry.OwningSpec != corpusFlagOwningSpec {
		return fmt.Errorf("contract violation (R-108-FL4): %q metadata owning_spec=%q, want %q", corpusFlagName, entry.OwningSpec, corpusFlagOwningSpec)
	}
	if entry.IntroducedInTrain != corpusFlagOwningTrain {
		return fmt.Errorf("contract violation (R-108-FL4): %q metadata introduced_in_train=%q, want %q", corpusFlagName, entry.IntroducedInTrain, corpusFlagOwningTrain)
	}
	if strings.TrimSpace(entry.IntroducedAt) == "" {
		return fmt.Errorf("contract violation (R-108-FL4): %q metadata introduced_at is empty; the flag-lifecycle clock (train + one cycle) has no start date", corpusFlagName)
	}
	return nil
}

// corpusFlagBaselineFixture is the minimal conformant pair: the shipped
// all-OFF shape reduced to literals. Adversarial cases mutate it.
func corpusFlagBaselineFixture() map[string][]byte {
	return map[string][]byte{
		"mvp": []byte(`version: 1
train: mvp
flags:
  corpusGrantEnforcement: false
metadata:
  corpusGrantEnforcement:
    owning_spec: specs/108-corpus-grant-enforcement/
    introduced_in_train: next
    introduced_at: "2026-08-11"
`),
		"next": []byte(`version: 1
train: next
flags:
  corpusGrantEnforcement: false
metadata: {}
`),
	}
}

// mutateCorpusFlagFixture applies exactly one mutation and refuses a no-op, so
// a stale literal cannot silently produce an unmutated fixture that "passes".
func mutateCorpusFlagFixture(t *testing.T, bundles map[string][]byte, train, old, replacement string) map[string][]byte {
	t.Helper()
	source, ok := bundles[train]
	if !ok {
		t.Fatalf("adversarial fixture is stale: no bundle for train %q", train)
	}
	if !strings.Contains(string(source), old) {
		t.Fatalf("adversarial fixture is stale: %q is not present in the %q bundle under mutation", old, train)
	}
	mutated := strings.Replace(string(source), old, replacement, 1)
	if mutated == string(source) {
		t.Fatalf("adversarial mutation of %q was a no-op; the fixture would be identical to the baseline", old)
	}
	out := make(map[string][]byte, len(bundles))
	for k, v := range bundles {
		out[k] = v
	}
	out[train] = []byte(mutated)
	return out
}

// requireCorpusFlagBaselinePasses is the anti-tautology precondition. Without
// it an adversarial case could pass because the baseline was already broken
// for an unrelated reason, which is how a regression test becomes decorative.
func requireCorpusFlagBaselinePasses(t *testing.T, bundles map[string][]byte) {
	t.Helper()
	if err := assertFlagBundleContract(bundles); err != nil {
		t.Fatalf("adversarial precondition failed: the unmutated baseline must satisfy the contract, got: %v", err)
	}
}

// corpusFlagLiveBundles reads every committed bundle off disk. It globs rather
// than naming two files so "declared in EVERY train bundle" stays true when a
// third train is cut; a named-file test would pass while a new bundle omitted
// the flag entirely.
func corpusFlagLiveBundles(t *testing.T) map[string][]byte {
	t.Helper()
	root := repoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(corpusFlagBundleGlob)))
	if err != nil {
		t.Fatalf("glob %s: %v", corpusFlagBundleGlob, err)
	}
	bundles := make(map[string][]byte, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		train := strings.TrimSuffix(strings.TrimPrefix(base, "feature-flags."), ".yaml")
		data, err := os.ReadFile(path) // #nosec G304 -- path comes from a fixed in-repo glob
		if err != nil {
			t.Fatalf("read %s: %v", base, err)
		}
		bundles[train] = data
	}
	if len(bundles) < 2 {
		t.Fatalf("expected at least the %q and %q bundles on disk, found %d matching %s",
			corpusFlagMetadataTrain, corpusFlagOwningTrain, len(bundles), corpusFlagBundleGlob)
	}
	for _, train := range []string{corpusFlagMetadataTrain, corpusFlagOwningTrain} {
		if _, ok := bundles[train]; !ok {
			t.Fatalf("committed bundle %s is missing", fmt.Sprintf(corpusFlagBundleFmt, train))
		}
	}
	return bundles
}

// TestCorpusGrantFlagBundle_LiveBundles reads the real committed bundles. No
// fixture, no mock: this is the assertion about what the repo actually ships.
func TestCorpusGrantFlagBundle_LiveBundles(t *testing.T) {
	if err := assertFlagBundleContract(corpusFlagLiveBundles(t)); err != nil {
		t.Fatal(err)
	}
}

// TestCorpusGrantFlagBundle_AcceptsShippedAllOffShape is adversarial case (c).
// It is the case that pins the corrected premise: under the withdrawn
// "default-ON in exactly one owning train" rule this fixture would be
// REJECTED, so a test lacking it would still pass under the false rule.
func TestCorpusGrantFlagBundle_AcceptsShippedAllOffShape(t *testing.T) {
	if err := assertFlagBundleContract(corpusFlagBaselineFixture()); err != nil {
		t.Fatalf("contract refused the shipped all-OFF shape (next=false, mvp=false): %v\nAn all-OFF dormant flag is conformant: release-train-guard.sh Check 8 skips the owning train before the G111 check, so no rule requires default-ON anywhere", err)
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsAbsentFlag is case (a). Without
// it the contract still passes if the flag is silently deleted from a bundle,
// because every remaining assertion is about bundles that still declare it.
func TestCorpusGrantFlagBundle_AdversarialRejectsAbsentFlag(t *testing.T) {
	for _, train := range []string{corpusFlagMetadataTrain, corpusFlagOwningTrain} {
		t.Run(train+"_flag_deleted", func(t *testing.T) {
			baseline := corpusFlagBaselineFixture()
			requireCorpusFlagBaselinePasses(t, baseline)

			broken := mutateCorpusFlagFixture(t, baseline, train,
				"  corpusGrantEnforcement: false\n", "")
			if err := assertFlagBundleContract(broken); err == nil {
				t.Fatalf("contract accepted a %q bundle with %q absent; R-108-FL2 requires it declared in EVERY train bundle", train, corpusFlagName)
			} else if !strings.Contains(err.Error(), "R-108-FL2") {
				t.Fatalf("rejected for the wrong reason, want an R-108-FL2 declaration failure, got: %v", err)
			}
		})
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsNonOwningTrainDefaultOn is case
// (b) — precisely the G111 condition. `mvp` does not own the flag, so a
// default-ON there is the one shape release-train-guard.sh Check 8 rejects.
func TestCorpusGrantFlagBundle_AdversarialRejectsNonOwningTrainDefaultOn(t *testing.T) {
	baseline := corpusFlagBaselineFixture()
	requireCorpusFlagBaselinePasses(t, baseline)

	broken := mutateCorpusFlagFixture(t, baseline, corpusFlagMetadataTrain,
		"corpusGrantEnforcement: false", "corpusGrantEnforcement: true")
	err := assertFlagBundleContract(broken)
	if err == nil {
		t.Fatalf("contract accepted %q (a NON-owning train) with %q default-ON; this is the G111 violation condition", corpusFlagMetadataTrain, corpusFlagName)
	}
	if !strings.Contains(err.Error(), "G111") {
		t.Fatalf("rejected for the wrong reason, want the G111 non-owning-train message, got: %v", err)
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsOwningTrainDefaultOn pins the
// time-bound half of R-108-FL3: the state this spec SHIPS is default-OFF in
// every train, owning train included. (TP-05-06 is the permanent regression
// and deliberately does not pin `next`, because bubbles.train flipping the
// owning train ON after a clean observation window is the intended end state.)
func TestCorpusGrantFlagBundle_AdversarialRejectsOwningTrainDefaultOn(t *testing.T) {
	baseline := corpusFlagBaselineFixture()
	requireCorpusFlagBaselinePasses(t, baseline)

	broken := mutateCorpusFlagFixture(t, baseline, corpusFlagOwningTrain,
		"corpusGrantEnforcement: false", "corpusGrantEnforcement: true")
	err := assertFlagBundleContract(broken)
	if err == nil {
		t.Fatalf("contract accepted owning train %q with %q default-ON; R-108-FL3 ships the flag default-OFF in EVERY train", corpusFlagOwningTrain, corpusFlagName)
	}
	if !strings.Contains(err.Error(), "R-108-FL3") {
		t.Fatalf("rejected for the wrong reason, want an R-108-FL3 default-OFF failure, got: %v", err)
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsMissingMetadata makes R-108-FL4
// falsifiable. Asserting the metadata values without proving their absence is
// caught would leave the metadata tick resting on an unfalsifiable check.
func TestCorpusGrantFlagBundle_AdversarialRejectsMissingMetadata(t *testing.T) {
	cases := []struct {
		name    string
		old     string
		replace string
	}{
		{"block_deleted", "  corpusGrantEnforcement:\n    owning_spec: specs/108-corpus-grant-enforcement/\n    introduced_in_train: next\n    introduced_at: \"2026-08-11\"\n", ""},
		{"wrong_owning_spec", "owning_spec: specs/108-corpus-grant-enforcement/", "owning_spec: specs/999-unrelated/"},
		{"wrong_introduced_in_train", "introduced_in_train: next", "introduced_in_train: mvp"},
		{"empty_introduced_at", "introduced_at: \"2026-08-11\"", "introduced_at: \"\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			baseline := corpusFlagBaselineFixture()
			requireCorpusFlagBaselinePasses(t, baseline)

			broken := mutateCorpusFlagFixture(t, baseline, corpusFlagMetadataTrain, tc.old, tc.replace)
			err := assertFlagBundleContract(broken)
			if err == nil {
				t.Fatalf("contract accepted a %q bundle whose %q metadata was mutated (%s); R-108-FL4 requires owning_spec, introduced_in_train, and introduced_at", corpusFlagMetadataTrain, corpusFlagName, tc.name)
			}
			if !strings.Contains(err.Error(), "R-108-FL4") {
				t.Fatalf("rejected for the wrong reason, want an R-108-FL4 metadata failure, got: %v", err)
			}
		})
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsNonBooleanValue guards the closed
// two-value vocabulary. A quoted "false" reads as OFF to a human but is a
// string to every consumer, so it must not be accepted as default-OFF.
func TestCorpusGrantFlagBundle_AdversarialRejectsNonBooleanValue(t *testing.T) {
	baseline := corpusFlagBaselineFixture()
	requireCorpusFlagBaselinePasses(t, baseline)

	broken := mutateCorpusFlagFixture(t, baseline, corpusFlagOwningTrain,
		"corpusGrantEnforcement: false", `corpusGrantEnforcement: "false"`)
	if err := assertFlagBundleContract(broken); err == nil {
		t.Fatalf("contract accepted a quoted string value for %q; the flag vocabulary is the two booleans true|false", corpusFlagName)
	}
}

// TestCorpusGrantFlagBundle_AdversarialRejectsMissingBundle proves the
// "declared in EVERY train bundle" claim is not satisfied by omission: dropping
// a whole bundle from the set must fail, not silently reduce the assertion.
func TestCorpusGrantFlagBundle_AdversarialRejectsMissingBundle(t *testing.T) {
	for _, train := range []string{corpusFlagMetadataTrain, corpusFlagOwningTrain} {
		t.Run(train+"_bundle_absent", func(t *testing.T) {
			baseline := corpusFlagBaselineFixture()
			requireCorpusFlagBaselinePasses(t, baseline)

			delete(baseline, train)
			if err := assertFlagBundleContract(baseline); err == nil {
				t.Fatalf("contract accepted a bundle set with train %q entirely absent", train)
			}
		})
	}
}
