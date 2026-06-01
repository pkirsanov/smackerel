// Spec 065 SCOPE-2 — unit tests for location_normalize covering
// SCN-065-A01 (palm springs ca → California), SCN-065-A02 (sf → San
// Francisco), and SCN-065-A03 (springfield → ambiguous candidate
// list). Provider is stubbed because these tests assert the
// handler's contract (preprocessing, ranking, envelope shaping,
// cache behavior) — the open-meteo HTTP adapter is exercised by the
// SCOPE-2 integration tests.

package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type stubProvider struct {
	name string
	// byQuery returns a fixed candidate list for the exact normalized
	// query string. Unknown queries fall through to noResultErr.
	byQuery map[string][]LocationCandidate
	// lastQuery records the most recent Geocode call for assertions.
	lastQuery string
	// callCount counts Geocode invocations (for cache-hit assertion).
	callCount int
	// fail, when non-nil, is returned from Geocode.
	fail error
}

func (s *stubProvider) Name() string { return s.name }

func (s *stubProvider) Geocode(_ context.Context, query string) ([]LocationCandidate, error) {
	s.callCount++
	s.lastQuery = query
	if s.fail != nil {
		return nil, s.fail
	}
	if c, ok := s.byQuery[query]; ok {
		return c, nil
	}
	return nil, nil
}

func newTestServices(t *testing.T, prov *stubProvider) *LocationServices {
	t.Helper()
	return &LocationServices{
		Provider:          prov,
		Cache:             NewLocationCache(10*time.Second, 8),
		AmbiguityFloor:    0.6,
		AmbiguityMaxCands: 5,
		Timeout:           2 * time.Second,
	}
}

func callHandler(t *testing.T, input string) Envelope {
	t.Helper()
	in, err := json.Marshal(locationInput{Input: input})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	raw, err := handleLocationNormalize(context.Background(), in)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if err := ValidateEnvelopeBytes(raw); err != nil {
		t.Fatalf("handler returned envelope that fails ValidateEnvelopeBytes: %v\n%s", err, string(raw))
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	return env
}

// SCN-065-A01: "palm springs ca" → resolved Palm Springs, California.
func TestLocationNormalizeMapsOpenMeteoPalmSpringsCA(t *testing.T) {
	prov := &stubProvider{
		name: "open-meteo",
		byQuery: map[string][]LocationCandidate{
			"Palm Springs, California": {
				{Name: "Palm Springs", Admin1: "California", Country: "United States", Latitude: 33.83, Longitude: -116.55, Confidence: 1.0},
			},
		},
	}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	env := callHandler(t, "palm springs ca")
	if env.Status != StatusResolved {
		t.Fatalf("status = %q, want resolved (env=%+v)", env.Status, env)
	}
	if got := env.Value["admin1"]; got != "California" {
		t.Errorf("value.admin1 = %v, want California", got)
	}
	if got := env.Value["name"]; got != "Palm Springs" {
		t.Errorf("value.name = %v, want Palm Springs", got)
	}
	if env.Source.Provider != "open-meteo" || env.Source.Kind != SourceKindHTTPProvider {
		t.Errorf("source = %+v, want provider=open-meteo kind=http_provider", env.Source)
	}
	if prov.callCount != 1 {
		t.Errorf("provider Geocode call count = %d, want 1", prov.callCount)
	}
	if prov.lastQuery != "Palm Springs, California" {
		t.Errorf("preprocessor sent %q, want %q", prov.lastQuery, "Palm Springs, California")
	}
}

// SCN-065-A02: "sf" → resolved San Francisco, California.
func TestLocationNormalizeMapsSFToSanFrancisco(t *testing.T) {
	prov := &stubProvider{
		name: "open-meteo",
		byQuery: map[string][]LocationCandidate{
			"San Francisco, California": {
				{Name: "San Francisco", Admin1: "California", Country: "United States", Latitude: 37.77, Longitude: -122.42, Confidence: 1.0},
			},
		},
	}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	env := callHandler(t, "sf")
	if env.Status != StatusResolved {
		t.Fatalf("status = %q, want resolved (env=%+v)", env.Status, env)
	}
	name, _ := env.Value["name"].(string)
	if name == "" || name != "San Francisco" {
		t.Errorf("value.name = %v, want San Francisco", env.Value["name"])
	}
	if got := env.Value["admin1"]; got != "California" {
		t.Errorf("value.admin1 = %v, want California", got)
	}
}

// SCN-065-A03: "springfield" → ambiguous with a ranked candidate list
// (<=5), with no guessed top result.
func TestLocationNormalizeReturnsAmbiguousEnvelopeForSpringfield(t *testing.T) {
	prov := &stubProvider{
		name: "open-meteo",
		byQuery: map[string][]LocationCandidate{
			"springfield": {
				// Synthetic confidences reflect open-meteo's
				// "first result is barely better than second"
				// behavior for the literal "springfield" query.
				{Name: "Springfield", Admin1: "Illinois", Country: "United States", Latitude: 39.78, Longitude: -89.65, Confidence: 0.55},
				{Name: "Springfield", Admin1: "Missouri", Country: "United States", Latitude: 37.21, Longitude: -93.29, Confidence: 0.50},
				{Name: "Springfield", Admin1: "Massachusetts", Country: "United States", Latitude: 42.10, Longitude: -72.59, Confidence: 0.40},
			},
		},
	}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	env := callHandler(t, "springfield")
	if env.Status != StatusAmbiguous {
		t.Fatalf("status = %q, want ambiguous (env=%+v)", env.Status, env)
	}
	if len(env.Candidates) == 0 {
		t.Fatal("ambiguous envelope must have at least one candidate")
	}
	if len(env.Candidates) > 5 {
		t.Errorf("candidates len = %d, want <= 5", len(env.Candidates))
	}
	if env.Candidates[0].Rank != 1 {
		t.Errorf("first candidate rank = %d, want 1", env.Candidates[0].Rank)
	}
	// Distinguishing field must help the user pick.
	for i, c := range env.Candidates {
		if c.Distinguishing == "" {
			t.Errorf("candidate[%d].distinguishing is empty; user has no way to disambiguate", i)
		}
	}
	// Resolved value MUST NOT be set when ambiguous.
	if len(env.Value) != 0 {
		t.Errorf("ambiguous envelope leaked Value: %v", env.Value)
	}
}

// Cache hit: second identical call MUST NOT invoke the provider.
func TestLocationNormalizeCacheHitSkipsProvider(t *testing.T) {
	prov := &stubProvider{
		name: "open-meteo",
		byQuery: map[string][]LocationCandidate{
			"Austin, Texas": {
				{Name: "Austin", Admin1: "Texas", Country: "United States", Latitude: 30.27, Longitude: -97.74, Confidence: 1.0},
			},
		},
	}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	_ = callHandler(t, "austin tx")
	_ = callHandler(t, "austin tx")
	if prov.callCount != 1 {
		t.Errorf("provider call count = %d, want 1 (second call must be served from cache)", prov.callCount)
	}
}

// Provider error returns a failed envelope and is NOT cached.
func TestLocationNormalizeProviderErrorReturnsFailedEnvelopeNotCached(t *testing.T) {
	prov := &stubProvider{name: "open-meteo", fail: errors.New("boom")}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	env := callHandler(t, "paris")
	if env.Status != StatusFailed {
		t.Fatalf("status = %q, want failed (env=%+v)", env.Status, env)
	}
	if env.Error == nil || env.Error.Code != "provider_error" {
		t.Errorf("error = %+v, want code=provider_error", env.Error)
	}
	// Second call must invoke the provider again — failures must not be cached.
	prov.fail = nil
	prov.byQuery = map[string][]LocationCandidate{
		"paris": {{Name: "Paris", Country: "France", Latitude: 48.85, Longitude: 2.35, Confidence: 1.0}},
	}
	env2 := callHandler(t, "paris")
	if env2.Status != StatusResolved {
		t.Errorf("second call status = %q, want resolved (failed envelope must not be cached)", env2.Status)
	}
	if prov.callCount != 2 {
		t.Errorf("provider call count = %d, want 2", prov.callCount)
	}
}

// Empty input is rejected explicitly (no silent provider call).
func TestLocationNormalizeRejectsEmptyInput(t *testing.T) {
	prov := &stubProvider{name: "open-meteo"}
	SetLocationServices(newTestServices(t, prov))
	t.Cleanup(ResetLocationServicesForTest)

	in, _ := json.Marshal(locationInput{Input: "   "})
	if _, err := handleLocationNormalize(context.Background(), in); err == nil {
		t.Fatal("expected error for whitespace-only input, got nil")
	}
	if prov.callCount != 0 {
		t.Errorf("provider called %d times; whitespace-only input must short-circuit", prov.callCount)
	}
}

// Handler returns a real Go error when services are not wired so the
// trace surfaces the misconfiguration instead of fabricating a result.
func TestLocationNormalizeRequiresSetServices(t *testing.T) {
	ResetLocationServicesForTest()
	in, _ := json.Marshal(locationInput{Input: "paris"})
	_, err := handleLocationNormalize(context.Background(), in)
	if err == nil {
		t.Fatal("expected location_normalize_not_configured error, got nil")
	}
}

// Unit test for the preprocessing table: state-abbrev + nicknames.
func TestNormalizeQueryRules(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"palm springs ca", "Palm Springs, California"},
		{"Austin TX", "Austin, Texas"},
		{"sf", "San Francisco, California"},
		{"SF", "San Francisco, California"},
		{"nyc", "New York"},
		{"Reykjavik", "Reykjavik"}, // unchanged — not in either table
		{"  paris  ", "paris"},     // outer ws trimmed, no other rewrite
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeQuery(tc.in); got != tc.want {
			t.Errorf("NormalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
