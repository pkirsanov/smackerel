// Spec 108 SCOPE-05 — flag, SST, release-packet and retirement contracts.
//
// TP-05-01 (flag-bundle parity), TP-05-02 (SST default-freedom),
// TP-05-05 (release-packet entry), TP-05-07 (retirement contract).
//
// These assert against the LIVE files, not fixtures, so they fail when the
// shipped configuration drifts rather than when a copy of it drifts. The
// adversarial cases in TP-05-01 additionally run the SAME validator over
// hand-built fixtures, because an assertion that only ever sees the correct
// shape cannot prove it would reject the wrong one.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const corpusGrantFlagName = "corpusGrantEnforcement"

type corpusFlagBundle struct {
	Flags    map[string]bool                   `yaml:"flags"`
	Metadata map[string]map[string]interface{} `yaml:"metadata"`
}

// assertCorpusFlagBundles is the validator under test. It is a free function
// so the adversarial fixtures below can drive the SAME logic that judges the
// live bundles — otherwise the negative cases would prove nothing about the
// assertion actually protecting the repo.
//
// bundles maps train name -> parsed bundle. metadataTrain names the train
// whose bundle must carry the metadata block.
func assertCorpusFlagBundles(bundles map[string]corpusFlagBundle, metadataTrain string) error {
	if len(bundles) == 0 {
		return fmt.Errorf("no bundles supplied")
	}

	// R-108-FL2 — declared in EVERY train bundle. A flag missing from a
	// bundle is not "off"; it is undefined, and the resolution path is
	// fail-loud, so a dropped declaration breaks that train's boot rather
	// than quietly defaulting.
	for train, b := range bundles {
		if _, declared := b.Flags[corpusGrantFlagName]; !declared {
			return fmt.Errorf("R-108-FL2 violation: %q is not declared in the %q bundle — a flag absent from a bundle is undefined, not off", corpusGrantFlagName, train)
		}
	}

	// R-108-FL3 — default-OFF in EVERY train, including the owning one.
	// The OBSERVE window cannot run if any train ships ENFORCE by default.
	for train, b := range bundles {
		if b.Flags[corpusGrantFlagName] {
			return fmt.Errorf("R-108-FL3 violation: %q is default-ON in train %q — every train must ship OFF so the OBSERVE window can run before anyone is denied", corpusGrantFlagName, train)
		}
	}

	// R-108-FL4 — the metadata block records provenance.
	meta, ok := bundles[metadataTrain].Metadata[corpusGrantFlagName]
	if !ok {
		return fmt.Errorf("R-108-FL4 violation: the %q bundle has no metadata block for %q — provenance (owning spec, introducing train, date) would be unrecoverable", metadataTrain, corpusGrantFlagName)
	}
	for _, key := range []string{"owning_spec", "introduced_in_train", "introduced_at"} {
		if v, present := meta[key]; !present || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
			return fmt.Errorf("R-108-FL4 violation: metadata for %q is missing %q", corpusGrantFlagName, key)
		}
	}
	if got := fmt.Sprintf("%v", meta["owning_spec"]); !strings.Contains(got, "108-corpus-grant-enforcement") {
		return fmt.Errorf("R-108-FL4 violation: owning_spec=%q does not name spec 108", got)
	}
	if got := fmt.Sprintf("%v", meta["introduced_in_train"]); got != "next" {
		return fmt.Errorf("R-108-FL4 violation: introduced_in_train=%q, want \"next\"", got)
	}
	return nil
}

func loadCorpusFlagBundle(t *testing.T, path string) corpusFlagBundle {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var b corpusFlagBundle
	if err := yaml.Unmarshal(raw, &b); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return b
}

// TestCorpusGrantFlag_BundleParity_TP_05_01 asserts the live bundles, then
// proves the assertion is falsifiable against three fixtures.
func TestCorpusGrantFlag_BundleParity_TP_05_01(t *testing.T) {
	root := repoRoot(t)
	live := map[string]corpusFlagBundle{
		"next": loadCorpusFlagBundle(t, filepath.Join(root, "config", "feature-flags.next.yaml")),
		"mvp":  loadCorpusFlagBundle(t, filepath.Join(root, "config", "feature-flags.mvp.yaml")),
	}

	t.Run("live_bundles_satisfy_the_contract", func(t *testing.T) {
		if err := assertCorpusFlagBundles(live, "mvp"); err != nil {
			t.Fatalf("live feature-flag bundles violate the spec 108 flag contract: %v", err)
		}
	})

	// (a) deletion — the flag silently dropped from a bundle.
	t.Run("adversarial_deletion_is_rejected", func(t *testing.T) {
		fixture := map[string]corpusFlagBundle{
			"next": {Flags: map[string]bool{corpusGrantFlagName: false}},
			"mvp":  {Flags: map[string]bool{}, Metadata: live["mvp"].Metadata},
		}
		if err := assertCorpusFlagBundles(fixture, "mvp"); err == nil {
			t.Fatal("a bundle with the flag ABSENT was accepted; the test cannot detect a silently dropped declaration")
		}
	})

	// (b) non-owning train ON — the G111 condition in release-train-guard.sh.
	t.Run("adversarial_non_owning_train_ON_is_rejected", func(t *testing.T) {
		fixture := map[string]corpusFlagBundle{
			"next": {Flags: map[string]bool{corpusGrantFlagName: false}},
			"mvp":  {Flags: map[string]bool{corpusGrantFlagName: true}, Metadata: live["mvp"].Metadata},
		}
		if err := assertCorpusFlagBundles(fixture, "mvp"); err == nil {
			t.Fatal("a NON-OWNING train with the flag default-ON was accepted; this is exactly the G111 violation condition")
		}
	})

	// (c) all-OFF accepted — pins the corrected premise (SCN-108-R01). A rule
	// demanding the flag be ON in its owning train would fail here, which is
	// the point: the shipped shape is OFF everywhere until the flip.
	t.Run("adversarial_all_OFF_is_accepted", func(t *testing.T) {
		fixture := map[string]corpusFlagBundle{
			"next": {Flags: map[string]bool{corpusGrantFlagName: false}},
			"mvp":  {Flags: map[string]bool{corpusGrantFlagName: false}, Metadata: live["mvp"].Metadata},
		}
		if err := assertCorpusFlagBundles(fixture, "mvp"); err != nil {
			t.Fatalf("the SHIPPED all-OFF shape was rejected (%v); a rule demanding ON somewhere would break the OBSERVE window", err)
		}
	})
}

// TestCorpusGrantFlag_SSTHasNoDefault_TP_05_02 asserts the SST key exists and
// that nothing in its resolution path supplies a fallback.
//
// A default here would be silent ENFORCE→OBSERVE drift (or the reverse) with
// no operator signal, which is precisely what smackerel-no-defaults forbids.
func TestCorpusGrantFlag_SSTHasNoDefault_TP_05_02(t *testing.T) {
	root := repoRoot(t)

	sstPath := filepath.Join(root, "config", "smackerel.yaml")
	sst, err := os.ReadFile(sstPath)
	if err != nil {
		t.Fatalf("read %s: %v", sstPath, err)
	}
	if !strings.Contains(string(sst), "corpus_grant_enforcement:") {
		t.Fatalf("%s does not declare auth.corpus_grant_enforcement — the resolution path is fail-loud, so an absent key breaks boot", sstPath)
	}

	genPath := filepath.Join(root, "scripts", "commands", "config.sh")
	gen, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read %s: %v", genPath, err)
	}
	genSrc := string(gen)

	// The generator must READ the key fail-loud...
	if !strings.Contains(genSrc, `required_value auth.corpus_grant_enforcement`) {
		t.Error("config.sh does not resolve auth.corpus_grant_enforcement via required_value; a missing key must abort generation, not resolve to a default")
	}

	// ...and EMIT it fail-loud. `${VAR:?msg}` aborts on empty; `${VAR:-x}`
	// would substitute a value nobody chose.
	if !regexp.MustCompile(`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT=\$\{SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:\?`).MatchString(genSrc) {
		t.Error("config.sh does not emit SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT with a fail-loud ${VAR:?...} guard")
	}

	// No fallback shape anywhere on this variable, in either direction.
	for _, bad := range []string{
		`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT:-`,
		`SMACKEREL_AUTH_CORPUS_GRANT_ENFORCEMENT-`,
	} {
		if strings.Contains(genSrc, bad) {
			t.Errorf("config.sh contains the fallback shape %q; a default silently decides the enforcement stage for the operator", bad)
		}
	}
}

// TestCorpusGrantFlag_ReleasePacketRecordsCapability_TP_05_05 asserts the v1
// packet records the shipped capability with its owning spec, train and flag.
//
// Read-only: docs/releases/** is bubbles.releases-owned. This test reports
// drift; it never edits the packet.
func TestCorpusGrantFlag_ReleasePacketRecordsCapability_TP_05_05(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "releases", "v1", "features.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(raw)

	for _, want := range []struct{ token, why string }{
		{"108-corpus-grant-enforcement", "the owning spec, so a reader can find the contract behind the capability"},
		{"corpusGrantEnforcement", "the flag, so the capability can be traced to its rollout control"},
		{"next", "the owning train"},
	} {
		if !strings.Contains(body, want.token) {
			t.Errorf("docs/releases/v1/features.md does not record %q — %s", want.token, want.why)
		}
	}
}

// TestCorpusGrantFlag_RetirementContractRecorded_TP_05_07 asserts all FOUR
// retirement clauses of §18 decision 6 are written down.
//
// An obligation that is implied rather than recorded decays: the flag outlives
// its train, the observe branch becomes permanent, and the counters accumulate
// cardinality nobody reads. Each clause is asserted separately so a partial
// deletion cannot pass.
func TestCorpusGrantFlag_RetirementContractRecorded_TP_05_07(t *testing.T) {
	root := repoRoot(t)
	candidates := []string{
		filepath.Join(root, "docs", "Operations.md"),
		filepath.Join(root, "specs", "108-corpus-grant-enforcement", "design.md"),
		filepath.Join(root, "specs", "108-corpus-grant-enforcement", "spec.md"),
	}

	var corpus strings.Builder
	for _, p := range candidates {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		corpus.Write(raw)
		corpus.WriteString("\n")
	}
	// Markdown emphasis sits BETWEEN words in these docs ("deleted
	// **together**"), so matching on raw text produces false negatives that
	// would train a reader to ignore this gate. Strip emphasis and collapse
	// whitespace so the assertion is about the prose, not its formatting.
	body := strings.ToLower(corpus.String())
	body = strings.NewReplacer("*", "", "_", "", "`", "").Replace(body)
	body = regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")

	clauses := []struct {
		name    string
		pattern *regexp.Regexp
		why     string
	}{
		{
			"train + one cycle",
			regexp.MustCompile(`train\s*\+\s*(one|1)\s*cycle`),
			"without a stated lifetime the flag silently becomes permanent",
		},
		{
			"flag + observe branch + counters retire together",
			regexp.MustCompile(`(retire|removed|deleted)\s+together|together[^.]{0,80}(observe branch|would-deny)`),
			"retiring the flag but leaving the observe branch keeps dead code and a counter nobody reads",
		},
		{
			"enforcement becomes unconditional",
			regexp.MustCompile(`unconditional`),
			"the end state must be stated, or removing the flag looks like removing the protection",
		},
		{
			"bubbles.train owns flip and retirement",
			regexp.MustCompile(`bubbles\.train`),
			"an unowned retirement is nobody's job",
		},
	}

	for _, c := range clauses {
		if !c.pattern.MatchString(body) {
			t.Errorf("retirement clause %q is not recorded in Operations.md / design.md / spec.md — %s", c.name, c.why)
		}
	}
}
