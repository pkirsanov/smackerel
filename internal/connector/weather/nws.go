package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// NWS (National Weather Service) active alert client.
//
// The NWS API documents:
//
//	GET https://api.weather.gov/alerts/active?point={lat},{lon}
//
// returns a GeoJSON FeatureCollection. Each Feature.Properties carries the
// CAP-style alert payload Smackerel needs. Per NWS terms, every request must
// include a User-Agent that identifies the caller and a contact channel.
//
// CAP severity levels relevant here are Extreme, Severe, Moderate, Minor and
// Unknown. Smackerel maps these to its internal RawArtifact processing tiers
// so high-severity alerts get full enrichment while routine watches stay
// light.

// nwsBaseURLDefault is the public NWS active alerts endpoint. Tests override
// the per-client baseURL via NewNWSClient to point at httptest servers.
const nwsBaseURLDefault = "https://api.weather.gov"

// nwsRequestTimeout bounds a single fetch call. NWS occasionally takes
// several seconds under load; 30s matches enrichFetchTimeout in enrich.go so
// the two upstream paths share one bound.
const nwsRequestTimeout = 30 * time.Second

// nwsAlertCacheTTL controls how long a (lat,lon) alerts response is reused
// before refetching. Alerts are time-sensitive but a sync that polls every
// few minutes should not hammer the upstream — 5 minutes balances both.
const nwsAlertCacheTTL = 5 * time.Minute

// nwsMaxResponseSize caps decoded body size to avoid OOM on a hostile
// upstream. A typical active-alerts payload is well under 256 KiB; 2 MiB
// leaves headroom for events with many features.
const nwsMaxResponseSize = 2 << 20

// nwsMaxErrorBodyDrain limits bytes drained from non-200 responses so the
// connection can be reused without consuming an unbounded error body.
const nwsMaxErrorBodyDrain = 1 << 16

// nwsLatMin/Max and nwsLonMin/Max are the absolute legal bounds.
// FetchActiveAlerts rejects requests outside these before issuing a network
// call. NWS itself only covers US territory; non-US points return an empty
// feature list, which is harmless and surfaces as zero alerts.
const (
	nwsLatMin = -90.0
	nwsLatMax = 90.0
	nwsLonMin = -180.0
	nwsLonMax = 180.0
)

// CAP severity strings as emitted by NWS.
const (
	CAPSeverityExtreme  = "Extreme"
	CAPSeveritySevere   = "Severe"
	CAPSeverityModerate = "Moderate"
	CAPSeverityMinor    = "Minor"
	CAPSeverityUnknown  = "Unknown"
)

// Smackerel ProcessingTier values. These mirror the strings used elsewhere
// in the connector package (see internal/connector/discord/discord.go).
const (
	tierFull     = "full"
	tierStandard = "standard"
	tierLight    = "light"
)

// NWSAlert is the normalized representation of a single NWS active alert.
// Fields map directly to the NWS GeoJSON properties block.
type NWSAlert struct {
	ID          string    `json:"id"`
	Event       string    `json:"event"`
	Severity    string    `json:"severity"`
	Headline    string    `json:"headline"`
	Description string    `json:"description"`
	Instruction string    `json:"instruction"`
	AreaDesc    string    `json:"area_desc"`
	Effective   time.Time `json:"effective"`
	Expires     time.Time `json:"expires"`
}

// NWSClient fetches active alerts from the NWS public API. It is safe for
// concurrent use; the cache map is protected by mu.
type NWSClient struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string

	mu    sync.Mutex
	cache map[string]nwsCacheEntry
}

type nwsCacheEntry struct {
	alerts    []NWSAlert
	expiresAt time.Time
}

// NewNWSClient constructs a client. Pass an empty baseURL to use the public
// NWS endpoint. Pass an empty userAgent to use the package-level Smackerel
// user-agent constant defined in weather.go.
func NewNWSClient(baseURL, ua string) *NWSClient {
	if baseURL == "" {
		baseURL = nwsBaseURLDefault
	}
	if ua == "" {
		ua = userAgent
	}
	return &NWSClient{
		httpClient: &http.Client{
			Timeout: nwsRequestTimeout,
			// Refuse redirects to prevent SSRF via open-redirect, mirroring
			// the policy enforced by the Open-Meteo client in weather.go.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("nws client refuses redirect to %s", req.URL.Hostname())
			},
		},
		baseURL:   strings.TrimRight(baseURL, "/"),
		userAgent: ua,
		cache:     make(map[string]nwsCacheEntry),
	}
}

// FetchActiveAlerts returns the currently active NWS alerts for the given
// coordinate. The response is cached per (lat,lon) for nwsAlertCacheTTL so
// repeated polls in a single sync run do not duplicate upstream load.
//
// Returns an error only on network/decode failure. An empty slice with no
// error means the upstream returned no active alerts (the common case for
// non-US locations and for US locations under quiet weather).
func (c *NWSClient) FetchActiveAlerts(ctx context.Context, lat, lon float64) ([]NWSAlert, error) {
	if lat < nwsLatMin || lat > nwsLatMax {
		return nil, fmt.Errorf("nws: latitude %.4f out of range [%v, %v]", lat, nwsLatMin, nwsLatMax)
	}
	if lon < nwsLonMin || lon > nwsLonMax {
		return nil, fmt.Errorf("nws: longitude %.4f out of range [%v, %v]", lon, nwsLonMin, nwsLonMax)
	}

	cacheKey := fmt.Sprintf("%.4f,%.4f", lat, lon)
	c.mu.Lock()
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		alerts := entry.alerts
		c.mu.Unlock()
		return alerts, nil
	}
	c.mu.Unlock()

	url := fmt.Sprintf("%s/alerts/active?point=%.4f,%.4f", c.baseURL, lat, lon)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("nws: build request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/geo+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("nws: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, nwsMaxErrorBodyDrain))
		return nil, fmt.Errorf("nws: returned status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, nwsMaxResponseSize)
	var raw struct {
		Features []struct {
			Properties struct {
				ID          string `json:"id"`
				Event       string `json:"event"`
				Severity    string `json:"severity"`
				Headline    string `json:"headline"`
				Description string `json:"description"`
				Instruction string `json:"instruction"`
				AreaDesc    string `json:"areaDesc"`
				Effective   string `json:"effective"`
				Expires     string `json:"expires"`
			} `json:"properties"`
		} `json:"features"`
	}
	if err := json.NewDecoder(limited).Decode(&raw); err != nil {
		_, _ = io.Copy(io.Discard, limited)
		return nil, fmt.Errorf("nws: decode response: %w", err)
	}
	_, _ = io.Copy(io.Discard, limited)

	alerts := make([]NWSAlert, 0, len(raw.Features))
	for _, f := range raw.Features {
		p := f.Properties
		eff, _ := time.Parse(time.RFC3339, p.Effective)
		exp, _ := time.Parse(time.RFC3339, p.Expires)
		alerts = append(alerts, NWSAlert{
			ID:          p.ID,
			Event:       p.Event,
			Severity:    p.Severity,
			Headline:    p.Headline,
			Description: p.Description,
			Instruction: p.Instruction,
			AreaDesc:    p.AreaDesc,
			Effective:   eff,
			Expires:     exp,
		})
	}

	c.mu.Lock()
	c.cache[cacheKey] = nwsCacheEntry{alerts: alerts, expiresAt: time.Now().Add(nwsAlertCacheTTL)}
	c.mu.Unlock()

	slog.Debug("nws active alerts fetched", "lat", lat, "lon", lon, "count", len(alerts))
	return alerts, nil
}

// mapCAPSeverityToTier converts CAP severity strings to Smackerel
// ProcessingTier strings. Unknown / empty / Minor all collapse to "light"
// because the artifact is informational-only and should not consume full
// enrichment budget.
func mapCAPSeverityToTier(severity string) string {
	switch severity {
	case CAPSeverityExtreme, CAPSeveritySevere:
		return tierFull
	case CAPSeverityModerate:
		return tierStandard
	case CAPSeverityMinor:
		return tierLight
	default:
		return tierLight
	}
}

// isHighSeverity returns true for severities that warrant proactive
// notification on the alerts.notify subject.
func isHighSeverity(severity string) bool {
	return severity == CAPSeverityExtreme || severity == CAPSeveritySevere
}
