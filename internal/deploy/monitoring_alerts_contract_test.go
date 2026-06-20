// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 049 — Monitoring Stack alert-rules contract
// (T-049-004).
//
// FR-049-003 / FR-049-005(d): "Every alert rule in
// config/prometheus/alerts.yml MUST reference a metric name that is
// actually emitted by the live runtime today. Fabricated metrics or
// stale metric names that nobody emits anymore are blocked at build
// time."
//
// This test:
//
//  1. Extracts every `smackerel_*` metric name registered by the Go
//     core (internal/metrics/*.go plus the spec 038 provider-neutral
//     drive metrics in internal/drive/observability/metrics.go, which
//     the thin observability package registers directly) and the Python
//     sidecar (ml/app/metrics.py + ml/app/embedder.py docstrings as a
//     fallback).
//  2. Parses config/prometheus/alerts.yml and walks every rule's
//     `expr:` field.
//  3. Extracts every `smackerel_<identifier>` reference from each
//     expression.
//  4. Asserts every extracted reference exists in the known-emitted
//     set. The synthetic Prometheus `up` metric is whitelisted
//     because Prometheus injects it for every scrape target.
//  5. Asserts every alert in the canonical required-alerts list is
//     present (so a regression that silently drops `SmackerelCoreUnavailable`
//     would fail).
//
// Adversarial sub-tests prove the contract catches both fabricated
// metrics and missing required alerts.
//
// Cross-reference:
//   - specs/049-monitoring-stack/spec.md FR-049-003 / FR-049-005(d)
//   - specs/049-monitoring-stack/design.md "Alert Rules" table
//   - config/prometheus/alerts.yml
//   - internal/metrics/*.go
//   - ml/app/metrics.py
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// promAlertDoc captures the prometheus alert-rule shape we need.
type promAlertDoc struct {
	Groups []struct {
		Name  string `yaml:"name"`
		Rules []struct {
			Alert       string            `yaml:"alert"`
			Expr        string            `yaml:"expr"`
			For         string            `yaml:"for"`
			Labels      map[string]string `yaml:"labels"`
			Annotations map[string]string `yaml:"annotations"`
		} `yaml:"rules"`
	} `yaml:"groups"`
}

// requiredAlerts is the canonical set of alert names that MUST exist
// in config/prometheus/alerts.yml. Adding a new required alert means
// the runtime must emit a matching metric (or use a Prometheus
// builtin) AND a row must exist in the design.md alerts table.
var requiredAlerts = []string{
	"SmackerelCoreUnavailable",
	"SmackerelMLUnavailable",
	"SmackerelIngestionStalled",
	"SmackerelNATSDeadLetterPressure",
	"SmackerelDBPoolSaturated",
	"SmackerelMLEmbeddingStarvation",
	"SmackerelAlertDeliveryFailing",
	"SmackerelBackupStale",
}

// promBuiltinMetrics are Prometheus-injected synthetic metrics that
// the runtime does NOT emit but are nonetheless valid in alert
// expressions. `up` is the per-scrape availability gauge. `time()` is
// not a metric; it's a function — handled separately in the regex.
var promBuiltinMetrics = map[string]struct{}{
	"up": {},
}

// goMetricNameRE matches `Name: "smackerel_<identifier>"` lines in
// the Go runtime's prometheus.Counter/Gauge/Histogram declarations.
// The opening quote uses both straight `"` and (rarely) backtick — we
// only need the double-quote form because the codebase doesn't use
// raw strings for metric names.
var goMetricNameRE = regexp.MustCompile(`Name:\s*"(smackerel_[a-zA-Z0-9_]+)"`)

// pyMetricNameRE matches the metric-name string in
// `Counter("smackerel_<identifier>", ...)`, `Gauge(...)`,
// `Histogram(...)`, `Summary(...)`. The Python sidecar uses the
// positional-arg form so the metric name is always the first string
// after the opening paren.
var pyMetricNameRE = regexp.MustCompile(`(?:Counter|Gauge|Histogram|Summary)\s*\(\s*"(smackerel_[a-zA-Z0-9_]+)"`)

// alertExprMetricRE pulls every `smackerel_<identifier>` AND the bare
// `up` symbol out of an expression string. PromQL identifiers can
// appear as bare names, as part of `rate(name[5m])`, with `{label=...}`
// selectors, or with `sum by (...) (name)` aggregations — the regex
// is intentionally broad on the prefix and strict on the identifier
// shape so it works for all those forms.
var alertExprMetricRE = regexp.MustCompile(`\b(smackerel_[a-zA-Z0-9_]+|up)\b`)

// loadKnownEmittedMetrics walks the Go core and Python sidecar source
// trees and extracts every `smackerel_*` metric name registered as a
// prometheus collector. Returns a set keyed by metric name.
func loadKnownEmittedMetrics(t *testing.T) map[string]struct{} {
	t.Helper()
	known := map[string]struct{}{}
	for k := range promBuiltinMetrics {
		known[k] = struct{}{}
	}

	root := repoRoot(t)

	// Walk internal/metrics/*.go — that's the canonical home for
	// runtime metric registration. The path is grepped exhaustively
	// rather than via filepath.Walk because the directory is small
	// and the list is well-known.
	goMetricsDir := filepath.Join(root, "internal", "metrics")
	goFiles, err := os.ReadDir(goMetricsDir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", goMetricsDir, err)
	}
	for _, ent := range goFiles {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(ent.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Join(goMetricsDir, ent.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", ent.Name(), err)
		}
		for _, m := range goMetricNameRE.FindAllSubmatch(body, -1) {
			known[string(m[1])] = struct{}{}
		}
	}

	// Walk ml/app/metrics.py for Python-side registrations.
	pyMetricsFile := filepath.Join(root, "ml", "app", "metrics.py")
	if body, err := os.ReadFile(pyMetricsFile); err == nil {
		for _, m := range pyMetricNameRE.FindAllSubmatch(body, -1) {
			known[string(m[1])] = struct{}{}
		}
	}

	// Walk internal/drive/observability/metrics.go for the spec 038
	// provider-neutral drive metrics (smackerel_drive_scan_files_total,
	// _extract_files_total, _save_attempts_total,
	// _retrieve_decisions_total, _provider_errors_total). These live
	// OUTSIDE internal/metrics/ — the thin observability package registers
	// them with the global registry directly — so without this explicit
	// read the smackerel-drive alert group (spec 038 round 30 devops sweep
	// F-038-DEVOPS-001) would reference metrics this contract could not
	// see and would falsely reject. The metric-name declaration form is
	// identical to internal/metrics/*.go, so goMetricNameRE applies.
	driveObsFile := filepath.Join(root, "internal", "drive", "observability", "metrics.go")
	if body, err := os.ReadFile(driveObsFile); err == nil {
		for _, m := range goMetricNameRE.FindAllSubmatch(body, -1) {
			known[string(m[1])] = struct{}{}
		}
	}

	return known
}

// assertAlertsContract returns nil iff every required alert exists in
// the document AND every metric referenced in every expression is in
// the known-emitted set.
func assertAlertsContract(yamlBytes []byte, knownEmitted map[string]struct{}) error {
	var doc promAlertDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}

	// Build the set of declared alerts and validate every expression.
	declared := map[string]bool{}
	for gi, g := range doc.Groups {
		for ri, r := range g.Rules {
			if r.Alert == "" {
				return fmt.Errorf("contract violation: groups[%d].rules[%d] has empty alert name (group=%q)", gi, ri, g.Name)
			}
			if r.Expr == "" {
				return fmt.Errorf("contract violation: alert %q has empty expr", r.Alert)
			}
			declared[r.Alert] = true
			refs := alertExprMetricRE.FindAllStringSubmatch(r.Expr, -1)
			for _, ref := range refs {
				name := ref[1]
				if _, ok := knownEmitted[name]; !ok {
					return fmt.Errorf("contract violation: alert %q references metric %q which is NOT emitted by the live runtime (not found in internal/metrics/*.go or ml/app/metrics.py) — either add the instrumentation in the runtime or remove/correct the alert", r.Alert, name)
				}
			}
		}
	}

	// Every required alert is present.
	for _, required := range requiredAlerts {
		if !declared[required] {
			return fmt.Errorf("contract violation: required alert %q is missing from config/prometheus/alerts.yml — spec 049 FR-049-003 + design.md alert-table demand this alert exists", required)
		}
	}

	return nil
}

// TestMonitoringAlertsContract_LiveFile is the primary T-049-004
// assertion.
func TestMonitoringAlertsContract_LiveFile(t *testing.T) {
	alertsPath := filepath.Join(repoRoot(t), "config", "prometheus", "alerts.yml")
	yamlBytes, err := os.ReadFile(alertsPath)
	if err != nil {
		t.Fatalf("failed to read live alerts.yml %q: %v", alertsPath, err)
	}
	known := loadKnownEmittedMetrics(t)
	if len(known) <= len(promBuiltinMetrics) {
		t.Fatalf("loadKnownEmittedMetrics returned only %d metrics — the regexes failed to extract anything from internal/metrics/*.go (this would make the contract vacuous)", len(known))
	}
	if err := assertAlertsContract(yamlBytes, known); err != nil {
		t.Fatalf("live config/prometheus/alerts.yml violates spec 049 FR-049-003 alert contract: %v", err)
	}
	// Build a sorted list for the success log.
	names := make([]string, 0, len(known))
	for k := range known {
		names = append(names, k)
	}
	sort.Strings(names)
	t.Logf("contract OK: live alerts.yml satisfies spec 049 FR-049-003 (all %d required alerts present; every metric reference is in the %d-entry known-emitted set including builtin `up`)", len(requiredAlerts), len(known))
}

// TestMonitoringAlertsContract_AdversarialFabricatedMetric proves the
// contract catches a regression where an alert references a metric
// name that no part of the runtime emits.
func TestMonitoringAlertsContract_AdversarialFabricatedMetric(t *testing.T) {
	const fixture = `
groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelCoreUnavailable
    expr: up{job="smackerel-core"} == 0
    for: 2m
  - alert: SmackerelMLUnavailable
    expr: up{job="smackerel-ml"} == 0
    for: 2m
  - alert: SmackerelIngestionStalled
    expr: rate(smackerel_artifacts_ingested_total[30m]) == 0
    for: 15m
  - alert: SmackerelNATSDeadLetterPressure
    expr: rate(smackerel_nats_deadletter_total[10m]) > 0.05
    for: 10m
  - alert: SmackerelDBPoolSaturated
    expr: smackerel_db_connections_active >= 9
    for: 5m
  - alert: SmackerelMLEmbeddingStarvation
    expr: rate(smackerel_ml_embedding_rejected_total[5m]) > 0
    for: 10m
  - alert: SmackerelAlertDeliveryFailing
    expr: rate(smackerel_alert_delivery_failures_total[15m]) > 0
    for: 15m
  - alert: SmackerelBackupStale
    expr: rate(smackerel_fabricated_metric_does_not_exist[1h]) == 0
    for: 30m
`
	known := loadKnownEmittedMetrics(t)
	err := assertAlertsContract([]byte(fixture), known)
	if err == nil {
		t.Fatal("adversarial contract test failed: fabricated metric was accepted (contract is tautological — would NOT catch a regression that references smackerel_fabricated_metric_does_not_exist)")
	}
	if !strings.Contains(err.Error(), "smackerel_fabricated_metric_does_not_exist") {
		t.Fatalf("adversarial contract test failed: error did not name the fabricated metric: %v", err)
	}
	t.Logf("adversarial OK: fabricated metric is rejected with: %v", err)
}

// TestMonitoringAlertsContract_AdversarialMissingRequiredAlert proves
// the contract catches a regression where a required alert is silently
// dropped from the rule file.
func TestMonitoringAlertsContract_AdversarialMissingRequiredAlert(t *testing.T) {
	// Same alerts.yml content but with SmackerelCoreUnavailable removed.
	const fixture = `
groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelMLUnavailable
    expr: up{job="smackerel-ml"} == 0
    for: 2m
  - alert: SmackerelIngestionStalled
    expr: rate(smackerel_artifacts_ingested_total[30m]) == 0
    for: 15m
  - alert: SmackerelNATSDeadLetterPressure
    expr: rate(smackerel_nats_deadletter_total[10m]) > 0.05
    for: 10m
  - alert: SmackerelDBPoolSaturated
    expr: smackerel_db_connections_active >= 9
    for: 5m
  - alert: SmackerelMLEmbeddingStarvation
    expr: rate(smackerel_ml_embedding_rejected_total[5m]) > 0
    for: 10m
  - alert: SmackerelAlertDeliveryFailing
    expr: rate(smackerel_alert_delivery_failures_total[15m]) > 0
    for: 15m
  - alert: SmackerelBackupStale
    expr: rate(smackerel_artifacts_ingested_total[24h]) == 0
    for: 30m
`
	known := loadKnownEmittedMetrics(t)
	err := assertAlertsContract([]byte(fixture), known)
	if err == nil {
		t.Fatal("adversarial contract test failed: file missing SmackerelCoreUnavailable was accepted (contract is tautological — would NOT catch a regression that drops a required alert)")
	}
	if !strings.Contains(err.Error(), "SmackerelCoreUnavailable") {
		t.Fatalf("adversarial contract test failed: error did not name the missing alert 'SmackerelCoreUnavailable': %v", err)
	}
	t.Logf("adversarial OK: missing required alert is rejected with: %v", err)
}

// TestMonitoringAlertsContract_AdversarialEmptyExpr proves the
// contract catches a regression where an alert has an empty expr (a
// silent no-op rule).
func TestMonitoringAlertsContract_AdversarialEmptyExpr(t *testing.T) {
	const fixture = `
groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelCoreUnavailable
    expr: ""
    for: 2m
`
	known := loadKnownEmittedMetrics(t)
	err := assertAlertsContract([]byte(fixture), known)
	if err == nil {
		t.Fatal("adversarial contract test failed: alert with empty expr was accepted")
	}
	if !strings.Contains(err.Error(), "empty expr") {
		t.Fatalf("adversarial contract test failed: error did not name the empty-expr problem: %v", err)
	}
	t.Logf("adversarial OK: alert with empty expr is rejected with: %v", err)
}
