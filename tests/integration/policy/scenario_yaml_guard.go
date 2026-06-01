//go:build integration || e2e

// Spec 067 Scope 2 — Scenario YAML policy guards.
//
// Shared scanner for scenario YAML files under
// config/prompt_contracts/. Emits Violations for:
//
//   G067-A01  principle alignment        — missing principleAlignment
//                                          block or unknown principle ID.
//   G067-A02  prompt cap                  — non-blank system_prompt
//                                          lines > cfg.ScenarioPromptMaxLines.
//
// The scanner also returns the policyExceptions declared inside each
// YAML so the Scope 1 baseline ratchet (G067-A07) can account for
// them. Required-keys are SST-sourced; thresholds come from
// PolicyConfig — no magic constants live here.
//
// Valid principle IDs are sourced from docs/Product-Principles.md
// at parse time. Both `Principle N` and `PN` short forms are
// accepted. Missing principles file is a bootstrap error (no silent
// pass).

package policy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	scenarioPolicySource    = "specs/067-intent-driven-policy-enforcement/spec.md"
	productPrinciplesSource = "docs/Product-Principles.md"
)

// scenarioYAML is the structural projection of a scenario YAML
// needed by the spec 067 guards. Unknown fields are ignored so the
// scanner is robust against scenario-specific extensions.
type scenarioYAML struct {
	ID                 string      `yaml:"id"`
	Version            string      `yaml:"version"`
	SystemPrompt       string      `yaml:"system_prompt"`
	PrincipleAlignment []string    `yaml:"principleAlignment"`
	PolicyExceptions   []yamlExcep `yaml:"policyExceptions"`
}

type yamlExcep struct {
	ID        string `yaml:"id"`
	Rule      string `yaml:"rule"`
	Owner     string `yaml:"owner"`
	Reason    string `yaml:"reason"`
	ExpiresOn string `yaml:"expires_on"`
}

// ScenarioFile is the parsed projection a guard test asserts on.
type ScenarioFile struct {
	Path              string
	ID                string
	HasPrincipleBlock bool
	PrincipleIDs      []string
	PromptLineCount   int
	Exceptions        []Exception
}

// LoadProductPrincipleIDs reads docs/Product-Principles.md and
// returns the set of valid principle IDs. Both the long form
// (`Principle N`) and the short form (`PN`) are produced for each
// numbered principle heading. Missing/empty file is a bootstrap
// error.
func LoadProductPrincipleIDs(path string) (map[string]struct{}, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("product-principles: read %s: %w", path, err)
	}
	re := regexp.MustCompile(`(?m)^## Principle (\d+)`)
	matches := re.FindAllStringSubmatch(string(raw), -1)
	if len(matches) == 0 {
		return nil, fmt.Errorf("product-principles: no `## Principle N` headings found in %s", path)
	}
	ids := map[string]struct{}{}
	for _, m := range matches {
		ids["Principle "+m[1]] = struct{}{}
		ids["P"+m[1]] = struct{}{}
	}
	return ids, nil
}

// ParseScenarioYAML reads and structurally parses one scenario YAML.
// The returned ScenarioFile is populated even when parsing partially
// succeeds; ParseErr is non-nil on hard parse failure.
func ParseScenarioYAML(path string) (ScenarioFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ScenarioFile{Path: path}, fmt.Errorf("scenario-yaml: read %s: %w", path, err)
	}
	var doc scenarioYAML
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return ScenarioFile{Path: path}, fmt.Errorf("scenario-yaml: parse %s: %w", path, err)
	}
	sf := ScenarioFile{
		Path:              path,
		ID:                doc.ID,
		HasPrincipleBlock: doc.PrincipleAlignment != nil,
		PrincipleIDs:      append([]string(nil), doc.PrincipleAlignment...),
		PromptLineCount:   countNonBlankLines(doc.SystemPrompt),
	}
	for _, e := range doc.PolicyExceptions {
		sf.Exceptions = append(sf.Exceptions, Exception{
			ID:        e.ID,
			RuleID:    e.Rule,
			Path:      path,
			Owner:     e.Owner,
			Reason:    e.Reason,
			ExpiresOn: e.ExpiresOn,
		})
	}
	return sf, nil
}

func countNonBlankLines(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// ListScenarioYAMLs returns the sorted set of `*.yaml` files under
// repo/config/prompt_contracts/.
func ListScenarioYAMLs(repo Root) ([]string, error) {
	root := filepath.Join(string(repo), "config", "prompt_contracts")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("scenario-yaml: read dir %s: %w", root, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		out = append(out, filepath.Join(root, name))
	}
	sort.Strings(out)
	return out, nil
}

// PrincipleAlignmentGuard scans the given scenario files and
// returns violations for missing principleAlignment blocks
// (G067-A01) and for declared IDs that are not present in the
// product-principles catalog.
func PrincipleAlignmentGuard(files []ScenarioFile, validIDs map[string]struct{}) []Violation {
	var out []Violation
	for _, sf := range files {
		if !sf.HasPrincipleBlock {
			out = append(out, Violation{
				RuleID:       "G067-A01",
				RuleName:     "scenario YAML missing principleAlignment block",
				Path:         relTo(sf.Path),
				Detail:       fmt.Sprintf("scenario %q has no principleAlignment block; principles catalog is %s", sf.ID, productPrinciplesSource),
				PolicySource: scenarioPolicySource,
				Owner:        "scenario-author",
				Resolution:   "add a principleAlignment block listing one or more IDs from " + productPrinciplesSource,
			})
			continue
		}
		if len(sf.PrincipleIDs) == 0 {
			out = append(out, Violation{
				RuleID:       "G067-A01",
				RuleName:     "scenario YAML principleAlignment block is empty",
				Path:         relTo(sf.Path),
				Detail:       fmt.Sprintf("scenario %q principleAlignment block is empty", sf.ID),
				PolicySource: scenarioPolicySource,
				Owner:        "scenario-author",
				Resolution:   "list at least one principle ID from " + productPrinciplesSource,
			})
			continue
		}
		for _, id := range sf.PrincipleIDs {
			if _, ok := validIDs[id]; !ok {
				out = append(out, Violation{
					RuleID:       "G067-A01",
					RuleName:     "scenario YAML principleAlignment cites unknown principle",
					Path:         relTo(sf.Path),
					Detail:       fmt.Sprintf("scenario %q cites principle %q which is not present in %s", sf.ID, id, productPrinciplesSource),
					PolicySource: scenarioPolicySource,
					Owner:        "scenario-author",
					Resolution:   "correct the principle ID to one declared in " + productPrinciplesSource,
				})
			}
		}
	}
	return out
}

// ScenarioPromptCapGuard returns G067-A02 violations for every
// scenario whose non-blank system_prompt line count exceeds
// cfg.ScenarioPromptMaxLines. The cap value is included in both the
// Detail and the Resolution so CI output names the configured
// threshold without consumers having to look it up.
func ScenarioPromptCapGuard(files []ScenarioFile, cfg PolicyConfig) []Violation {
	if cfg.ScenarioPromptMaxLines <= 0 {
		// Defensive: the SST loader already fails loud, but guard
		// against an injected zero from a test that forgot to set
		// the cap.
		return nil
	}
	var out []Violation
	for _, sf := range files {
		if sf.PromptLineCount > cfg.ScenarioPromptMaxLines {
			out = append(out, Violation{
				RuleID:       "G067-A02",
				RuleName:     "scenario system_prompt exceeds policy.scenario_prompt_max_lines",
				Path:         relTo(sf.Path),
				Detail:       fmt.Sprintf("scenario %q system_prompt has %d non-blank lines; cap is %d (policy.scenario_prompt_max_lines)", sf.ID, sf.PromptLineCount, cfg.ScenarioPromptMaxLines),
				PolicySource: scenarioPolicySource,
				Owner:        "scenario-author",
				Resolution:   fmt.Sprintf("shrink the system_prompt to at most %d non-blank lines or raise policy.scenario_prompt_max_lines via SST", cfg.ScenarioPromptMaxLines),
			})
		}
	}
	return out
}

// relTo returns p with the repo root prefix trimmed when present so
// guard output is portable across checkouts. If the path does not
// contain "config/prompt_contracts", it is returned unchanged.
func relTo(p string) string {
	idx := strings.Index(p, "config/prompt_contracts")
	if idx < 0 {
		return p
	}
	return p[idx:]
}
