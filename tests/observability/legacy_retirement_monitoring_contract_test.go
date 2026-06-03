// Spec 076 SCOPE-6b — Legacy retirement monitoring-contract test.
//
// Validates the spec 076 SCOPE-6b observability artifacts:
//
//  1. deploy/observability/grafana/dashboards/legacy_retirement.json
//     parses as Grafana dashboard JSON, declares the rolling-7-day
//     residual-usage panel, and the panel queries the canonical
//     metric name registered in
//     internal/assistant/legacyretirement/telemetry.go.
//
//  2. deploy/observability/prometheus/alerts.legacy_retirement.yml.tmpl
//     parses as Prometheus alert-rule YAML, declares the
//     SmackerelLegacyRetirementResidualBreach rule, the rule
//     references the same canonical metric, AND the rule's
//     comparison threshold is the SST placeholder
//     ${LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS}
//     (not a numeric literal). Sourcing the threshold from the SST
//     key shared with spec 076 SCOPE-6a's evaluator means a single
//     SST edit moves both surfaces in lockstep with no literal
//     drift.
//
// Adversarial sub-test proves an injected numeric-literal threshold
// trips the SST-sourcing check, so a regression that hard-coded the
// rollback budget into the alert would fail this test.
//
// This is a unit-category test (no live Prometheus or Grafana
// required) so it runs under `./smackerel.sh test unit` and gates
// PRs ahead of the spec 076 SCOPE-6d live-stack execution of
// TP-076-06-04.

package observability

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	residualMetricName      = "smackerel_legacy_command_residual_total"
	rollbackThresholdEnvVar = "LEGACY_RETIREMENT_ROLLBACK_THRESHOLD_DAILY_INVOCATIONS"
	alertName               = "SmackerelLegacyRetirementResidualBreach"
)

func legacyRetirementRepoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	cur := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur
		}
		cur = filepath.Dir(cur)
	}
	t.Fatalf("could not locate repo root (go.mod) walking up from %q", wd)
	return ""
}

type lrPanel struct {
	ID      int           `json:"id"`
	Title   string        `json:"title"`
	Type    string        `json:"type"`
	Targets []panelTarget `json:"targets"`
}

type lrDashboard struct {
	Title         string    `json:"title"`
	UID           string    `json:"uid"`
	SchemaVersion int       `json:"schemaVersion"`
	Tags          []string  `json:"tags"`
	Panels        []lrPanel `json:"panels"`
}

// TestLegacyRetirementDashboard_ResidualPanelRollingSevenDay covers
// DoD #1 ("Grafana panel JSON committed and loads") and DoD #2
// ("rolling-7-day query returns" — static contract half; the live
// half is owned by spec 076 SCOPE-6d's TP-076-06-04 integration
// test).
func TestLegacyRetirementDashboard_ResidualPanelRollingSevenDay(t *testing.T) {
	path := filepath.Join(
		legacyRetirementRepoRoot(t),
		"deploy", "observability", "grafana", "dashboards",
		"legacy_retirement.json",
	)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var d lrDashboard
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if d.UID == "" {
		t.Error("dashboard uid must be set so Grafana's filesystem provider can address it")
	}
	if d.SchemaVersion < 38 {
		t.Errorf("schemaVersion %d below Grafana 9 minimum (38)", d.SchemaVersion)
	}
	if len(d.Panels) == 0 {
		t.Fatal("dashboard has no panels — spec 076 SCOPE-6b requires the residual panel")
	}
	// Locate the residual panel by metric reference.
	var residual *lrPanel
	for i := range d.Panels {
		for _, tgt := range d.Panels[i].Targets {
			if strings.Contains(tgt.Expr, residualMetricName) {
				residual = &d.Panels[i]
				break
			}
		}
		if residual != nil {
			break
		}
	}
	if residual == nil {
		t.Fatalf("no panel queries metric %q — spec 076 SCOPE-6b dashboard contract violated", residualMetricName)
	}
	// Assert the rolling-7-day window appears in the panel expression.
	// Either `[7d]` (range vector) or the dashboard `time.from = now-7d`
	// satisfies the "rolling 7-day" semantic; we enforce the range-vector
	// form because it makes the alert and the panel use the same window.
	var sawSevenDayWindow bool
	for _, tgt := range residual.Targets {
		if strings.Contains(tgt.Expr, "[7d]") {
			sawSevenDayWindow = true
			break
		}
	}
	if !sawSevenDayWindow {
		exprs := make([]string, 0, len(residual.Targets))
		for _, tgt := range residual.Targets {
			exprs = append(exprs, tgt.Expr)
		}
		t.Errorf("residual panel does not use a [7d] range vector; exprs=%v", exprs)
	}
}

// promAlertRule mirrors the spec 049 contract-test shape but is
// re-declared here so the spec 076 test stays self-contained and
// the test surface for SCOPE-6b is auditable in one file.
type promAlertRule struct {
	Alert       string            `yaml:"alert"`
	Expr        string            `yaml:"expr"`
	For         string            `yaml:"for"`
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}

type promAlertGroup struct {
	Name  string          `yaml:"name"`
	Rules []promAlertRule `yaml:"rules"`
}

type promAlertDoc struct {
	Groups []promAlertGroup `yaml:"groups"`
}

func loadLegacyRetirementAlertRule(t *testing.T) (template string, rule promAlertRule) {
	t.Helper()
	path := filepath.Join(
		legacyRetirementRepoRoot(t),
		"deploy", "observability", "prometheus",
		"alerts.legacy_retirement.yml.tmpl",
	)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// The template contains envsubst placeholders that are not valid
	// YAML by themselves on the right-hand side of `>` (they expand to
	// a bare integer at deploy time). Substitute a numeric sentinel
	// purely for the YAML parse so we can introspect the rule shape;
	// the unsubstituted template body is what we assert on for the
	// SST-sourcing check below.
	const sentinel = "424242424242"
	substituted := strings.ReplaceAll(string(body), "${"+rollbackThresholdEnvVar+"}", sentinel)
	var doc promAlertDoc
	if err := yaml.Unmarshal([]byte(substituted), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal %s after envsubst sentinel: %v", path, err)
	}
	for _, g := range doc.Groups {
		for _, r := range g.Rules {
			if r.Alert == alertName {
				return string(body), r
			}
		}
	}
	t.Fatalf("alert rule %q not found in %s", alertName, path)
	return "", promAlertRule{}
}

// TestLegacyRetirementAlert_QueriesResidualMetric covers DoD #3a:
// the alert rule references the same canonical metric the dashboard
// panel renders.
func TestLegacyRetirementAlert_QueriesResidualMetric(t *testing.T) {
	_, rule := loadLegacyRetirementAlertRule(t)
	if !strings.Contains(rule.Expr, residualMetricName) {
		t.Fatalf("alert %q expr does not reference metric %q; expr=%q", alertName, residualMetricName, rule.Expr)
	}
	if !strings.Contains(rule.Expr, "[7d]") {
		t.Fatalf("alert %q expr does not use a [7d] range vector; expr=%q", alertName, rule.Expr)
	}
}

// assertThresholdIsSSTSourced is the SST-vs-literal contract check.
// The unsubstituted template body MUST contain the env-var placeholder
// AND the rule's comparison RHS MUST NOT be a bare numeric literal.
//
// Returns nil iff both conditions hold.
func assertThresholdIsSSTSourced(templateBody, expr string) error {
	placeholder := "${" + rollbackThresholdEnvVar + "}"
	if !strings.Contains(templateBody, placeholder) {
		return fmt.Errorf("template body does not reference SST placeholder %s — threshold MUST be sourced from the SST key shared with spec 076 SCOPE-6a", placeholder)
	}
	// Find the comparison operator and inspect the right-hand side of
	// the rule expression. We accept `>`, `>=`, `<`, `<=`, `==`, `!=`
	// but the legacy-retirement rule uses `>`; the regex is generic so
	// a future rule swap stays covered.
	cmpRE := regexp.MustCompile(`(?s)(<=|>=|==|!=|<|>)\s*(.+?)$`)
	matches := cmpRE.FindStringSubmatch(strings.TrimSpace(expr))
	if matches == nil {
		return fmt.Errorf("could not locate comparison operator in alert expr %q", expr)
	}
	rhs := strings.TrimSpace(matches[2])
	// Strip a trailing pipe-or-newline block tail just in case.
	rhs = strings.TrimSpace(strings.SplitN(rhs, "\n", 2)[0])
	// SST-sourced RHS MUST contain the placeholder. A bare numeric
	// literal RHS is a contract violation regardless of the value.
	if !strings.Contains(rhs, placeholder) {
		// One more case: the placeholder may have been substituted by
		// the caller before we got here (we DO substitute a sentinel
		// inside loadLegacyRetirementAlertRule to make YAML parse).
		// Caller passes the UNSUBSTITUTED template body separately —
		// the rhs here is from the substituted parse, so the placeholder
		// won't be present. Instead, assert the rhs is NOT a literal
		// other than our sentinel.
		bareNumRE := regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)
		if bareNumRE.MatchString(rhs) && rhs != "424242424242" {
			return fmt.Errorf("alert expr RHS %q is a bare numeric literal; threshold MUST be sourced from %s", rhs, placeholder)
		}
		if rhs != "424242424242" {
			return fmt.Errorf("alert expr RHS %q is neither the SST placeholder nor the test sentinel; threshold MUST be sourced from %s", rhs, placeholder)
		}
	}
	return nil
}

// TestLegacyRetirementAlert_ThresholdSourcedFromSST covers DoD #3b:
// the alert threshold is sourced from the SST key, not a literal.
func TestLegacyRetirementAlert_ThresholdSourcedFromSST(t *testing.T) {
	templateBody, rule := loadLegacyRetirementAlertRule(t)
	if err := assertThresholdIsSSTSourced(templateBody, rule.Expr); err != nil {
		t.Fatalf("SST-sourcing contract violated: %v", err)
	}
}

// TestLegacyRetirementAlert_AdversarialLiteralThresholdRejected is
// the adversarial regression: if a future edit replaced the SST
// placeholder with a numeric literal on the RHS, the contract check
// MUST reject it. Without this sub-test the SST-sourcing check could
// silently pass on a degraded template.
func TestLegacyRetirementAlert_AdversarialLiteralThresholdRejected(t *testing.T) {
	const fakeTemplate = `groups:
- name: smackerel-legacy-retirement
  rules:
  - alert: SmackerelLegacyRetirementResidualBreach
    expr: sum by (command) (increase(smackerel_legacy_command_residual_total[7d])) > 100
    for: 15m
`
	// First confirm the fake parses and the rule lives where we expect.
	var doc promAlertDoc
	if err := yaml.Unmarshal([]byte(fakeTemplate), &doc); err != nil {
		t.Fatalf("yaml.Unmarshal fake template: %v", err)
	}
	if len(doc.Groups) == 0 || len(doc.Groups[0].Rules) == 0 {
		t.Fatal("fake template did not produce a rule")
	}
	rule := doc.Groups[0].Rules[0]
	err := assertThresholdIsSSTSourced(fakeTemplate, rule.Expr)
	if err == nil {
		t.Fatal("adversarial fake template with literal threshold was NOT rejected — SST-sourcing contract is vacuous")
	}
	if !strings.Contains(err.Error(), rollbackThresholdEnvVar) {
		t.Fatalf("rejection error %q does not mention SST key %s — operator wouldn't know how to fix it", err.Error(), rollbackThresholdEnvVar)
	}
}
