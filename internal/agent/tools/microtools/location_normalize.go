// Spec 065 SCOPE-2 — location_normalize micro-tool.
//
// location_normalize is the generic, reusable canonical-location
// resolver every scenario can call. It owns:
//   - colloquial preprocessing (US state-abbrev expansion, common
//     city nicknames) so downstream geocoders see provider-friendly
//     input;
//   - upstream geocoder dispatch via a pluggable Provider;
//   - bounded-candidate ambiguity envelopes (status="ambiguous") for
//     borderline inputs (e.g. "springfield"), so the assistant facade
//     can run a spec 061 disambiguation prompt instead of guessing;
//   - tool-local LRU cache keyed by (provider, normalized input).
//
// Wiring contract (mirrors weather): production code in cmd/core
// constructs a *Services (provider + cache + sst defaults) and calls
// SetServices once at startup. Until SetServices is called the handler
// returns a real Go error so the trace surfaces the misconfiguration
// immediately instead of silently emitting a fake "no result".
//
// Smackerel NO-DEFAULTS: every SST key consumed by the wiring layer
// is REQUIRED at config-load time (see internal/config/assistant_tools.go).
// This package neither reads env vars nor falls back to default
// providers; an unknown provider name MUST be rejected by the wiring
// layer before SetServices is called.

package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// LocationNormalizeToolName is the canonical tool name registered
// through the spec 037 agent registry.
const LocationNormalizeToolName = "location_normalize"

// LocationCandidate is the canonical-location shape every provider
// returns. The handler folds these into either a resolved Value or
// an ambiguous Candidates list on the shared Envelope.
type LocationCandidate struct {
	Name       string  `json:"name"`
	Admin1     string  `json:"admin1,omitempty"`
	Country    string  `json:"country"`
	Latitude   float64 `json:"lat"`
	Longitude  float64 `json:"lon"`
	Confidence float64 `json:"confidence"`
}

// LocationProvider is the minimum surface a canonical-location
// backend must implement. The provider is a pure adapter; caching is
// owned by LocationCache.
type LocationProvider interface {
	// Name returns a short stable identifier ("open-meteo")
	// embedded in Envelope.Source for attribution.
	Name() string
	// Geocode resolves a single normalized query string into a
	// ranked candidate list (highest confidence first). Returning
	// (nil, nil) means "provider had no result"; the handler maps
	// that to status="failed" with code="no_result".
	Geocode(ctx context.Context, query string) ([]LocationCandidate, error)
}

// LocationServices holds the runtime dependencies for the handler.
type LocationServices struct {
	Provider          LocationProvider
	Cache             *LocationCache
	AmbiguityFloor    float64       // when the top candidate's confidence is below this, mark ambiguous
	AmbiguityMaxCands int           // upper bound on candidates rendered to the user (>=1)
	Timeout           time.Duration // per-call deadline
}

var (
	locSvcMu sync.RWMutex
	locSvc   *LocationServices
)

// SetLocationServices wires the production location_normalize
// runtime. Pass nil to clear (test-only).
func SetLocationServices(s *LocationServices) {
	locSvcMu.Lock()
	defer locSvcMu.Unlock()
	locSvc = s
}

// ResetLocationServicesForTest clears the wired services. Test-only.
func ResetLocationServicesForTest() {
	locSvcMu.Lock()
	defer locSvcMu.Unlock()
	locSvc = nil
}

func loadLocationServices() (*LocationServices, error) {
	locSvcMu.RLock()
	defer locSvcMu.RUnlock()
	if locSvc == nil {
		return nil, errors.New("location_normalize_not_configured")
	}
	if locSvc.Provider == nil {
		return nil, errors.New("location_normalize_provider_not_configured")
	}
	if locSvc.Cache == nil {
		return nil, errors.New("location_normalize_cache_not_configured")
	}
	if locSvc.AmbiguityFloor < 0 || locSvc.AmbiguityFloor > 1 {
		return nil, fmt.Errorf("location_normalize_ambiguity_floor_out_of_range: %g", locSvc.AmbiguityFloor)
	}
	if locSvc.AmbiguityMaxCands < 1 {
		return nil, fmt.Errorf("location_normalize_ambiguity_max_cands_invalid: %d", locSvc.AmbiguityMaxCands)
	}
	if locSvc.Timeout <= 0 {
		return nil, fmt.Errorf("location_normalize_timeout_invalid: %s", locSvc.Timeout)
	}
	return locSvc, nil
}

// -------------------- schemas --------------------

var locationInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["input"],
  "properties": {
    "input": {"type": "string", "minLength": 1}
  }
}`)

// Output schema accepts the shared Envelope shape. We keep it
// permissive on the inner Value / Candidates members because the
// envelope contract is enforced by ValidateEnvelopeBytes in the
// handler.
var locationOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": true,
  "required": ["schema_version", "status", "source"],
  "properties": {
    "schema_version": {"type": "string"},
    "status":         {"type": "string", "enum": ["resolved", "ambiguous", "failed"]},
    "source":         {"type": "object"}
  }
}`)

// -------------------- registration --------------------

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             LocationNormalizeToolName,
		Description:      "Resolve a colloquial location string (e.g. \"palm springs ca\", \"sf\") to a canonical {name, admin1, country, lat, lon} envelope with provider attribution; returns status=ambiguous with a ranked candidate list when input is borderline.",
		InputSchema:      locationInputSchema,
		OutputSchema:     locationOutputSchema,
		SideEffectClass:  agent.SideEffectExternal,
		OwningPackage:    "internal/agent/tools/microtools",
		PerCallTimeoutMs: 2000,
		Handler:          handleLocationNormalize,
	})
}

// -------------------- handler --------------------

type locationInput struct {
	Input string `json:"input"`
}

func handleLocationNormalize(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadLocationServices()
	if err != nil {
		return nil, err
	}
	var in locationInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("location_normalize_bad_input: %w", err)
	}
	original := strings.TrimSpace(in.Input)
	if original == "" {
		return nil, errors.New("location_normalize_empty_input")
	}
	normalized := NormalizeQuery(original)

	providerName := svc.Provider.Name()
	if cached, ok := svc.Cache.Get(providerName, normalized); ok {
		return marshalEnvelope(cached)
	}

	callCtx, cancel := context.WithTimeout(ctx, svc.Timeout)
	defer cancel()

	candidates, err := svc.Provider.Geocode(callCtx, normalized)
	if err != nil {
		env := buildFailedEnvelope(providerName, "provider_error", err.Error())
		// do not cache failures so transient provider errors can recover next call
		return marshalEnvelope(env)
	}

	env := shapeEnvelope(providerName, candidates, svc.AmbiguityFloor, svc.AmbiguityMaxCands)
	svc.Cache.Put(providerName, normalized, env)
	return marshalEnvelope(env)
}

// shapeEnvelope folds a provider's candidate list into a validated
// Envelope. Decision rules:
//   - zero candidates                                  → failed (no_result)
//   - one candidate                                    → resolved
//   - top candidate ≥ ambiguityFloor AND
//     top.confidence is strictly greater than candidates[1] → resolved
//   - otherwise                                        → ambiguous
//
// Candidates is capped at maxCands.
func shapeEnvelope(provider string, cands []LocationCandidate, ambiguityFloor float64, maxCands int) Envelope {
	now := time.Now().UTC()
	src := Source{
		Provider:    provider,
		Kind:        SourceKindHTTPProvider,
		RetrievedAt: now,
		Attribution: "Data: " + provider,
	}
	if len(cands) == 0 {
		return Envelope{
			SchemaVersion: CurrentSchemaVersion,
			Status:        StatusFailed,
			Source:        src,
			Error: &Error{
				Code:    "no_result",
				Message: "geocoder returned no candidates",
			},
		}
	}
	top := cands[0]
	resolved := false
	if len(cands) == 1 {
		resolved = true
	} else if top.Confidence >= ambiguityFloor && top.Confidence > cands[1].Confidence {
		resolved = true
	}
	if resolved {
		return Envelope{
			SchemaVersion: CurrentSchemaVersion,
			Status:        StatusResolved,
			Value:         candidateValueMap(top),
			Confidence:    clamp01(top.Confidence),
			Source:        src,
		}
	}
	limit := maxCands
	if limit > len(cands) {
		limit = len(cands)
	}
	out := make([]Candidate, 0, limit)
	for i, c := range cands[:limit] {
		out = append(out, Candidate{
			Rank:           i + 1,
			Label:          candidateLabel(c),
			Value:          candidateValueMap(c),
			Confidence:     clamp01(c.Confidence),
			Distinguishing: candidateDistinguishing(c),
		})
	}
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusAmbiguous,
		Candidates:    out,
		Source:        src,
	}
}

func buildFailedEnvelope(provider, code, msg string) Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusFailed,
		Source: Source{
			Provider:    provider,
			Kind:        SourceKindHTTPProvider,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Data: " + provider,
		},
		Error: &Error{Code: code, Message: msg},
	}
}

func candidateValueMap(c LocationCandidate) map[string]any {
	v := map[string]any{
		"name":    c.Name,
		"country": c.Country,
		"lat":     c.Latitude,
		"lon":     c.Longitude,
	}
	if c.Admin1 != "" {
		v["admin1"] = c.Admin1
	}
	return v
}

func candidateLabel(c LocationCandidate) string {
	parts := []string{c.Name}
	if c.Admin1 != "" {
		parts = append(parts, c.Admin1)
	}
	if c.Country != "" {
		parts = append(parts, c.Country)
	}
	return strings.Join(parts, ", ")
}

func candidateDistinguishing(c LocationCandidate) string {
	if c.Admin1 != "" && c.Country != "" {
		return c.Admin1 + ", " + c.Country
	}
	if c.Country != "" {
		return c.Country
	}
	return ""
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

func marshalEnvelope(env Envelope) (json.RawMessage, error) {
	if err := ValidateEnvelope(env); err != nil {
		return nil, fmt.Errorf("location_normalize_envelope_invalid: %w", err)
	}
	return json.Marshal(env)
}
