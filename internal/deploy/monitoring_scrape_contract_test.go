// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 049 — Monitoring Stack Prometheus scrape contract
// (T-049-001).
//
// This test parses the committed Prometheus scrape template at
// `config/prometheus/prometheus.yml.tmpl` and asserts the FR-049-001
// invariants hold:
//
//  1. Both required scrape jobs exist: `smackerel-core` and
//     `smackerel-ml`.
//  2. Every target uses the compose service name (`smackerel-core`,
//     `smackerel-ml`) — never a literal IP, hostname, or tailnet
//     identifier. This is the "No env-specific content" guard for
//     monitoring configuration.
//  3. The `rule_files:` section references `/etc/prometheus/alerts.yml`
//     so the alert-rule contract test (T-049-004) has a known anchor.
//  4. The template substitutes only the four env vars that
//     scripts/commands/config.sh actually passes to envsubst
//     (CORE_CONTAINER_PORT, ML_CONTAINER_PORT,
//     PROMETHEUS_SCRAPE_INTERVAL_S, PROMETHEUS_EVALUATION_INTERVAL_S).
//     A stray `${SOME_OTHER_VAR}` would not be substituted and would
//     produce an invalid generated file at runtime; the test catches
//     it at build time.
//
// Adversarial sub-tests prove the contract function would FAIL on the
// two most likely regressions:
//   - Dropping the `smackerel-ml` job from the template.
//   - Replacing a target's service name with a literal IP.
//
// Cross-reference:
//   - specs/049-monitoring-stack/spec.md FR-049-001 / FR-049-005(a)
//   - specs/049-monitoring-stack/design.md "Prometheus Scrape Config Template"
//   - config/prometheus/prometheus.yml.tmpl
//   - scripts/commands/config.sh (envsubst step)
package deploy

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// promScrapeTemplate captures the minimal shape we assert. The
// `static_configs.targets` field is `[]string` because the template
// uses the short-form list (`- "host:port"`).
type promScrapeTemplate struct {
	Global struct {
		ScrapeInterval     string `yaml:"scrape_interval"`
		EvaluationInterval string `yaml:"evaluation_interval"`
		ExternalLabels     struct {
			Deployment string `yaml:"deployment"`
		} `yaml:"external_labels"`
	} `yaml:"global"`
	RuleFiles     []string `yaml:"rule_files"`
	ScrapeConfigs []struct {
		JobName       string `yaml:"job_name"`
		MetricsPath   string `yaml:"metrics_path"`
		StaticConfigs []struct {
			Targets []string          `yaml:"targets"`
			Labels  map[string]string `yaml:"labels"`
		} `yaml:"static_configs"`
	} `yaml:"scrape_configs"`
}

// allowedScrapeTemplateVars is the closed set of env-var names that
// scripts/commands/config.sh passes to envsubst when rendering the
// template. Any other ${VAR} pattern in the template is a bug.
var allowedScrapeTemplateVars = map[string]struct{}{
	"PROMETHEUS_SCRAPE_INTERVAL_S":     {},
	"PROMETHEUS_EVALUATION_INTERVAL_S": {},
	"CORE_CONTAINER_PORT":              {},
	"ML_CONTAINER_PORT":                {},
}

// scrapeVarRE matches `${IDENTIFIER}` placeholders in the template
// (excluding YAML/shell-escape forms; envsubst only substitutes
// `${VAR}` literal braced names).
var scrapeVarRE = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// requiredScrapeJobs is the canonical set of jobs the template MUST
// declare. Spec 049 FR-049-001. Extending this list requires extending
// the live runtime to expose a matching `/metrics` surface.
var requiredScrapeJobs = []string{"smackerel-core", "smackerel-ml"}

// assertScrapeTemplateContract returns nil iff the four invariants
// (FR-049-001 / FR-049-005(a)) hold for the YAML bytes.
func assertScrapeTemplateContract(yamlBytes []byte) error {
	// Check 1 — every `${VAR}` placeholder is in the allowlist.
	for _, match := range scrapeVarRE.FindAllStringSubmatch(string(yamlBytes), -1) {
		varName := match[1]
		if _, ok := allowedScrapeTemplateVars[varName]; !ok {
			return fmt.Errorf("contract violation: template references unknown env var ${%s}; scripts/commands/config.sh only passes %v to envsubst, so this placeholder would be emitted to the generated file unsubstituted (likely a typo or a forgotten config.sh update)", varName, sortedScrapeTemplateVars())
		}
	}

	// Check 2-4 — parse YAML and assert structural invariants.
	var doc promScrapeTemplate
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	if len(doc.RuleFiles) == 0 {
		return fmt.Errorf("contract violation: scrape template declares no rule_files; spec 049 FR-049-001 + FR-049-003 require rule_files to reference /etc/prometheus/alerts.yml so the alert-rule contract anchor exists")
	}
	if !containsString(doc.RuleFiles, "/etc/prometheus/alerts.yml") {
		return fmt.Errorf("contract violation: scrape template rule_files=%v does not include /etc/prometheus/alerts.yml — the alert-rule file (FR-049-003) is mounted at that path by the prometheus service in deploy/compose.deploy.yml; missing this entry means the rules never load", doc.RuleFiles)
	}

	jobNames := map[string]int{}
	for i, sc := range doc.ScrapeConfigs {
		jobNames[sc.JobName] = i
		if sc.MetricsPath != "/metrics" {
			return fmt.Errorf("contract violation: scrape_configs[%d].metrics_path=%q for job %q — every Smackerel scrape job MUST use /metrics (the unauthenticated Prometheus scrape pattern enforced by spec 030)", i, sc.MetricsPath, sc.JobName)
		}
		if len(sc.StaticConfigs) == 0 {
			return fmt.Errorf("contract violation: scrape_configs[%d] (job=%q) has no static_configs — every job MUST declare at least one target", i, sc.JobName)
		}
		for j, stc := range sc.StaticConfigs {
			if len(stc.Targets) == 0 {
				return fmt.Errorf("contract violation: scrape_configs[%d].static_configs[%d] (job=%q) has no targets — every static_configs entry MUST declare at least one host:port target", i, j, sc.JobName)
			}
			for k, t := range stc.Targets {
				if err := assertNoEnvSpecificContent(t); err != nil {
					return fmt.Errorf("contract violation: scrape_configs[%d].static_configs[%d].targets[%d]=%q (job=%q): %w — per `.github/copilot-instructions.md` 'No env-specific content', monitoring targets MUST be addressed by compose service name (e.g. smackerel-core:CORE_CONTAINER_PORT) so the file stays generic", i, j, k, t, sc.JobName, err)
				}
			}
		}
	}

	// Check: every required job is present.
	for _, required := range requiredScrapeJobs {
		if _, ok := jobNames[required]; !ok {
			return fmt.Errorf("contract violation: required scrape job %q is missing from the template — spec 049 FR-049-001 demands scrape coverage for both smackerel-core and smackerel-ml (the only product-owned /metrics surfaces today)", required)
		}
	}

	return nil
}

// assertNoEnvSpecificContent returns nil iff the target string contains
// no literal IPv4 address, IPv6 address, or hostname that looks
// environment-specific. The hostname-portion of `host:port` MUST equal
// a known compose service name; we accept that, plus generic
// substitution placeholders.
func assertNoEnvSpecificContent(target string) error {
	hostPort := target
	host := hostPort
	if idx := strings.LastIndex(hostPort, ":"); idx >= 0 {
		host = hostPort[:idx]
	}
	// Strip leading scheme noise just in case.
	host = strings.TrimSpace(host)

	// Reject literal IPv4. Even 127.0.0.1 is rejected here — the
	// compose-network scrape MUST go via service name, not loopback.
	if ip := net.ParseIP(host); ip != nil {
		return fmt.Errorf("target host %q is a literal IP address; use the compose service name instead", host)
	}
	// Reject any tailnet-shaped FQDN. The string `ts.net` is the
	// well-known Tailscale MagicDNS suffix; even though no real
	// tailnet identifier is permitted, we belt-and-braces the check.
	lower := strings.ToLower(host)
	if strings.HasSuffix(lower, ".ts.net") {
		return fmt.Errorf("target host %q contains a tailnet identifier; tailnet FQDNs belong to the deploy-adapter overlay, not the product repo", host)
	}
	// Accept the two known compose service names.
	if host == "smackerel-core" || host == "smackerel-ml" {
		return nil
	}
	// Accept the case where the target carries a `${VAR}` substitution
	// but the host portion is still a compose service name.
	if strings.HasPrefix(host, "smackerel-") {
		return nil
	}
	return fmt.Errorf("target host %q is neither a recognised compose service name nor a substitution placeholder", host)
}

// containsString is a tiny helper since this file uses Go's stdlib
// only and slices.Contains arrived in 1.21+; the repo's `go.mod`
// already uses 1.21+ but keeping this helper keeps the test
// self-contained.
func containsString(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// sortedScrapeTemplateVars returns the allowlist as a sorted slice
// for stable error messages.
func sortedScrapeTemplateVars() []string {
	out := make([]string, 0, len(allowedScrapeTemplateVars))
	for k := range allowedScrapeTemplateVars {
		out = append(out, k)
	}
	// inline insertion-sort to avoid pulling in sort just for an error
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// TestMonitoringScrapeContract_LiveTemplate is the primary T-049-001
// assertion. It loads the committed template and asserts every
// invariant holds.
func TestMonitoringScrapeContract_LiveTemplate(t *testing.T) {
	tmplPath := filepath.Join(repoRoot(t), "config", "prometheus", "prometheus.yml.tmpl")
	yamlBytes, err := os.ReadFile(tmplPath)
	if err != nil {
		t.Fatalf("failed to read live prometheus template %q: %v", tmplPath, err)
	}
	if err := assertScrapeTemplateContract(yamlBytes); err != nil {
		t.Fatalf("live config/prometheus/prometheus.yml.tmpl violates spec 049 FR-049-001 scrape contract: %v", err)
	}
	t.Logf("contract OK: config/prometheus/prometheus.yml.tmpl satisfies spec 049 FR-049-001 (jobs %v present, every target addresses a compose service name with no env-specific content, rule_files references /etc/prometheus/alerts.yml)", requiredScrapeJobs)
}

// TestMonitoringScrapeContract_AdversarialMissingMLJob proves the
// contract catches a regression where smackerel-ml is silently dropped
// from the scrape template (e.g. someone reorganising the file).
func TestMonitoringScrapeContract_AdversarialMissingMLJob(t *testing.T) {
	const fixture = `
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL_S}s
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL_S}s
rule_files:
  - /etc/prometheus/alerts.yml
scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-core:${CORE_CONTAINER_PORT}"
`
	err := assertScrapeTemplateContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: template missing smackerel-ml was accepted (contract is tautological — would NOT catch a regression that drops the ML scrape job)")
	}
	if !strings.Contains(err.Error(), "smackerel-ml") {
		t.Fatalf("adversarial contract test failed: error did not mention 'smackerel-ml': %v", err)
	}
	t.Logf("adversarial OK: template missing smackerel-ml is rejected with: %v", err)
}

// TestMonitoringScrapeContract_AdversarialLiteralIP proves the
// contract catches a regression where a target is addressed by literal
// IP instead of compose service name (the "No env-specific content"
// guard).
func TestMonitoringScrapeContract_AdversarialLiteralIP(t *testing.T) {
	const fixture = `
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL_S}s
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL_S}s
rule_files:
  - /etc/prometheus/alerts.yml
scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - "127.0.0.1:${CORE_CONTAINER_PORT}"
  - job_name: smackerel-ml
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-ml:${ML_CONTAINER_PORT}"
`
	err := assertScrapeTemplateContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: target with literal IP 127.0.0.1 was accepted (contract is tautological — would NOT catch a regression that bakes a host IP into the scrape config)")
	}
	if !strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("adversarial contract test failed: error did not name the offending literal '127.0.0.1': %v", err)
	}
	if !strings.Contains(err.Error(), "compose service name") {
		t.Fatalf("adversarial contract test failed: error did not mention 'compose service name': %v", err)
	}
	t.Logf("adversarial OK: literal IP target is rejected with: %v", err)
}

// TestMonitoringScrapeContract_AdversarialMissingRuleFiles proves the
// contract catches a regression where rule_files: is dropped, leaving
// alert rules unloaded.
func TestMonitoringScrapeContract_AdversarialMissingRuleFiles(t *testing.T) {
	const fixture = `
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL_S}s
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL_S}s
scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-core:${CORE_CONTAINER_PORT}"
  - job_name: smackerel-ml
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-ml:${ML_CONTAINER_PORT}"
`
	err := assertScrapeTemplateContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: template without rule_files was accepted")
	}
	if !strings.Contains(err.Error(), "rule_files") {
		t.Fatalf("adversarial contract test failed: error did not mention 'rule_files': %v", err)
	}
	t.Logf("adversarial OK: missing rule_files is rejected with: %v", err)
}

// TestMonitoringScrapeContract_AdversarialStrayEnvVar proves the
// contract catches a regression where a stray ${SOME_OTHER_VAR}
// placeholder leaks into the template — envsubst would leave that
// unsubstituted and the generated file would be invalid.
func TestMonitoringScrapeContract_AdversarialStrayEnvVar(t *testing.T) {
	const fixture = `
global:
  scrape_interval: ${PROMETHEUS_SCRAPE_INTERVAL_S}s
  evaluation_interval: ${PROMETHEUS_EVALUATION_INTERVAL_S}s
rule_files:
  - /etc/prometheus/alerts.yml
scrape_configs:
  - job_name: smackerel-core
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-core:${CORE_CONTAINER_PORT}"
        labels:
          deployment: ${SMACKEREL_DEPLOYMENT_ID}
  - job_name: smackerel-ml
    metrics_path: /metrics
    static_configs:
      - targets:
          - "smackerel-ml:${ML_CONTAINER_PORT}"
`
	err := assertScrapeTemplateContract([]byte(fixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: stray ${SMACKEREL_DEPLOYMENT_ID} placeholder was accepted")
	}
	if !strings.Contains(err.Error(), "SMACKEREL_DEPLOYMENT_ID") {
		t.Fatalf("adversarial contract test failed: error did not name the offending variable 'SMACKEREL_DEPLOYMENT_ID': %v", err)
	}
	t.Logf("adversarial OK: stray env-var placeholder is rejected with: %v", err)
}
