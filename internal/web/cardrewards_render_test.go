// Spec 092 — Go-level parse/render + CSP-clean coverage for the card-rewards
// templates.
//
// Closes the gap (scopes.md Finding #2) that cardRewardsTemplates /
// cardRewardsInsightsTemplates are parsed via template.Must inside
// NewCardRewardsWebHandler but exercised by no _test.go: handler_test.go's
// TestAllTemplates_Present / TestTemplates_NoInlineEventHandlers scan only the
// knowledge-base allTemplates const, never the card-rewards set. This test
// constructs the handler (so the template.Must parse runs) and renders all ten
// pages + their sub-pages + the two card-select partials with representative
// view models, asserting (a) no render error, (b) the spec-092 design-system
// markers are present (proving the new head/nav chrome applies), and (c) the
// rendered markup is Go-level CSP-clean (no inline <script>, no inline event
// handler) — mirroring the KB guard for the surface this feature changes.
package web

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/cardrewards"
)

func crRefStr(s string) *string        { return &s }
func crRefInt(i int) *int              { return &i }
func crRefTime(t time.Time) *time.Time { return &t }

// cardRewardsRenderFixtures returns one representative populated view model per
// page name so every page renders its populated (not just empty-state) branch.
func cardRewardsRenderFixtures() []struct {
	name string
	data map[string]any
} {
	deadline := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	card := walletCardRow{
		UserCard: cardrewards.UserCard{
			ID: "uc-1", CardCatalogID: "cat-1",
			Nickname: crRefStr("Daily Driver"), Note: crRefStr("everyday card"),
			Active: true, CatalogName: "Sample Cash",
		},
		CardType: "fixed",
	}
	offer := offerRow{
		Offer: cardrewards.Offer{
			ID: "of-1", UserCardID: crRefStr("uc-1"), Title: "5% Dining", Category: "dining",
			Rate: 5, RateType: "percent", LimitCents: crRefInt(50000),
			SharedLimitGroup: crRefStr("grp-a"), ActivationRequired: true, Activated: true,
		},
		CardName: "Sample Cash",
	}
	selection := selectionRow{
		Selection: cardrewards.Selection{
			ID: "sel-1", UserCardID: "uc-1", Category: "groceries", Tier: crRefInt(1),
			PeriodLabel: "2026-Q3", Enrolled: true,
		},
		CardName: "Sample Cash",
	}
	bonus := bonusRow{
		SignupBonus: cardrewards.SignupBonus{
			ID: "bn-1", UserCardID: "uc-1", BonusType: "spend", Description: "Spend $3k in 90 days",
			SpendRequiredCents: crRefInt(300000), SpendProgressCents: 150000,
			Deadline: crRefTime(deadline), Met: false,
		},
		CardName: "Sample Cash",
	}
	alias := cardrewards.CategoryAlias{
		CanonicalCategory: "groceries", Equivalents: []string{"supermarket", "grocery"},
		Starred: true, Priority: crRefInt(1),
	}
	rec := recommendationRow{
		CardRecommendation: cardrewards.CardRecommendation{
			ID: "rc-1", PeriodLabel: "2026-06", Category: "dining",
			RecommendedUserCardID: crRefStr("uc-1"), Rate: 5, Reason: "best dining rate",
			StarredOverride: true,
		},
		CardName: "Sample Cash",
	}
	rotating := rotatingRow{
		RotatingCategory: cardrewards.RotatingCategory{
			ID: "rt-1", CardCatalogID: "cat-1", PeriodLabel: "2026-Q3",
			Categories: []string{"gas", "groceries"}, Confidence: 0.42,
			NeedsVerification: true, ManualOverride: false,
		},
		CatalogName: "Sample Cash",
		Citations: []cardrewards.RotatingCategoryObservation{
			{ID: "ob-1", SourceName: "SourceA", SourceURL: "https://example.com/a",
				Categories: []string{"gas"}, Confidence: 0.4},
		},
	}
	pending := cardrewards.PendingReEnrollment{
		UserCardID: "uc-1", CatalogName: "Sample Cash", Category: "dining",
		Tier: crRefInt(1), PeriodLabel: "2026-Q3",
	}
	report := cardrewards.OptimizationReport{
		Period: "2026-06",
		Categories: []cardrewards.OptimizationResult{
			{Category: "dining", CardName: "Sample Cash", Rate: 5, RateType: "percent", Reason: "best pick"},
		},
	}
	run := cardrewards.CardRun{
		ID: "run-1", RunType: "scrape", Trigger: "manual", Status: "success",
		SourcesAttempted: 2, SourcesSucceeded: 2, CategoriesExtracted: 3, EventsWritten: 4,
		CreatedAt: time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC),
	}
	cards := []cardOption{{ID: "uc-1", Name: "Sample Cash"}}
	candidate := cardrewards.Candidate{CardID: "cat-1", Name: "Sample Cash", Score: 0.9, MatchType: "substring"}

	return []struct {
		name string
		data map[string]any
	}{
		{"cardrewards-wallet.html", map[string]any{"Title": "My Cards", "Cards": []walletCardRow{card}}},
		{"cardrewards-wallet-add.html", map[string]any{"Title": "Add Card", "Query": "cash", "Candidates": []cardrewards.Candidate{candidate}}},
		{"cardrewards-wallet-add-custom.html", map[string]any{"Title": "Add Custom Card"}},
		{"cardrewards-wallet-edit.html", map[string]any{"Title": "Edit Card", "Card": card.UserCard}},
		{"cardrewards-offers.html", map[string]any{"Title": "Offers", "Offers": []offerRow{offer}, "Cards": cards}},
		{"cardrewards-offer-edit.html", map[string]any{"Title": "Edit Offer", "Offer": offer.Offer, "Cards": cards}},
		{"cardrewards-selections.html", map[string]any{"Title": "Selections", "Selections": []selectionRow{selection}, "Cards": cards}},
		{"cardrewards-selection-edit.html", map[string]any{"Title": "Edit Selection", "Selection": selection.Selection, "Cards": cards}},
		{"cardrewards-bonuses.html", map[string]any{"Title": "Sign-up Bonuses", "Bonuses": []bonusRow{bonus}, "Cards": cards}},
		{"cardrewards-categories.html", map[string]any{"Title": "Categories", "Aliases": []cardrewards.CategoryAlias{alias}}},
		{"cardrewards-dashboard.html", map[string]any{
			"Title": "Card Rewards", "Period": "2026-06",
			"Recommendations": []recommendationRow{rec}, "ActiveRotating": []rotatingRow{rotating},
			"NeedsVerification": []rotatingRow{rotating}, "PendingReEnroll": []cardrewards.PendingReEnrollment{pending},
		}},
		{"cardrewards-recommendations.html", map[string]any{"Title": "Recommendations", "Period": "2026-06", "Recommendations": []recommendationRow{rec}, "Cards": cards}},
		{"cardrewards-rotating.html", map[string]any{"Title": "Rotating Categories", "Rows": []rotatingRow{rotating}}},
		{"cardrewards-report.html", map[string]any{"Title": "Optimization Report", "Report": report}},
		{"cardrewards-admin.html", map[string]any{"Title": "Admin", "Runs": []cardrewards.CardRun{run}, "TriggersEnabled": true}},
	}
}

// assertCardRewardsCSPClean fails if the rendered card-rewards markup contains
// an inline <script> or any inline event handler — the Go-level CSP guard for
// the card-rewards surface (the only inline style permitted is the server-
// computed .progress-fill width="…%", which is not an event handler).
func assertCardRewardsCSPClean(t *testing.T, page, html string) {
	t.Helper()
	for _, forbidden := range []string{"<script", "onclick=", "onsubmit=", "onload=", "onerror=", "onchange=", "javascript:"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("%s: rendered markup contains CSP-forbidden %q — the card-rewards pages must stay script-free", page, forbidden)
		}
	}
}

// TestCardRewardsTemplates_ParseAndRenderAllPages constructs the handler (so the
// template.Must parse of cardRewardsTemplates + cardRewardsInsightsTemplates
// runs) and renders every page with representative data, asserting no render
// error, the spec-092 design-system chrome markers, and Go-level CSP-cleanliness.
func TestCardRewardsTemplates_ParseAndRenderAllPages(t *testing.T) {
	h := NewCardRewardsWebHandler(nil)
	if h.Templates == nil {
		t.Fatal("expected parsed card-rewards templates")
	}

	// Spec-092 §3/§4 chrome markers present on every full page (proves the new
	// design-token system + responsive nav shell apply via head/nav).
	chromeMarkers := []string{"--bg-primary", ".cr-nav", ".main-content", "app-container"}

	for _, p := range cardRewardsRenderFixtures() {
		t.Run(p.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := h.Templates.ExecuteTemplate(&buf, p.name, p.data); err != nil {
				t.Fatalf("render %s: %v", p.name, err)
			}
			html := buf.String()
			if len(html) == 0 {
				t.Fatalf("render %s: empty output", p.name)
			}
			for _, marker := range chromeMarkers {
				if !strings.Contains(html, marker) {
					t.Errorf("render %s: missing design-system chrome marker %q", p.name, marker)
				}
			}
			assertCardRewardsCSPClean(t, p.name, html)
		})
	}
}

// TestCardRewardsTemplates_PartialsRenderCSPClean renders the two card-select
// partials (invoked inside the add forms) directly, proving they parse, render
// against a card list, and stay CSP-clean.
func TestCardRewardsTemplates_PartialsRenderCSPClean(t *testing.T) {
	h := NewCardRewardsWebHandler(nil)
	data := map[string]any{"Cards": []cardOption{{ID: "uc-1", Name: "Sample Cash"}}}
	for _, name := range []string{"cardrewards-card-select", "cardrewards-card-select-required"} {
		var buf bytes.Buffer
		if err := h.Templates.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		html := buf.String()
		if !strings.Contains(html, "user_card_id") {
			t.Errorf("render %s: expected the card <select name=user_card_id>", name)
		}
		assertCardRewardsCSPClean(t, name, html)
	}
}

// TestCardRewardsTemplates_ElevatedMarkersAndDataHooks is the Go-level proof for
// the SCOPE-02 (Scope-10 page bodies) and SCOPE-03 (Scope-11 page bodies)
// restructure: each page renders the spec-092 §4/§5 elevated component classes
// (markers) AND still carries every critical design §7 data-* test hook on its
// new element (the data-* preservation contract that the live Playwright specs
// lock by locator). A dropped/renamed hook here is the same regression an
// existing e2e-ui spec would catch — caught at Go speed, every `test unit --go`.
func TestCardRewardsTemplates_ElevatedMarkersAndDataHooks(t *testing.T) {
	h := NewCardRewardsWebHandler(nil)

	type pageExpect struct {
		markers   []string // spec-092 elevated design-system markers
		dataHooks []string // design §7 critical data-* hooks (must survive 1:1)
	}
	// Keyed by template name; values built against cardRewardsRenderFixtures()
	// populated view models (so the populated branch + its data-* hooks render).
	expect := map[string]pageExpect{
		// ---- SCOPE-02 (cardrewards_templates.go page bodies) ----
		"cardrewards-wallet.html": {
			markers: []string{"card-grid", "card-header", `class="badge badge-success"`, `class="badge type-badge"`, `class="btn btn-danger btn-sm"`},
			dataHooks: []string{
				`data-card-id="uc-1"`, "data-card-name", `data-card-type="fixed"`,
				`data-card-status="active"`, "data-card-note",
				`data-action="edit"`, `data-action="toggle"`, `data-action="delete"`,
			},
		},
		"cardrewards-wallet-add.html": {
			markers:   []string{"card-grid", `class="btn btn-primary btn-sm"`},
			dataHooks: []string{`data-candidate-id="cat-1"`, "data-candidate-name", `data-action="confirm-add"`},
		},
		"cardrewards-wallet-add-custom.html": {
			markers:   []string{"form-row", `class="btn btn-primary"`},
			dataHooks: []string{`data-action="create-custom"`},
		},
		"cardrewards-wallet-edit.html": {
			markers:   []string{"form-row", "btn-row"},
			dataHooks: []string{"data-card-catalog", `data-action="save-card"`},
		},
		"cardrewards-offers.html": {
			markers: []string{"card-grid", `class="badge badge-info"`, `class="badge badge-success"`},
			dataHooks: []string{
				`data-offer-id="of-1"`, "data-offer-title", "data-offer-card",
				`data-shared-limit-group="grp-a"`, `data-offer-status="activated"`,
				`data-action="edit"`, `data-action="create-offer"`,
			},
		},
		"cardrewards-offer-edit.html": {
			markers:   []string{"form-row", "btn-row"},
			dataHooks: []string{`data-action="save-offer"`},
		},
		"cardrewards-selections.html": {
			markers: []string{"card-grid", `class="badge badge-neutral"`},
			dataHooks: []string{
				`data-selection-id="sel-1"`, "data-selection-category", "data-selection-card",
				`data-selection-tier="1"`, `data-action="edit"`, `data-action="save-selection"`,
			},
		},
		"cardrewards-selection-edit.html": {
			markers:   []string{"form-row", "btn-row"},
			dataHooks: []string{`data-action="save-selection"`},
		},
		"cardrewards-bonuses.html": {
			markers: []string{"card-grid", `<div class="progress" role="progressbar"`, `aria-valuenow="50"`, `<div class="progress-fill" style="width:50%">`},
			dataHooks: []string{
				`data-bonus-id="bn-1"`, "data-bonus-description", "data-bonus-card",
				"data-bonus-progress", `data-action="update-progress"`, `data-action="create-bonus"`,
			},
		},
		"cardrewards-categories.html": {
			markers: []string{"table-wrap", `class="cr-table"`, `class="chip"`, `class="badge badge-starred"`},
			dataHooks: []string{
				`data-category="groceries"`, `data-starred="true"`, "data-category-name",
				"data-category-equivalents", `data-action="save-category"`,
			},
		},
		// ---- SCOPE-03 (cardrewards_dashboard_templates.go page bodies) ----
		"cardrewards-dashboard.html": {
			markers: []string{"stats-grid", `class="stat-card stat-card--link"`, "stat-card--urgent", "alert alert-warning", "card-grid"},
			dataHooks: []string{
				"data-dashboard", "data-rec-row", `data-rec-category="dining"`, "data-rec-card",
				"data-rec-starred", "data-rec-reason", "data-active-rotating", "data-catalog",
				"data-needs-verification", `data-badge="needs-verification"`, "data-pending-reenroll",
			},
		},
		"cardrewards-recommendations.html": {
			markers: []string{"card-grid", `class="badge badge-starred"`},
			dataHooks: []string{
				"data-rec-row", `data-rec-category="dining"`, `data-rec-starred="true"`,
				"data-rec-card", "data-rec-card-id", "data-rec-reason",
				`data-rec-starred-badge="true"`, `data-action="regenerate"`,
				`data-action="save-recommendation"`, `data-action="unstar"`,
			},
		},
		"cardrewards-rotating.html": {
			markers: []string{"card-grid", `class="progress"`, `role="progressbar"`, `style="width:42%"`, "progress-fill--warning"},
			dataHooks: []string{
				"data-rotating-row", `data-rotating-id="rt-1"`, `data-needs-verification="true"`,
				`data-manual-override="false"`, "data-confidence", "data-rotating-categories",
				"data-confidence-badge", `data-badge="needs-verification"`, "data-citation",
				`data-citation-source="SourceA"`, `data-action="verify"`,
			},
		},
		"cardrewards-report.html": {
			markers: []string{"table-wrap", `class="cr-table"`, "<strong>"},
			dataHooks: []string{
				"data-report-row", `data-report-category="dining"`, "data-report-card", "data-report-reason",
			},
		},
		"cardrewards-admin.html": {
			markers: []string{"table-wrap", `class="cr-table"`, `class="btn btn-secondary"`, `class="badge badge-success"`},
			dataHooks: []string{
				`data-action="scrape-now"`, `data-action="sync-calendar-now"`, "data-run-row",
				`data-run-id="run-1"`, `data-run-type="scrape"`, `data-run-trigger="manual"`,
				`data-run-status="success"`, "data-events-written", "data-events-written-cell",
			},
		},
	}

	fixtures := map[string]map[string]any{}
	for _, p := range cardRewardsRenderFixtures() {
		fixtures[p.name] = p.data
	}

	for name, e := range expect {
		t.Run(name, func(t *testing.T) {
			data, ok := fixtures[name]
			if !ok {
				t.Fatalf("no render fixture for %s", name)
			}
			var buf bytes.Buffer
			if err := h.Templates.ExecuteTemplate(&buf, name, data); err != nil {
				t.Fatalf("render %s: %v", name, err)
			}
			html := buf.String()
			for _, m := range e.markers {
				if !strings.Contains(html, m) {
					t.Errorf("render %s: missing elevated design-system marker %q", name, m)
				}
			}
			for _, d := range e.dataHooks {
				if !strings.Contains(html, d) {
					t.Errorf("render %s: dropped/renamed preserved data-* hook %q (design §7 violation)", name, d)
				}
			}
			// The restructured bodies must remain Go-level CSP-clean too.
			assertCardRewardsCSPClean(t, name, html)
		})
	}
}
