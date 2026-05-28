// Spec 061 SCOPE-09 — dashboard fragment schema test.
//
// Validates the assistant.json Grafana dashboard fragment is
// well-formed JSON, declares exactly the 7 panels design.md §8.4
// requires, and references ONLY metric series that the
// internal/assistant/metrics package or its provenance sibling
// register.
//
// This is the "honest substitution" target for DoD #4 when a live
// Grafana load is not available in CI.

package observability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type panel struct {
	ID          int            `json:"id"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Targets     []panelTarget  `json:"targets"`
	GridPos     map[string]int `json:"gridPos"`
}

type panelTarget struct {
	Expr string `json:"expr"`
}

type dashboard struct {
	Title         string                 `json:"title"`
	UID           string                 `json:"uid"`
	SchemaVersion int                    `json:"schemaVersion"`
	Panels        []panel                `json:"panels"`
	Templating    map[string]interface{} `json:"templating"`
}

func dashboardPath(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for cur := wd; cur != "/"; cur = filepath.Dir(cur) {
		p := filepath.Join(cur, "deploy", "observability", "grafana", "dashboards", "assistant.json")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Fatalf("could not locate deploy/observability/grafana/dashboards/assistant.json walking up from %q", wd)
	return ""
}

func loadDashboard(t *testing.T) dashboard {
	t.Helper()
	p := dashboardPath(t)
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	var d dashboard
	if err := json.Unmarshal(body, &d); err != nil {
		t.Fatalf("unmarshal %s: %v", p, err)
	}
	return d
}

// TestAssistantDashboard_IsValidGrafanaJSON proves the file parses
// as Grafana dashboard JSON and the canonical fields are set.
func TestAssistantDashboard_IsValidGrafanaJSON(t *testing.T) {
	d := loadDashboard(t)
	if d.Title == "" {
		t.Error("dashboard title must be set")
	}
	if d.UID == "" {
		t.Error("dashboard uid must be set (Grafana provider key)")
	}
	if d.SchemaVersion < 30 {
		t.Errorf("schemaVersion %d below Grafana 9 minimum", d.SchemaVersion)
	}
}

// TestAssistantDashboard_HasExactlySevenPanels enforces design.md
// §8.4 panel count (per-transport turn volume + band mix, per-scenario
// success/failure, capture-as-fallback, provenance violations, active
// threads, confirm-card outcomes, source-assembly drops).
func TestAssistantDashboard_HasExactlySevenPanels(t *testing.T) {
	d := loadDashboard(t)
	if got, want := len(d.Panels), 7; got != want {
		t.Errorf("panel count: want %d got %d (design §8.4 fixes this at 7)", want, got)
	}
	seenIDs := map[int]struct{}{}
	for _, p := range d.Panels {
		if _, dup := seenIDs[p.ID]; dup {
			t.Errorf("panel id %d duplicated", p.ID)
		}
		seenIDs[p.ID] = struct{}{}
		if p.Title == "" {
			t.Errorf("panel id %d missing title", p.ID)
		}
		if p.Type == "" {
			t.Errorf("panel id %d missing type", p.ID)
		}
		if p.Description == "" {
			t.Errorf("panel id %d missing description (operator runbook anchor)", p.ID)
		}
	}
}

// TestAssistantDashboard_OnlyReferencesRegisteredMetricSeries asserts
// every PromQL target references a metric series that exists in
// internal/assistant/metrics (8 series) or its provenance sibling
// (1 series) or the source-assembly counter (1 series). Drift in
// either direction fails: a dashboard referencing a non-existent
// metric is a runtime gap; a registered metric without a panel
// is acceptable (this test does not enforce the reverse direction).
func TestAssistantDashboard_OnlyReferencesRegisteredMetricSeries(t *testing.T) {
	d := loadDashboard(t)
	allowed := map[string]struct{}{
		"smackerel_assistant_facade_turns_total":            {},
		"smackerel_assistant_facade_latency_seconds":        {},
		"smackerel_assistant_router_band_total":             {},
		"smackerel_assistant_skill_invocations_total":       {},
		"smackerel_assistant_capture_fallback_total":        {},
		"smackerel_assistant_confirm_card_outcomes_total":   {},
		"smackerel_assistant_disambiguation_outcomes_total": {},
		"smackerel_assistant_active_threads":                {},
		"smackerel_assistant_provenance_violations_total":   {},
		"smackerel_assistant_source_assembly_drops_total":   {},
	}
	for _, p := range d.Panels {
		for _, tgt := range p.Targets {
			// Find every smackerel_assistant_ identifier in the expr.
			for _, tok := range tokensWithPrefix(tgt.Expr, "smackerel_assistant_") {
				if _, ok := allowed[tok]; !ok {
					t.Errorf("panel id %d references unregistered metric %q (expr=%q)",
						p.ID, tok, tgt.Expr)
				}
			}
		}
	}
}

// tokensWithPrefix extracts every contiguous [A-Za-z0-9_] run whose
// prefix matches `prefix`. Tiny ad-hoc tokenizer so the test stays
// dependency-free.
func tokensWithPrefix(s, prefix string) []string {
	var out []string
	i := 0
	for i < len(s) {
		if !isIdentChar(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && isIdentChar(s[j]) {
			j++
		}
		tok := s[i:j]
		if strings.HasPrefix(tok, prefix) {
			out = append(out, tok)
		}
		i = j
	}
	return out
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') ||
		(c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') ||
		c == '_'
}
