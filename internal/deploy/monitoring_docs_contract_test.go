// Copyright (c) 2026 Philip Kirsanov
// SPDX-License-Identifier: MIT

// Package deploy spec 049 — Monitoring Stack documentation contract
// (T-049-005).
//
// FR-049-005(e): "`docs/Operations.md` MUST carry a Monitoring Stack
// section with a dashboard inventory, alert runbook, and metrics
// access boundary. Every alert name in `config/prometheus/alerts.yml`
// MUST appear at least once in the operator-facing docs so an on-call
// engineer reading Operations.md can find the firing-action runbook."
//
// This static test:
//
//  1. Asserts the canonical headings exist in `docs/Operations.md`:
//     - `## Monitoring Stack`
//     - `### Dashboard Inventory`
//     - `### Alert Runbook`
//     - `### Metrics Access Boundary`
//  2. Asserts every alert name from the live alerts.yml is mentioned
//     verbatim in `docs/Operations.md` (so the runbook stays
//     complete).
//
// Adversarial sub-tests prove the contract catches both a stripped
// heading and a missing alert mention.
//
// Cross-reference:
//   - specs/049-monitoring-stack/spec.md FR-049-005(e)
//   - docs/Operations.md "Monitoring Stack" section
package deploy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// requiredOperationsHeadings is the canonical set of section/sub-section
// headings that the Monitoring Stack section in Operations.md MUST
// contain. The headings are matched as substrings of full lines so
// trailing whitespace or surrounding decoration does not break the
// check.
var requiredOperationsHeadings = []string{
	"## Monitoring Stack",
	"### Dashboard Inventory",
	"### Alert Runbook",
	"### Metrics Access Boundary",
}

// assertDocsContract returns nil iff the documentation text contains
// every required heading AND every alert name from the supplied
// alerts-yaml is mentioned at least once.
func assertDocsContract(docsBytes, alertsBytes []byte) error {
	docs := string(docsBytes)

	// Heading checks.
	for _, h := range requiredOperationsHeadings {
		if !strings.Contains(docs, h) {
			return fmt.Errorf("contract violation: docs/Operations.md is missing required heading %q — spec 049 FR-049-005(e) demands the Monitoring Stack section carries a dashboard inventory, alert runbook, and metrics access boundary so on-call engineers can find runbook context", h)
		}
	}

	// Alert-name mentions. Reuse the promAlertDoc type from the
	// alerts-contract test (sibling file in the same package).
	var alertDoc promAlertDoc
	if err := yaml.Unmarshal(alertsBytes, &alertDoc); err != nil {
		return fmt.Errorf("yaml.Unmarshal(alerts.yml) failed: %w", err)
	}
	for _, g := range alertDoc.Groups {
		for _, r := range g.Rules {
			if r.Alert == "" {
				continue
			}
			if !strings.Contains(docs, r.Alert) {
				return fmt.Errorf("contract violation: alert %q from config/prometheus/alerts.yml is not mentioned in docs/Operations.md — an on-call engineer who searches for the alert name MUST find the runbook row; add the alert to the Alert Runbook table", r.Alert)
			}
		}
	}
	return nil
}

// TestMonitoringDocsContract_LiveFile is the primary T-049-005
// assertion.
func TestMonitoringDocsContract_LiveFile(t *testing.T) {
	root := repoRoot(t)
	docsBytes, err := os.ReadFile(filepath.Join(root, "docs", "Operations.md"))
	if err != nil {
		t.Fatalf("failed to read docs/Operations.md: %v", err)
	}
	alertsBytes, err := os.ReadFile(filepath.Join(root, "config", "prometheus", "alerts.yml"))
	if err != nil {
		t.Fatalf("failed to read config/prometheus/alerts.yml: %v", err)
	}
	if err := assertDocsContract(docsBytes, alertsBytes); err != nil {
		t.Fatalf("live docs/Operations.md violates spec 049 FR-049-005(e) docs contract: %v", err)
	}
	t.Logf("contract OK: docs/Operations.md satisfies spec 049 FR-049-005(e) (all required headings present; every alert name from alerts.yml is mentioned at least once)")
}

// TestMonitoringDocsContract_AdversarialMissingHeading proves the
// contract catches a regression where the Dashboard Inventory section
// is silently dropped (e.g. someone rewrites the section and forgets
// the subsection structure).
func TestMonitoringDocsContract_AdversarialMissingHeading(t *testing.T) {
	const docsFixture = `# Operations

## Monitoring Stack

This section is incomplete — the Dashboard Inventory and Alert Runbook
sub-headings were dropped during a refactor.

### Metrics Access Boundary

| Concern | Owner |
|---------|-------|
| /metrics endpoint | Product |
`
	const alertsFixture = `
groups:
- name: g
  rules:
  - alert: SmackerelCoreUnavailable
    expr: up == 0
`
	err := assertDocsContract([]byte(docsFixture), []byte(alertsFixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: docs missing 'Dashboard Inventory' heading was accepted (contract is tautological — would NOT catch a regression that drops a section)")
	}
	if !strings.Contains(err.Error(), "Dashboard Inventory") {
		t.Fatalf("adversarial contract test failed: error did not name the missing heading 'Dashboard Inventory': %v", err)
	}
	t.Logf("adversarial OK: docs missing required heading is rejected with: %v", err)
}

// TestMonitoringDocsContract_AdversarialMissingAlertMention proves the
// contract catches a regression where a new alert is added to
// alerts.yml but the runbook table in Operations.md isn't updated.
func TestMonitoringDocsContract_AdversarialMissingAlertMention(t *testing.T) {
	const docsFixture = `# Operations

## Monitoring Stack

### Dashboard Inventory

| # | Dashboard | Purpose |
|---|-----------|---------|
| 1 | Service Health | core/ML up status |

### Alert Runbook

Only SmackerelCoreUnavailable is mentioned here.

### Metrics Access Boundary

| Concern | Owner |
|---------|-------|
| /metrics | Product |
`
	const alertsFixture = `
groups:
- name: g
  rules:
  - alert: SmackerelCoreUnavailable
    expr: up{job="smackerel-core"} == 0
  - alert: SmackerelNewAlertNotInDocs
    expr: rate(smackerel_artifacts_ingested_total[1h]) == 0
`
	err := assertDocsContract([]byte(docsFixture), []byte(alertsFixture))
	if err == nil {
		t.Fatal("adversarial contract test failed: docs missing 'SmackerelNewAlertNotInDocs' mention was accepted (contract is tautological — would NOT catch a regression where a new alert is added without a runbook row)")
	}
	if !strings.Contains(err.Error(), "SmackerelNewAlertNotInDocs") {
		t.Fatalf("adversarial contract test failed: error did not name the missing alert mention: %v", err)
	}
	t.Logf("adversarial OK: docs missing alert mention is rejected with: %v", err)
}
