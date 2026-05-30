// BUG-061-003 — alias-map normalization unit tests + router pre-pass
// assertion proving the embed call receives the normalized form.

package agent

import (
	"context"
	"testing"
)

func TestNormalizeForRouting_AliasMap(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want string
	}{
		{"recepie", "recipe"},
		{"recipie", "recipe"},
		{"recipies", "recipes"},
		{"recepies", "recipes"},
		{"Recepie", "recipe"},
		{"RECEPIE", "recipe"},
		{"find best recepie", "find best recipe"},
		{"find best recepie.", "find best recipe."},
		{"recipies, please!", "recipes, please!"},
		{"find best recipe", "find best recipe"},
		{"what should I cook tonight", "what should I cook tonight"},
		{"", ""},
		{"   ", "   "},
		{"tagliatelle", "tagliatelle"},
		{"recepie? yes", "recipe? yes"},
	}
	for _, tc := range cases {
		got := NormalizeForRouting(tc.in)
		if got != tc.want {
			t.Errorf("NormalizeForRouting(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// TestRouter_NormalizesBeforeEmbed_BUG061003 — adversarial regression:
// the router MUST rewrite the input through NormalizeForRouting
// BEFORE embedding so misspelled trigger words still hit BandHigh.
// The envelope's RawInput is preserved for downstream skills.
func TestRouter_NormalizesBeforeEmbed_BUG061003(t *testing.T) {
	t.Parallel()

	recipeVec := []float32{1, 0, 0}
	other := []float32{0, 1, 0}
	emb := newRecordingEmbedder(map[string][]float32{
		"find best recipe": recipeVec,
		"weather":          other,
	})

	recipe := makeScenario("recipe_search", "find best recipe")
	weather := makeScenario("weather_query", "weather")

	r, err := NewRouter(context.Background(),
		defaultRoutingCfg(),
		[]*Scenario{recipe, weather}, emb)
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	env := IntentEnvelope{RawInput: "find best recepie"}
	sc, decision, ok := r.Route(context.Background(), env)
	if !ok || sc == nil || sc.ID != "recipe_search" {
		t.Fatalf("Route(%q) = (%v,%+v,%v); want recipe_search", env.RawInput, sc, decision, ok)
	}
	if env.RawInput != "find best recepie" {
		t.Errorf("env.RawInput mutated: got %q; want %q", env.RawInput, "find best recepie")
	}
}
