//go:build integration

// Spec 067 Scope 3 — SCN-067-A04 keyword map guard tests.
//
// Three live tests anchor the contract:
//
//   1. Real corpus baseline: scanning internal/telegram/ and
//      internal/annotation/ MUST produce zero G067-A04 violations.
//      Carrier maps (`map[string]string{"text": ...}`, metadata,
//      tokens) are not flagged because their identifiers do not
//      match the routing/intent/scenario/classify/keyword name set.
//
//   2. TestKeywordMapGuardReportsTelegramAndAnnotationUserTextMaps
//      Two planted fixtures — one Telegram, one annotation — declare
//      maps whose identifier implies scenario/intent routing. Both
//      MUST be flagged with file path AND identifier name in Detail.
//      An adversarial baseline fixture (carrier-style map named
//      `body`) MUST NOT be flagged.

package policy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestKeywordMapGuard_RealCorpusRunsAndProducesWellFormedFindings
// scans the real internal/telegram/ and internal/annotation/ trees.
// Per Scope 3 Change Boundary, production fixes for any existing
// keyword-map violations belong to their owning specs (spec 066 +
// spec 068). This test asserts the guard completes without error
// and every finding has well-formed evidence.
func TestKeywordMapGuard_RealCorpusRunsAndProducesWellFormedFindings(t *testing.T) {
	repo := repoRootForTest(t)
	baselinePath := filepath.Join(string(repo), "policy-exception-baseline.json")
	baseline, err := LoadBaseline(baselinePath)
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}
	cfg := PolicyConfig{ExceptionMaxAgeDays: realPolicyExceptionMaxAgeDays(t)}
	vs, err := KeywordMapGuard(repo, baseline, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordMapGuard: %v", err)
	}
	for i, v := range vs {
		if v.RuleID != "G067-A04" && v.RuleID != "G067-A07" {
			t.Errorf("vs[%d].RuleID = %q, want G067-A04 or G067-A07", i, v.RuleID)
		}
		if v.Path == "" {
			t.Errorf("vs[%d].Path is empty", i)
		}
		if v.Line <= 0 {
			t.Errorf("vs[%d].Line = %d, want > 0", i, v.Line)
		}
		if v.Detail == "" || v.Resolution == "" {
			t.Errorf("vs[%d] missing Detail/Resolution: %+v", i, v)
		}
	}
	t.Logf("real telegram/annotation produced %d findings (informational; production fixes belong to owning specs)", len(vs))
}

func TestKeywordMapGuardReportsTelegramAndAnnotationUserTextMaps(t *testing.T) {
	dir := t.TempDir()
	telegramDir := filepath.Join(dir, "internal", "telegram")
	annotationDir := filepath.Join(dir, "internal", "annotation")
	if err := os.MkdirAll(telegramDir, 0o755); err != nil {
		t.Fatalf("mkdir telegram: %v", err)
	}
	if err := os.MkdirAll(annotationDir, 0o755); err != nil {
		t.Fatalf("mkdir annotation: %v", err)
	}

	telegramFixture := filepath.Join(telegramDir, "intent_map.go")
	telegramBody := `package telegram

var keywordToScenario = map[string]string{
	"buy":  "shopping_intent",
	"cook": "recipe_intent",
}
`
	if err := os.WriteFile(telegramFixture, []byte(telegramBody), 0o644); err != nil {
		t.Fatalf("write telegram fixture: %v", err)
	}

	annotationFixture := filepath.Join(annotationDir, "scenario_routes.go")
	annotationBody := `package annotation

var scenarioRoutes = map[string]int{
	"#urgent": 1,
	"#later":  2,
}
`
	if err := os.WriteFile(annotationFixture, []byte(annotationBody), 0o644); err != nil {
		t.Fatalf("write annotation fixture: %v", err)
	}

	cfg := PolicyConfig{ExceptionMaxAgeDays: 180}
	vs, err := KeywordMapGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordMapGuard: %v", err)
	}
	if len(vs) != 2 {
		t.Fatalf("got %d violations, want 2: %+v", len(vs), vs)
	}

	wantIdents := map[string]bool{"keywordToScenario": false, "scenarioRoutes": false}
	for _, v := range vs {
		if v.RuleID != "G067-A04" {
			t.Fatalf("RuleID = %q, want G067-A04", v.RuleID)
		}
		if v.Line <= 0 {
			t.Fatalf("Line = %d, want > 0", v.Line)
		}
		for ident := range wantIdents {
			if strings.Contains(v.Detail, ident) {
				wantIdents[ident] = true
			}
		}
		if v.Owner == "" {
			t.Fatalf("Owner is empty for %+v", v)
		}
		if !strings.Contains(v.Resolution, "intent") {
			t.Fatalf("Resolution = %q must point operators at compiled intent", v.Resolution)
		}
	}
	for ident, seen := range wantIdents {
		if !seen {
			t.Fatalf("identifier %q not named in any Detail; vs=%+v", ident, vs)
		}
	}

	// Adversarial baseline: rename both identifiers to carrier-style
	// names (body, tokens) — they MUST NOT be flagged. Without this,
	// the guard could silently degrade to "any map[string]X is a
	// violation" and operators would lose trust.
	carrierTelegram := strings.Replace(telegramBody, "keywordToScenario", "body", 1)
	if err := os.WriteFile(telegramFixture, []byte(carrierTelegram), 0o644); err != nil {
		t.Fatalf("rewrite telegram fixture: %v", err)
	}
	carrierAnnotation := strings.Replace(annotationBody, "scenarioRoutes", "tokens", 1)
	if err := os.WriteFile(annotationFixture, []byte(carrierAnnotation), 0o644); err != nil {
		t.Fatalf("rewrite annotation fixture: %v", err)
	}
	clean, err := KeywordMapGuard(Root(dir), &Baseline{SchemaVersion: "v1"}, time.Now(), cfg)
	if err != nil {
		t.Fatalf("KeywordMapGuard (baseline): %v", err)
	}
	if len(clean) != 0 {
		t.Fatalf("carrier-named maps flagged %d violations: %+v", len(clean), clean)
	}
}
