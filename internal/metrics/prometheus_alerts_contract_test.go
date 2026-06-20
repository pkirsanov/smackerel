// Package metrics — contract test for `config/prometheus/alerts.yml`.
//
// alerts.yml has no upstream validation surface in this repo: no CI step
// runs `promtool check rules`, no Go test parses it. A regression that
// truncates an expr, drops an alert, or breaks the YAML rule-group shape
// would ship silently to a deploy adapter that loads it (home-lab Caddy
// → Prometheus stack).
//
// This contract test enforces structural invariants:
//
//  1. The file parses as a valid Prometheus rule-group document.
//  2. Every group has a non-empty `name` and a non-empty `rules` list.
//  3. Every alert rule has non-empty `alert`, `expr`, `for`, `labels`,
//     and `annotations.summary` + `annotations.description`.
//  4. Every alert's `labels.severity` is one of {info, warning, critical}.
//  5. Every alert's `labels.component` is a non-empty string.
//  6. Specific alert rules that are explicitly owned by certified specs
//     MUST exist (regression guard against accidental deletion):
//     - spec 056 round 11 (devops sweep finding F-056-DEVOPS-001):
//     smackerel-connector-twitter group with TwitterAPIRateLimitChronicExhaustion
//     and TwitterAPIRetryStorm
//     - spec 030 / 049 (monitoring stack): smackerel-availability group
//     with SmackerelCoreUnavailable and SmackerelMLUnavailable
//     - spec 048 (backup-restore-automation): smackerel-backup group
//     with SmackerelBackupStale
//     - spec 046 (nats-production-hardening): smackerel-nats group
//     with SmackerelNATSDeadLetterPressure
//     - spec 050 (ml-sidecar-health-isolation): smackerel-ml-embedding
//     group with SmackerelMLEmbeddingStarvation
//     - spec 081 round 16 (devops sweep finding F-081-DEVOPS-001):
//     smackerel-ml-nats group with SmackerelMLNATSDeadLetterPressure
//     and SmackerelMLNATSDeadLetterPublishFailing
//     - spec 037 round 22 (devops sweep finding F-037-DEVOPS-001):
//     smackerel-agent group with SmackerelAgentProviderErrors,
//     SmackerelAgentInvocationTimeouts, and SmackerelAgentAllowlistViolations
//     - spec 038 round 30 (devops sweep finding F-038-DEVOPS-001):
//     smackerel-drive group with DriveProviderErrors, DriveSaveBackFailing,
//     and DriveRetrieveRefusalSpike (provider-neutral cloud-drive surface;
//     metrics emitted by internal/drive/observability/)
//     - spec 004 round 31 (devops sweep finding F-004-DEVOPS-001):
//     smackerel-intelligence group with
//     SmackerelIntelligenceAlertProductionFailing (production-side twin of
//     the delivery-side SmackerelAlertDeliveryFailing) and
//     SmackerelDigestSynthesisDegraded (digest LLM-synthesis fallback);
//     metrics emitted by internal/intelligence/ + internal/digest/
//
// Adversarial sub-tests prove the contract function would FAIL on each
// failure mode (yaml shape break, empty expr, unknown severity, deleted
// required alert). Without these, the test could be tautological.
package metrics

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// alertsDoc is the minimal YAML shape this test needs. It intentionally
// does NOT model every field of alerts.yml so adding unrelated fields
// (recording rules, query_offset, etc.) stays a non-event.
type alertsDoc struct {
	Groups []alertGroup `yaml:"groups"`
}

type alertGroup struct {
	Name  string      `yaml:"name"`
	Rules []alertRule `yaml:"rules"`
}

type alertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

// Required alert rules owned by certified specs. Deleting any of these
// in a future edit MUST fail the contract test rather than silently
// removing operator-facing alerting that an upstream spec relies on.
type requiredAlert struct {
	group string
	alert string
	owner string // documentation only — surfaces in the failure message
}

var requiredAlerts = []requiredAlert{
	{group: "smackerel-availability", alert: "SmackerelCoreUnavailable", owner: "spec 030 / 049 monitoring stack"},
	{group: "smackerel-availability", alert: "SmackerelMLUnavailable", owner: "spec 030 / 049 monitoring stack"},
	{group: "smackerel-nats", alert: "SmackerelNATSDeadLetterPressure", owner: "spec 046 nats-production-hardening"},
	{group: "smackerel-ml-embedding", alert: "SmackerelMLEmbeddingStarvation", owner: "spec 050 ml-sidecar-health-isolation"},
	{group: "smackerel-connector-twitter", alert: "TwitterAPIRateLimitChronicExhaustion", owner: "spec 056 round 11 devops sweep F-056-DEVOPS-001"},
	{group: "smackerel-connector-twitter", alert: "TwitterAPIRetryStorm", owner: "spec 056 round 11 devops sweep F-056-DEVOPS-001"},
	{group: "smackerel-backup", alert: "SmackerelBackupStale", owner: "spec 048 backup-restore-automation"},
	{group: "smackerel-connector-sync", alert: "ConnectorSyncFailureRateHigh24h", owner: "spec 005 / 011 maps connector devops F-005-DEVOPS-001"},
	{group: "smackerel-ml-nats", alert: "SmackerelMLNATSDeadLetterPressure", owner: "spec 081 round 16 devops sweep F-081-DEVOPS-001"},
	{group: "smackerel-ml-nats", alert: "SmackerelMLNATSDeadLetterPublishFailing", owner: "spec 081 round 16 devops sweep F-081-DEVOPS-001"},
	{group: "smackerel-agent", alert: "SmackerelAgentProviderErrors", owner: "spec 037 round 22 devops sweep F-037-DEVOPS-001"},
	{group: "smackerel-agent", alert: "SmackerelAgentInvocationTimeouts", owner: "spec 037 round 22 devops sweep F-037-DEVOPS-001"},
	{group: "smackerel-agent", alert: "SmackerelAgentAllowlistViolations", owner: "spec 037 round 22 devops sweep F-037-DEVOPS-001"},
	{group: "smackerel-drive", alert: "DriveProviderErrors", owner: "spec 038 round 30 devops sweep F-038-DEVOPS-001"},
	{group: "smackerel-drive", alert: "DriveSaveBackFailing", owner: "spec 038 round 30 devops sweep F-038-DEVOPS-001"},
	{group: "smackerel-drive", alert: "DriveRetrieveRefusalSpike", owner: "spec 038 round 30 devops sweep F-038-DEVOPS-001"},
	{group: "smackerel-intelligence", alert: "SmackerelIntelligenceAlertProductionFailing", owner: "spec 004 round 31 devops sweep F-004-DEVOPS-001"},
	{group: "smackerel-intelligence", alert: "SmackerelDigestSynthesisDegraded", owner: "spec 004 round 31 devops sweep F-004-DEVOPS-001"},
}

var allowedSeverities = map[string]bool{
	"info":     true,
	"warning":  true,
	"critical": true,
}

func alertsRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed — cannot locate test file")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

// assertAlertsContract returns nil iff every invariant holds for the
// alerts document encoded in yamlBytes. On any violation it returns a
// non-nil error naming the specific group/rule and the specific
// violation so the adversarial sub-tests can pattern-match.
func assertAlertsContract(yamlBytes []byte) error {
	var doc alertsDoc
	if err := yaml.Unmarshal(yamlBytes, &doc); err != nil {
		return fmt.Errorf("yaml.Unmarshal failed: %w", err)
	}
	if len(doc.Groups) == 0 {
		return fmt.Errorf("contract violation: alerts.yml has zero rule groups (expected >= 1)")
	}

	// Build a quick lookup of which (group, alert) pairs exist so the
	// requiredAlerts loop below can check membership in O(1).
	seen := make(map[string]bool, 32)
	for gi, g := range doc.Groups {
		if strings.TrimSpace(g.Name) == "" {
			return fmt.Errorf("contract violation: groups[%d].name is empty", gi)
		}
		if len(g.Rules) == 0 {
			return fmt.Errorf("contract violation: groups[%d].rules (group %q) is empty", gi, g.Name)
		}
		for ri, r := range g.Rules {
			where := fmt.Sprintf("groups[%d].rules[%d] (group=%q)", gi, ri, g.Name)
			if strings.TrimSpace(r.Alert) == "" {
				return fmt.Errorf("contract violation: %s.alert is empty", where)
			}
			if strings.TrimSpace(r.Expr) == "" {
				return fmt.Errorf("contract violation: %s.expr is empty for alert %q", where, r.Alert)
			}
			if strings.TrimSpace(r.For) == "" {
				return fmt.Errorf("contract violation: %s.for is empty for alert %q", where, r.Alert)
			}
			sev, ok := r.Labels["severity"]
			if !ok || strings.TrimSpace(sev) == "" {
				return fmt.Errorf("contract violation: %s.labels.severity missing for alert %q", where, r.Alert)
			}
			if !allowedSeverities[sev] {
				return fmt.Errorf("contract violation: %s.labels.severity=%q for alert %q is not in {info, warning, critical}", where, sev, r.Alert)
			}
			if strings.TrimSpace(r.Labels["component"]) == "" {
				return fmt.Errorf("contract violation: %s.labels.component missing for alert %q", where, r.Alert)
			}
			if strings.TrimSpace(r.Annotations["summary"]) == "" {
				return fmt.Errorf("contract violation: %s.annotations.summary missing for alert %q", where, r.Alert)
			}
			if strings.TrimSpace(r.Annotations["description"]) == "" {
				return fmt.Errorf("contract violation: %s.annotations.description missing for alert %q", where, r.Alert)
			}
			seen[g.Name+"|"+r.Alert] = true
		}
	}

	for _, req := range requiredAlerts {
		key := req.group + "|" + req.alert
		if !seen[key] {
			return fmt.Errorf("contract violation: required alert %q in group %q is missing (owner: %s) — deleting this rule removes operator-facing alerting that the owning spec relies on", req.alert, req.group, req.owner)
		}
	}

	return nil
}

// TestAlertsContract_LiveFile parses the live alerts.yml from the repo and
// asserts every invariant holds. A failure here means the file shipped
// with this commit would not be a valid Prometheus rule document, or it
// silently dropped an alert rule owned by a certified spec.
func TestAlertsContract_LiveFile(t *testing.T) {
	path := filepath.Join(alertsRepoRoot(t), "config", "prometheus", "alerts.yml")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := assertAlertsContract(b); err != nil {
		t.Fatalf("live alerts.yml violates contract: %v", err)
	}
}

// TestAlertsContract_AdversarialYAMLBreak proves the contract test would
// FAIL if the YAML structure regressed to invalid syntax (catches
// accidental tab/indent breakage that yaml.Unmarshal would reject).
func TestAlertsContract_AdversarialYAMLBreak(t *testing.T) {
	bad := []byte("groups:\n- name: g1\n  rules:\n  - alert: A\n      expr: 1  # bad indent\n")
	err := assertAlertsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test did not reject malformed YAML — would let broken alerts.yml ship")
	}
	if !strings.Contains(err.Error(), "yaml.Unmarshal failed") {
		t.Fatalf("expected yaml.Unmarshal failure message, got: %v", err)
	}
}

// TestAlertsContract_AdversarialEmptyExpr proves the contract test would
// FAIL if any alert's expr were truncated to empty (the most common
// silent-break failure mode: a copy/paste loses the PromQL body).
func TestAlertsContract_AdversarialEmptyExpr(t *testing.T) {
	bad := []byte(`groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelCoreUnavailable
    expr: ""
    for: 5m
    labels:
      severity: critical
      component: core
    annotations:
      summary: "core down"
      description: "core down"
`)
	err := assertAlertsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted an alert with empty expr — would let silently-broken PromQL ship")
	}
	if !strings.Contains(err.Error(), ".expr is empty") {
		t.Fatalf("expected '.expr is empty' message, got: %v", err)
	}
}

// TestAlertsContract_AdversarialUnknownSeverity proves the contract test
// would FAIL if a typo or invented severity ("warn", "high", "p1") shipped.
// Prometheus accepts any label value; the contract enforces the project
// convention so alertmanager routing rules stay deterministic.
func TestAlertsContract_AdversarialUnknownSeverity(t *testing.T) {
	bad := []byte(`groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelCoreUnavailable
    expr: up == 0
    for: 5m
    labels:
      severity: high
      component: core
    annotations:
      summary: "core down"
      description: "core down"
`)
	err := assertAlertsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test accepted severity=high — would let alertmanager routing silently break")
	}
	if !strings.Contains(err.Error(), "is not in {info, warning, critical}") {
		t.Fatalf("expected severity-rejection message, got: %v", err)
	}
}

// TestAlertsContract_AdversarialDeletedRequiredAlert proves the contract
// test would FAIL if a required alert owned by a certified spec were
// removed. Synthesizes a doc that is otherwise valid but omits
// TwitterAPIRateLimitChronicExhaustion (owned by spec 056 round 11) —
// without the requiredAlerts check, a refactor that "cleans up" alerts
// could silently drop operator-facing alerting an upstream spec relies on.
func TestAlertsContract_AdversarialDeletedRequiredAlert(t *testing.T) {
	bad := []byte(`groups:
- name: smackerel-availability
  rules:
  - alert: SmackerelCoreUnavailable
    expr: up == 0
    for: 5m
    labels: {severity: critical, component: core}
    annotations: {summary: "x", description: "y"}
  - alert: SmackerelMLUnavailable
    expr: up == 0
    for: 5m
    labels: {severity: critical, component: ml}
    annotations: {summary: "x", description: "y"}
- name: smackerel-nats
  rules:
  - alert: SmackerelNATSDeadLetterPressure
    expr: rate(x[5m]) > 0
    for: 10m
    labels: {severity: warning, component: nats}
    annotations: {summary: "x", description: "y"}
- name: smackerel-ml-embedding
  rules:
  - alert: SmackerelMLEmbeddingStarvation
    expr: rate(x[5m]) > 0
    for: 10m
    labels: {severity: warning, component: ml}
    annotations: {summary: "x", description: "y"}
- name: smackerel-backup
  rules:
  - alert: SmackerelBackupStale
    expr: time() > 0
    for: 1m
    labels: {severity: warning, component: backup}
    annotations: {summary: "x", description: "y"}
`)
	err := assertAlertsContract(bad)
	if err == nil {
		t.Fatalf("ADVERSARIAL FAILURE: contract test did not detect deletion of a required alert — F-056-DEVOPS-001 would silently regress")
	}
	if !strings.Contains(err.Error(), "TwitterAPIRateLimitChronicExhaustion") {
		t.Fatalf("expected message naming the deleted alert, got: %v", err)
	}
}
