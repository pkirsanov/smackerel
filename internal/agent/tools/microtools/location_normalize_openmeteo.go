// Spec 065 SCOPE-2 — Open-Meteo geocoding adapter for
// location_normalize.
//
// The Open-Meteo geocoding API (https://open-meteo.com/en/docs/geocoding-api)
// is key-less and returns a ranked list of candidates with admin1
// (state/region) and country. We map its response 1:1 onto
// LocationCandidate; ranking is preserved as the provider returned it.
//
// Confidence: the geocoder does not return a per-result confidence
// score. We synthesize one as 1/(rank+1) (rank starts at 0) so the
// envelope-shaping logic in location_normalize.go can apply the
// ambiguity floor uniformly across providers. This means the top
// result has confidence 1.0 and the second 0.5, which encodes the
// "decisively better" semantic that shapeEnvelope expects.
//
// HTTP behavior: the adapter takes a *http.Client whose Timeout is
// the per-call deadline. The wiring layer constructs it from the SST
// ASSISTANT_TOOLS_LOCATION_NORMALIZE_TIMEOUT_MS value (no defaults).

package microtools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// OpenMeteoGeocoder is the Open-Meteo LocationProvider.
type OpenMeteoGeocoder struct {
	httpClient *http.Client
	geocodeURL string
}

// NewOpenMeteoGeocoder constructs the adapter. Both inputs are
// REQUIRED; an empty geocodeURL or nil httpClient panics so a
// misconfigured wiring layer is caught at startup.
func NewOpenMeteoGeocoder(httpClient *http.Client, geocodeURL string) *OpenMeteoGeocoder {
	if httpClient == nil {
		panic("microtools.NewOpenMeteoGeocoder: httpClient must not be nil")
	}
	if geocodeURL == "" {
		panic("microtools.NewOpenMeteoGeocoder: geocodeURL must not be empty")
	}
	return &OpenMeteoGeocoder{httpClient: httpClient, geocodeURL: geocodeURL}
}

// Name returns the canonical provider identifier.
func (p *OpenMeteoGeocoder) Name() string { return "open-meteo" }

// openMeteoResp matches the geocoding-API JSON shape we consume.
type openMeteoResp struct {
	Results []struct {
		Name      string  `json:"name"`
		Admin1    string  `json:"admin1"`
		Country   string  `json:"country"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"results"`
}

// Geocode performs a single upstream call and returns the ranked
// candidate list, capped at 5 (the upper bound documented in the
// SCN-065-A03 design).
func (p *OpenMeteoGeocoder) Geocode(ctx context.Context, query string) ([]LocationCandidate, error) {
	q := url.Values{}
	q.Set("name", query)
	q.Set("count", "5")
	q.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.geocodeURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var g openMeteoResp
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, err
	}
	if len(g.Results) == 0 {
		return nil, nil
	}
	out := make([]LocationCandidate, 0, len(g.Results))
	sameNameAmbiguity := len(g.Results) > 1 &&
		strings.EqualFold(strings.TrimSpace(g.Results[0].Name), strings.TrimSpace(g.Results[1].Name))
	for i, r := range g.Results {
		confidence := 1.0 / float64(i+1)
		if sameNameAmbiguity {
			// The provider has no confidence field. Multiple top results with
			// the same place name but different regions are equally plausible;
			// retaining a rank-derived 1.0/0.5 split would falsely resolve one.
			confidence = 0.5
		}
		out = append(out, LocationCandidate{
			Name:       r.Name,
			Admin1:     r.Admin1,
			Country:    r.Country,
			Latitude:   r.Latitude,
			Longitude:  r.Longitude,
			Confidence: confidence,
		})
	}
	return out, nil
}
