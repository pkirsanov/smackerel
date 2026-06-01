//go:build integration

// Spec 071 SCOPE-04 — Assistant Intents dashboard inventory (SCN-071-A06).
//
// Parses the committed Grafana dashboard JSON and asserts:
//
//   1. The required panels named in spec 071 design §"Assistant
//      Intents Dashboard" are present.
//   2. Each panel's Prometheus expression references one of the
//      canonical metrics the spec 071 recorder/retention/sweep + spec
//      064 refusal counter actually emit. A regression that renamed
//      the IntentTrace metric without updating the dashboard would
//      surface here.
//   3. The refusal-cause join panel (panel "Refusal causes (join with
//      openknowledge counter)") queries both
//      smackerel_assistant_intent_traces_total AND
//      openknowledge_refusal_total — without both targets the
//      dashboard cannot prove SCN-071-A07 vocabulary agreement.
//
// This test is in the `monitoring_integration` package because the
// dashboard inventory is part of the spec 049 monitoring stack
// contract, even though the JSON itself does not require a live
// Prometheus/Grafana to validate the contract.

package monitoring_integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type dashboard struct {
	Title  string   `json:"title"`
	UID    string   `json:"uid"`
	Tags   []string `json:"tags"`
	Panels []panel  `json:"panels"`
}

type panel struct {
	ID          int      `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Targets     []target `json:"targets"`
}

type target struct {
	RefID string `json:"refId"`
	Expr  string `json:"expr"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	// runtime.Caller(0) → this test file under tests/integration/monitoring/.
	// Walk two levels up to reach the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) returned !ok")
	}
	// thisFile = .../tests/integration/monitoring/<file>
	// repo root = three dirs up from <file> → tests/, then repo
	dir := filepath.Dir(thisFile) // .../tests/integration/monitoring
	for i := 0; i < 3; i++ {
		dir = filepath.Dir(dir)
	}
	return dir
}

func loadAssistantIntentsDashboard(t *testing.T) dashboard {
	t.Helper()
	path := filepath.Join(repoRoot(t), "deploy", "observability", "grafana", "dashboards", "assistant_intents.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var d dashboard
	if err := json.Unmarshal(b, &d); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if d.Title == "" || d.UID == "" {
		t.Fatalf("dashboard %s missing title/uid: %+v", path, d)
	}
	return d
}

// TestAssistantIntentsDashboardHasRequiredPanels — SCN-071-A06 panel
// inventory check.
func TestAssistantIntentsDashboardHasRequiredPanels(t *testing.T) {
	d := loadAssistantIntentsDashboard(t)

	// Design §"Assistant Intents Dashboard" required-panel set. The
	// titles can be edited for clarity but the semantic claim
	// (substring) MUST remain in the panel title so the dashboard
	// inventory stays self-documenting.
	required := []string{
		"Total turns",
		"Top action classes",
		"Clarification rate",
		"Refusal causes",
		"Compiler errors",
		"Capture-as-fallback",
		"Recent trace samples",
	}
	titles := make([]string, 0, len(d.Panels))
	for _, p := range d.Panels {
		titles = append(titles, p.Title)
	}
	for _, want := range required {
		found := false
		for _, got := range titles {
			if strings.Contains(got, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("required panel substring %q missing from dashboard panels %v", want, titles)
		}
	}
}

// TestAssistantIntentsDashboardQueriesCanonicalMetrics — SCN-071-A06
// metric-name regression. Each panel must reference at least one
// canonical metric that the spec 071 recorder/retention export or
// the spec 068 compiler or the spec 064 refusal counter actually
// emit. A typo / rename in the dashboard would surface here.
func TestAssistantIntentsDashboardQueriesCanonicalMetrics(t *testing.T) {
	d := loadAssistantIntentsDashboard(t)

	// Canonical metric names the dashboard is allowed to query.
	// Every panel's expr MUST reference at least one of these. New
	// metrics added later must be appended here AND to the
	// recorder/exporter that emits them.
	canonical := []string{
		"smackerel_assistant_intent_traces_total",                     // spec 071 SCOPE-02 export fan-out
		"smackerel_assistant_intent_trace_retention_sweep_rows_total", // spec 071 SCOPE-02 retention sweep
		"openknowledge_refusal_total",                                 // spec 064 refusal counter (join key)
		"intent_compiler_errors_total",                                // spec 068 compiler error counter
	}

	for _, p := range d.Panels {
		if len(p.Targets) == 0 {
			t.Errorf("panel %q (id=%d) has no targets", p.Title, p.ID)
			continue
		}
		matched := false
	OUTER:
		for _, tgt := range p.Targets {
			for _, name := range canonical {
				if strings.Contains(tgt.Expr, name) {
					matched = true
					break OUTER
				}
			}
		}
		if !matched {
			exprs := make([]string, 0, len(p.Targets))
			for _, tgt := range p.Targets {
				exprs = append(exprs, tgt.Expr)
			}
			t.Errorf("panel %q (id=%d) references no canonical metric; exprs=%v; allow-set=%v", p.Title, p.ID, exprs, canonical)
		}
	}
}

// TestAssistantIntentsDashboardRefusalPanelJoinsBothSources —
// SCN-071-A07 dashboard-side join check. The refusal panel MUST
// query both the IntentTrace metric AND the openknowledge counter
// in distinct targets so a missing series on either side is visible.
// Adversarial: a regression that dropped the openknowledge target
// would render only one half of the join and silently hide
// vocabulary drift.
func TestAssistantIntentsDashboardRefusalPanelJoinsBothSources(t *testing.T) {
	d := loadAssistantIntentsDashboard(t)

	var refusalPanel *panel
	for i := range d.Panels {
		if strings.Contains(d.Panels[i].Title, "Refusal causes") {
			refusalPanel = &d.Panels[i]
			break
		}
	}
	if refusalPanel == nil {
		t.Fatal("dashboard has no 'Refusal causes' panel — SCN-071-A07 join cannot be visualised")
	}

	var sawTraceMetric, sawCounterMetric bool
	for _, tgt := range refusalPanel.Targets {
		if strings.Contains(tgt.Expr, "smackerel_assistant_intent_traces_total") {
			sawTraceMetric = true
		}
		if strings.Contains(tgt.Expr, "openknowledge_refusal_total") {
			sawCounterMetric = true
		}
	}
	if !sawTraceMetric {
		t.Errorf("refusal panel does not query smackerel_assistant_intent_traces_total; join hides drift on the trace side")
	}
	if !sawCounterMetric {
		t.Errorf("refusal panel does not query openknowledge_refusal_total; join hides drift on the counter side")
	}
}
