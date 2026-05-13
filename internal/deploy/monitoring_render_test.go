// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 049 — Monitoring Stack Prometheus render contract
// (T-049-002).
//
// This test simulates the render step performed by
// scripts/commands/config.sh: it loads the committed template at
// `config/prometheus/prometheus.yml.tmpl`, substitutes the known env
// vars with synthetic values, parses the result as YAML, and asserts
// the rendered document is structurally valid and contains the
// substituted values at the expected paths.
//
// FR-049-001 + FR-049-005(b): "Config-generation MUST render a valid
// Prometheus scrape file from the SST without leaving any
// unsubstituted placeholders or producing invalid YAML."
//
// The Go test simulates envsubst behaviour by using
// strings.NewReplacer over the documented env-var allowlist. This is
// intentional: it is a contract test, not an integration test of
// envsubst itself. The actual `./smackerel.sh config generate`
// invocation in CI gives end-to-end coverage; this static test runs
// in milliseconds and catches template bugs at unit-test time.
//
// Adversarial sub-test proves the contract function would FAIL if the
// template were corrupted so substitution leaves invalid YAML.
//
// Cross-reference:
//   - specs/049-monitoring-stack/spec.md FR-049-001 / FR-049-005(b)
//   - config/prometheus/prometheus.yml.tmpl
//   - scripts/commands/config.sh (envsubst step)
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// canonicalRenderValues are the synthetic env values used by the
// render test. They are intentionally distinct, non-default integers
// so the assertions can prove substitution actually happened (vs the
// document accidentally already containing the right numbers).
var canonicalRenderValues = map[string]string{
	"PROMETHEUS_SCRAPE_INTERVAL_S":     "23",
	"PROMETHEUS_EVALUATION_INTERVAL_S": "29",
	"CORE_CONTAINER_PORT":              "18080",
	"ML_CONTAINER_PORT":                "18081",
}

// renderUnsubstitutedRE catches any `${VAR}` placeholder left in the
// rendered output. Same regex shape as scrapeVarRE but kept local so
// the two test files do not couple.
var renderUnsubstitutedRE = regexp.MustCompile(`\$\{[A-Z_][A-Z0-9_]*\}`)

// renderTemplate performs simple ${VAR} → value substitution over the
// documented allowlist. Anything not in the allowlist is left intact
// (envsubst behaviour when called WITHOUT a variable allowlist would
// substitute every `${VAR}` it knows; here we mirror the more
// restrictive `envsubst '$VAR1 $VAR2 ...'` form that
// scripts/commands/config.sh uses).
func renderTemplate(t *testing.T, raw string, values map[string]string) string {
	t.Helper()
	pairs := make([]string, 0, len(values)*2)
	for k, v := range values {
		pairs = append(pairs, "${"+k+"}", v)
	}
	return strings.NewReplacer(pairs...).Replace(raw)
}

// assertRenderContract returns nil iff the rendered text is valid
// YAML, contains no unsubstituted `${VAR}` placeholders from the
// allowlist, and contains the substituted values at the documented
// paths.
func assertRenderContract(rendered string, values map[string]string) error {
	// Check 1 — no leftover placeholders from the substitution set.
	for _, match := range renderUnsubstitutedRE.FindAllString(rendered, -1) {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		if _, ok := values[varName]; ok {
			return fmt.Errorf("contract violation: rendered output contains unsubstituted placeholder %s — envsubst was expected to replace it with the provided value, so this likely means the template uses a name not in the documented allowlist or the substitution step is broken", match)
		}
		// A placeholder NOT in `values` is a stray template variable
		// that the scrape contract test already catches (T-049-001);
		// flag it here too with the same precision.
		return fmt.Errorf("contract violation: rendered output contains unsubstituted placeholder %s that is not in the canonical render-value set — scripts/commands/config.sh would emit this character-for-character into the generated file", match)
	}

	// Check 2 — YAML parses cleanly.
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		return fmt.Errorf("contract violation: rendered output is not valid YAML: %w", err)
	}

	// Check 3 — substituted values appear at expected paths.
	wantScrapeInterval := values["PROMETHEUS_SCRAPE_INTERVAL_S"] + "s"
	wantEvalInterval := values["PROMETHEUS_EVALUATION_INTERVAL_S"] + "s"
	wantCorePort := values["CORE_CONTAINER_PORT"]
	wantMLPort := values["ML_CONTAINER_PORT"]

	// global.scrape_interval / global.evaluation_interval
	global, ok := parsed["global"].(map[string]any)
	if !ok {
		return fmt.Errorf("contract violation: rendered document has no top-level `global` mapping")
	}
	if got, _ := global["scrape_interval"].(string); got != wantScrapeInterval {
		return fmt.Errorf("contract violation: global.scrape_interval=%q, want %q (substitution of PROMETHEUS_SCRAPE_INTERVAL_S did not apply)", got, wantScrapeInterval)
	}
	if got, _ := global["evaluation_interval"].(string); got != wantEvalInterval {
		return fmt.Errorf("contract violation: global.evaluation_interval=%q, want %q (substitution of PROMETHEUS_EVALUATION_INTERVAL_S did not apply)", got, wantEvalInterval)
	}

	// scrape_configs targets must include the substituted ports.
	scrapeConfigs, ok := parsed["scrape_configs"].([]any)
	if !ok {
		return fmt.Errorf("contract violation: rendered document has no scrape_configs list")
	}
	seenCore, seenML := false, false
	for _, scIface := range scrapeConfigs {
		sc, ok := scIface.(map[string]any)
		if !ok {
			continue
		}
		jobName, _ := sc["job_name"].(string)
		stcs, _ := sc["static_configs"].([]any)
		for _, stcIface := range stcs {
			stc, ok := stcIface.(map[string]any)
			if !ok {
				continue
			}
			targets, _ := stc["targets"].([]any)
			for _, tIface := range targets {
				target, _ := tIface.(string)
				switch jobName {
				case "smackerel-core":
					if target == "smackerel-core:"+wantCorePort {
						seenCore = true
					}
				case "smackerel-ml":
					if target == "smackerel-ml:"+wantMLPort {
						seenML = true
					}
				}
			}
		}
	}
	if !seenCore {
		return fmt.Errorf("contract violation: rendered scrape_configs has no target `smackerel-core:%s` — substitution of CORE_CONTAINER_PORT did not apply at the expected position", wantCorePort)
	}
	if !seenML {
		return fmt.Errorf("contract violation: rendered scrape_configs has no target `smackerel-ml:%s` — substitution of ML_CONTAINER_PORT did not apply at the expected position", wantMLPort)
	}
	return nil
}

// TestMonitoringRender_LiveTemplate is the primary T-049-002 assertion.
// It loads the live template, renders with canonical synthetic values,
// and asserts the result is a valid Prometheus config carrying those
// values at the expected paths.
func TestMonitoringRender_LiveTemplate(t *testing.T) {
	tmplPath := filepath.Join(repoRoot(t), "config", "prometheus", "prometheus.yml.tmpl")
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("failed to read live prometheus template %q: %v", tmplPath, err)
	}
	rendered := renderTemplate(t, string(raw), canonicalRenderValues)
	if err := assertRenderContract(rendered, canonicalRenderValues); err != nil {
		t.Fatalf("live config/prometheus/prometheus.yml.tmpl violates spec 049 FR-049-001/FR-049-005(b) render contract: %v", err)
	}
	t.Logf("contract OK: live template renders to valid YAML with all canonical substitutions applied (scrape_interval=%ss, evaluation_interval=%ss, core target port=%s, ml target port=%s)",
		canonicalRenderValues["PROMETHEUS_SCRAPE_INTERVAL_S"],
		canonicalRenderValues["PROMETHEUS_EVALUATION_INTERVAL_S"],
		canonicalRenderValues["CORE_CONTAINER_PORT"],
		canonicalRenderValues["ML_CONTAINER_PORT"],
	)
}

// TestMonitoringRender_AdversarialUnsubstitutedVar proves the contract
// catches a regression where rendering would leave a placeholder in
// place (e.g. config.sh forgets to add a new var to the envsubst
// allowlist).
func TestMonitoringRender_AdversarialUnsubstitutedVar(t *testing.T) {
	// Render with an INCOMPLETE value set — drop ML_CONTAINER_PORT.
	incomplete := map[string]string{
		"PROMETHEUS_SCRAPE_INTERVAL_S":     "23",
		"PROMETHEUS_EVALUATION_INTERVAL_S": "29",
		"CORE_CONTAINER_PORT":              "18080",
		// ML_CONTAINER_PORT intentionally omitted.
	}
	tmplPath := filepath.Join(repoRoot(t), "config", "prometheus", "prometheus.yml.tmpl")
	raw, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("failed to read live prometheus template %q: %v", tmplPath, err)
	}
	rendered := renderTemplate(t, string(raw), incomplete)
	err = assertRenderContract(rendered, incomplete)
	if err == nil {
		t.Fatal("adversarial contract test failed: rendering with ML_CONTAINER_PORT omitted produced a passing result (contract is tautological — would NOT catch a regression where config.sh drops a substitution var)")
	}
	if !strings.Contains(err.Error(), "${ML_CONTAINER_PORT}") {
		t.Fatalf("adversarial contract test failed: error did not name the unsubstituted placeholder '${ML_CONTAINER_PORT}': %v", err)
	}
	t.Logf("adversarial OK: missing substitution var is rejected with: %v", err)
}

// TestMonitoringRender_AdversarialInvalidYAML proves the contract
// catches a regression where the rendered output is not valid YAML
// or where the structure no longer matches the documented shape
// (e.g. someone reorganises the template and the substitution paths
// no longer resolve).
func TestMonitoringRender_AdversarialInvalidYAML(t *testing.T) {
	// This fixture is "valid YAML" but with the wrong structure: the
	// scrape_interval is hard-coded (`15s`) so substitution would not
	// affect it, AND the core target uses a literal port that does
	// not match the canonical render value. Either failure mode is
	// proof the contract catches corruption.
	const broken = `
global:
  scrape_interval: 15s
  evaluation_interval: 15s
scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-core:9090"  # literal port, not substituted
        labels:
          component: core
  - job_name: smackerel-ml
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-ml:9091"  # literal port, not substituted
        labels:
          component: ml
`
	err := assertRenderContract(broken, canonicalRenderValues)
	if err == nil {
		t.Fatal("adversarial contract test failed: corrupted rendered YAML passed (contract is tautological — would NOT catch a regression where substitution silently fails to apply to scrape_interval / target ports)")
	}
	// Acceptable failure modes:
	//   - YAML parse error
	//   - global.scrape_interval mismatch (substitution didn't apply)
	//   - missing canonical target (substitution didn't apply at port)
	switch {
	case strings.Contains(err.Error(), "valid YAML"):
	case strings.Contains(err.Error(), "global.scrape_interval"):
	case strings.Contains(err.Error(), "smackerel-core:") && strings.Contains(err.Error(), "want"):
	case strings.Contains(err.Error(), "smackerel-core:"+canonicalRenderValues["CORE_CONTAINER_PORT"]):
	case strings.Contains(err.Error(), "smackerel-ml:"+canonicalRenderValues["ML_CONTAINER_PORT"]):
	default:
		t.Fatalf("adversarial contract test failed: error did not name a known corruption signal (YAML/scrape_interval/target): %v", err)
	}
	t.Logf("adversarial OK: corrupted rendered output is rejected with: %v", err)
}
