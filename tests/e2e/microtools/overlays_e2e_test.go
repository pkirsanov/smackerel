//go:build e2e

// Spec 076 SCOPE-3 — TP-076-03-06.
//
// Regression E2E: TestMicroToolOverlays_FullMatrix.
//
// Drives the live spec 037 agent tool registry end-to-end across the
// four generic micro-tools shipped under spec 065 and re-asserted by
// spec 076 SCOPE-3:
//
//   - SCN-065-A01 location_normalize resolved case (canonical city)
//   - SCN-065-A02 location_normalize ambiguous case (Springfield)
//   - SCN-065-A03 location_normalize overlay rule (US state-abbrev
//     preprocessing applied without leaking PII into the provider
//     query)
//   - SCN-065-A04 unit_convert (3 cups flour → grams via the live
//     deterministic catalog)
//   - SCN-065-A05 calculator adversarial (identifier/function refusal)
//   - SCN-065-A06 entity_resolve resolved + ambiguous branches
//
// "Live" here means the registered Handler returned by
// agent.ByName(...) — the same function the production agent loop
// invokes. Stub Providers/Resolvers are injected via the exported
// Set*Services wiring so the registry-side envelope contract, schema
// validation, and overlay preprocessing are exercised exactly as in
// production. Catalog/expression evaluation for unit_convert and
// calculator runs against the real, non-stubbed package code.
//
// Tool-registry canary: the first subtest asserts every micro-tool
// is present in the spec 037 registry. If any registration regresses
// the canary fails before per-scenario checks run.

package microtools_e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/microtools"
)

// callTool invokes the registered handler for the given tool name
// with the supplied JSON args, validates the returned envelope
// against the shared invariants, and returns the decoded shape.
func callTool(t *testing.T, name string, args any) microtools.Envelope {
	t.Helper()
	tool, ok := agent.ByName(name)
	if !ok {
		t.Fatalf("tool %q is not registered in the spec 037 agent registry", name)
	}
	if tool.Handler == nil {
		t.Fatalf("tool %q registered with nil Handler", name)
	}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args for %q: %v", name, err)
	}
	out, err := tool.Handler(context.Background(), raw)
	if err != nil {
		t.Fatalf("handler %q returned error: %v", name, err)
	}
	if err := microtools.ValidateEnvelopeBytes(out); err != nil {
		t.Fatalf("handler %q returned envelope failing ValidateEnvelopeBytes: %v\nraw=%s", name, err, string(out))
	}
	var env microtools.Envelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope for %q: %v\nraw=%s", name, err, string(out))
	}
	return env
}

// stubLocationProvider records the exact normalized query forwarded
// to the geocoder so SCN-065-A03 can assert the overlay rule rewrote
// "palm springs ca" into the canonical "Palm Springs, California"
// before the provider was contacted (i.e. no raw user PII leaked).
type stubLocationProvider struct {
	byQuery   map[string][]microtools.LocationCandidate
	lastQuery string
}

func (s *stubLocationProvider) Name() string { return "stub-open-meteo" }

func (s *stubLocationProvider) Geocode(_ context.Context, query string) ([]microtools.LocationCandidate, error) {
	s.lastQuery = query
	if c, ok := s.byQuery[query]; ok {
		return c, nil
	}
	return nil, nil
}

// stubEntityResolver supplies deterministic ranked candidates for
// the entity_resolve handler and records the user_id it received so
// the matrix can assert user-scoping is honored end-to-end.
type stubEntityResolver struct {
	candidates []microtools.EntityCandidate
	lastUser   string
}

func (s *stubEntityResolver) Resolve(_ context.Context, userID, _, _ string, _ int) ([]microtools.EntityCandidate, error) {
	s.lastUser = userID
	return s.candidates, nil
}

func TestMicroToolOverlays_FullMatrix(t *testing.T) {
	// Wire deterministic services BEFORE the canary so the tools whose
	// registration is gated behind Set*Services (unit_convert,
	// calculator) are present in the registry when the canary runs.
	// location_normalize and entity_resolve register at package init
	// regardless; wiring services here ensures their handlers can also
	// execute against deterministic stubs in the per-scenario subtests.
	locProv := &stubLocationProvider{byQuery: map[string][]microtools.LocationCandidate{
		"Palm Springs, California": {
			{Name: "Palm Springs", Admin1: "California", Country: "United States", Latitude: 33.83, Longitude: -116.55, Confidence: 1.0},
		},
		"springfield": {
			{Name: "Springfield", Admin1: "Illinois", Country: "United States", Latitude: 39.78, Longitude: -89.65, Confidence: 0.55},
			{Name: "Springfield", Admin1: "Missouri", Country: "United States", Latitude: 37.21, Longitude: -93.29, Confidence: 0.50},
			{Name: "Springfield", Admin1: "Massachusetts", Country: "United States", Latitude: 42.10, Longitude: -72.59, Confidence: 0.40},
		},
	}}
	microtools.SetLocationServices(&microtools.LocationServices{
		Provider:          locProv,
		Cache:             microtools.NewLocationCache(5*time.Second, 8),
		AmbiguityFloor:    0.6,
		AmbiguityMaxCands: 5,
		Timeout:           2 * time.Second,
	})
	t.Cleanup(microtools.ResetLocationServicesForTest)

	microtools.SetUnitConvertServices(&microtools.UnitConvertServices{CatalogVersion: "spec-076-e2e"})
	t.Cleanup(microtools.ResetUnitConvertServicesForTest)

	microtools.SetCalculatorServices(&microtools.CalculatorServices{MaxExpressionChars: 256})
	t.Cleanup(microtools.ResetCalculatorServicesForTest)

	entResolver := &stubEntityResolver{}
	microtools.SetEntityResolveServices(&microtools.EntityResolveServices{
		Resolver:        entResolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})
	t.Cleanup(microtools.ResetEntityResolveServicesForTest)

	// Tool-registry canary: every micro-tool MUST be registered with
	// the live spec 037 registry. unit_convert and calculator register
	// lazily on the first SetServices call; the wiring above guarantees
	// the canary observes the registered set the production agent loop
	// dispatches against.
	t.Run("tool_registry_canary", func(t *testing.T) {
		for _, name := range []string{
			microtools.LocationNormalizeToolName,
			microtools.UnitConvertToolName,
			microtools.CalculatorToolName,
			microtools.EntityResolveToolName,
		} {
			if !agent.Has(name) {
				t.Errorf("tool %q missing from spec 037 registry", name)
			}
		}
	})

	t.Run("SCN-065-A01_location_normalize_resolved", func(t *testing.T) {
		env := callTool(t, microtools.LocationNormalizeToolName, map[string]string{"input": "palm springs ca"})
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status=%q, want resolved (env=%+v)", env.Status, env)
		}
		if got := env.Value["name"]; got != "Palm Springs" {
			t.Errorf("value.name=%v, want Palm Springs", got)
		}
		if got := env.Value["admin1"]; got != "California" {
			t.Errorf("value.admin1=%v, want California", got)
		}
	})

	t.Run("SCN-065-A02_location_normalize_ambiguous", func(t *testing.T) {
		env := callTool(t, microtools.LocationNormalizeToolName, map[string]string{"input": "springfield"})
		if env.Status != microtools.StatusAmbiguous {
			t.Fatalf("status=%q, want ambiguous (env=%+v)", env.Status, env)
		}
		if len(env.Candidates) == 0 {
			t.Fatal("ambiguous envelope MUST carry candidates")
		}
		if len(env.Candidates) > 5 {
			t.Errorf("candidates len=%d, want <=5", len(env.Candidates))
		}
		if len(env.Value) != 0 {
			t.Errorf("ambiguous envelope leaked Value: %v", env.Value)
		}
	})

	t.Run("SCN-065-A03_location_overlay_rewrites_query", func(t *testing.T) {
		// Use a dedicated provider/services pair so the overlay
		// assertion is not polluted by the queries other subtests
		// issued. The raw user phrase "palm springs ca" MUST be
		// rewritten into the canonical "Palm Springs, California"
		// before the stub provider sees it; if the overlay regressed,
		// `lastQuery` would not match.
		overlayProv := &stubLocationProvider{byQuery: map[string][]microtools.LocationCandidate{
			"Palm Springs, California": {
				{Name: "Palm Springs", Admin1: "California", Country: "United States", Latitude: 33.83, Longitude: -116.55, Confidence: 1.0},
			},
		}}
		microtools.SetLocationServices(&microtools.LocationServices{
			Provider:          overlayProv,
			Cache:             microtools.NewLocationCache(5*time.Second, 8),
			AmbiguityFloor:    0.6,
			AmbiguityMaxCands: 5,
			Timeout:           2 * time.Second,
		})
		env := callTool(t, microtools.LocationNormalizeToolName, map[string]string{"input": "palm springs ca"})
		if env.Status != microtools.StatusResolved {
			t.Fatalf("overlay branch status=%q, want resolved", env.Status)
		}
		if overlayProv.lastQuery != "Palm Springs, California" {
			t.Fatalf("overlay rule regressed: provider saw %q, want %q (raw colloquial input leaked into geocoder query)",
				overlayProv.lastQuery, "Palm Springs, California")
		}
	})

	t.Run("SCN-065-A04_unit_convert_cups_to_grams", func(t *testing.T) {
		env := callTool(t, microtools.UnitConvertToolName, map[string]any{
			"value": 3.0, "from": "cup", "to": "g", "substance": "flour",
		})
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status=%q, want resolved (env=%+v)", env.Status, env)
		}
		gramsAny, ok := env.Value["value"]
		if !ok {
			t.Fatalf("value.value missing; env=%+v", env)
		}
		grams, ok := gramsAny.(float64)
		if !ok {
			t.Fatalf("value.value not numeric: %T %v", gramsAny, gramsAny)
		}
		// 3 US legal cups (720ml) of flour ≈ 360g via the standard
		// flour density bridge. Allow a generous tolerance because
		// the catalog rounds to significant figures.
		if grams < 300 || grams > 420 {
			t.Errorf("flour cups→grams=%v out of expected 300..420 band", grams)
		}
	})

	t.Run("SCN-065-A05_calculator_rejects_identifier", func(t *testing.T) {
		env := callTool(t, microtools.CalculatorToolName, map[string]string{"expression": "abs(-3) + 1"})
		if env.Status != microtools.StatusFailed {
			t.Fatalf("calculator MUST refuse identifier/function calls; got status=%q env=%+v", env.Status, env)
		}
		if env.Error == nil || strings.TrimSpace(env.Error.Code) == "" {
			t.Errorf("failed envelope MUST carry Error.Code; got %+v", env.Error)
		}
	})

	t.Run("SCN-065-A06_entity_resolve_resolved", func(t *testing.T) {
		entResolver.candidates = []microtools.EntityCandidate{
			{ArtifactID: "art-1", Label: "Apartment lease 2024", Score: 0.92, ArtifactType: "document", Snippet: "signed 2024-06"},
			{ArtifactID: "art-2", Label: "Car lease 2023", Score: 0.55, ArtifactType: "document"},
		}
		env := callTool(t, microtools.EntityResolveToolName, map[string]any{
			"input": "the lease", "user_id": "u-076-3", "scope": "documents", "top_k": 5,
		})
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status=%q, want resolved (env=%+v)", env.Status, env)
		}
		if got := env.Value["artifact_id"]; got != "art-1" {
			t.Errorf("artifact_id=%v, want art-1", got)
		}
		if entResolver.lastUser != "u-076-3" {
			t.Errorf("user-scoping leaked: resolver saw user=%q, want u-076-3", entResolver.lastUser)
		}
	})

	t.Run("SCN-065-A06_entity_resolve_ambiguous", func(t *testing.T) {
		entResolver.candidates = []microtools.EntityCandidate{
			{ArtifactID: "art-3", Label: "Lease A", Score: 0.55, ArtifactType: "document"},
			{ArtifactID: "art-4", Label: "Lease B", Score: 0.50, ArtifactType: "document"},
		}
		env := callTool(t, microtools.EntityResolveToolName, map[string]any{
			"input": "lease", "user_id": "u-076-3", "scope": "documents", "top_k": 5,
		})
		if env.Status != microtools.StatusAmbiguous {
			t.Fatalf("status=%q, want ambiguous (env=%+v)", env.Status, env)
		}
		if len(env.Candidates) < 2 {
			t.Errorf("ambiguous envelope MUST surface multiple candidates; got %d", len(env.Candidates))
		}
	})
}
