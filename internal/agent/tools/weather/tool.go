// Package weather registers the spec 061 SCOPE-03 `weather_lookup`
// agent tool. The tool delegates to a pluggable Provider (open-meteo
// in v1) and short-circuits identical lookups within the configured
// TTL via an in-process LRU cache that PRESERVES the original
// `retrieved_at` timestamp on cache hits (design §5.2).
//
// Per BS-009, the public surface is the provider Interface + Cache;
// concrete providers (open-meteo, accuweather, …) live in sibling
// files and are picked by SST (assistant.skills.weather.provider).
//
// Wiring contract: production code in cmd/core constructs a *Services
// (provider + cache + SST defaults) and calls SetServices once at
// startup. Until SetServices is called the handler returns a real Go
// error so the trace surfaces the misconfiguration immediately
// instead of returning a stale-looking empty forecast.
package weather

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

// ToolName is the single tool registered by this package.
const ToolName = "weather_lookup"

// ForecastWindow captures the time window the user asked about.
// "now" is the default current-conditions lookup; "today" / "tomorrow"
// / "weekend" map to provider-specific forecast horizons.
type ForecastWindow string

const (
	WindowNow      ForecastWindow = "now"
	WindowToday    ForecastWindow = "today"
	WindowTomorrow ForecastWindow = "tomorrow"
	WindowWeekend  ForecastWindow = "weekend"
)

// CurrentConditions is the provider-neutral "right now" snapshot for
// the resolved location (spec 094). Numeric values are expressed in the
// units named by Forecast.Units; they carry no embedded unit so a
// consumer renders them with the descriptor. WindDir is an 8-point
// compass abbreviation (e.g. "NE"); Sunrise/Sunset are local "HH:MM".
type CurrentConditions struct {
	Condition   string  `json:"condition"`
	Temp        float64 `json:"temp"`
	FeelsLike   float64 `json:"feels_like"`
	HumidityPct int     `json:"humidity_pct"`
	Precip      float64 `json:"precip"`
	WindSpeed   float64 `json:"wind_speed"`
	WindDir     string  `json:"wind_dir"`
	UVIndex     float64 `json:"uv_index"`
	Sunrise     string  `json:"sunrise"`
	Sunset      string  `json:"sunset"`
}

// DailyForecast is one provider-neutral daily-outlook row (spec 094).
// Date is the local calendar date "YYYY-MM-DD".
type DailyForecast struct {
	Date          string  `json:"date"`
	Condition     string  `json:"condition"`
	TempMax       float64 `json:"temp_max"`
	TempMin       float64 `json:"temp_min"`
	PrecipProbPct int     `json:"precip_prob_pct"`
	UVIndexMax    float64 `json:"uv_index_max"`
}

// ForecastUnits is the self-describing unit descriptor (display symbols)
// for the numeric fields on CurrentConditions and DailyForecast.
type ForecastUnits struct {
	Temperature   string `json:"temperature"`
	WindSpeed     string `json:"wind_speed"`
	Precipitation string `json:"precipitation"`
}

// Forecast is the provider-neutral result returned to the agent and
// cached by Cache. RetrievedAt is the wall-clock moment the upstream
// provider responded; it MUST be propagated unchanged through cache
// hits so the capability layer can render an accurate "as of …"
// attribution.
//
// Spec 094: Current/Daily/Units are ADDITIVE structured blocks that
// carry the full rich answer for machine consumers (the web/mobile
// frontend); ForecastLine remains the rendered, human-readable answer
// every transport displays. ForecastLine/ProviderName/RetrievedAt are
// preserved verbatim from spec 061 (name + meaning unchanged).
type Forecast struct {
	ForecastLine string            `json:"forecast_line"`
	Current      CurrentConditions `json:"current"`
	Daily        []DailyForecast   `json:"daily"`
	Units        ForecastUnits     `json:"units"`
	ProviderName string            `json:"provider_name"`
	RetrievedAt  time.Time         `json:"retrieved_at"`
}

// Provider is the minimum surface every concrete weather provider must
// implement. Implementations are pure adapters — they MUST NOT cache
// internally; caching is owned by the Cache passed to Services.
type Provider interface {
	// Name returns a short provider identifier (e.g. "open-meteo")
	// that is embedded in Forecast.ProviderName for attribution.
	Name() string
	// Lookup performs a single upstream call. Implementations are
	// responsible for parsing the upstream response into a Forecast
	// and stamping RetrievedAt at the moment the upstream responded.
	Lookup(ctx context.Context, location string, window ForecastWindow) (Forecast, error)
}

// Services holds the runtime dependencies required by the weather tool
// handler. Production wiring constructs one in cmd/core and calls
// SetServices once before the agent bridge starts dispatching.
type Services struct {
	// Provider is the concrete upstream adapter (open-meteo by default).
	Provider Provider
	// Cache is the in-process LRU; cache hits bypass Provider.Lookup
	// but PRESERVE the cached Forecast.RetrievedAt timestamp.
	Cache *Cache
}

var (
	servicesMu sync.RWMutex
	services   *Services
)

// SetServices wires the production weather runtime. Pass nil to
// clear (test-only).
func SetServices(s *Services) {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = s
}

// ResetForTest clears the wired services. Test-only.
func ResetForTest() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = nil
}

func loadServices() (*Services, error) {
	servicesMu.RLock()
	defer servicesMu.RUnlock()
	if services == nil {
		return nil, errors.New("weather_tools_not_configured")
	}
	if services.Provider == nil {
		return nil, errors.New("weather_tools_provider_not_configured")
	}
	if services.Cache == nil {
		return nil, errors.New("weather_tools_cache_not_configured")
	}
	return services, nil
}

// -------------------- schemas --------------------

var inputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["location"],
  "properties": {
    "location":        {"type": "string", "minLength": 1},
    "forecast_window": {"type": "string", "enum": ["now", "today", "tomorrow", "weekend"]}
  }
}`)

var outputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["forecast_line", "current", "daily", "units", "provider_name", "retrieved_at"],
  "properties": {
    "forecast_line": {"type": "string"},
    "provider_name": {"type": "string"},
    "retrieved_at":  {"type": "string"},
    "units": {
      "type": "object",
      "additionalProperties": false,
      "required": ["temperature", "wind_speed", "precipitation"],
      "properties": {
        "temperature":   {"type": "string"},
        "wind_speed":    {"type": "string"},
        "precipitation": {"type": "string"}
      }
    },
    "current": {
      "type": "object",
      "additionalProperties": false,
      "required": ["condition", "temp", "feels_like", "humidity_pct", "precip", "wind_speed", "wind_dir", "uv_index", "sunrise", "sunset"],
      "properties": {
        "condition":    {"type": "string"},
        "temp":         {"type": "number"},
        "feels_like":   {"type": "number"},
        "humidity_pct": {"type": "integer"},
        "precip":       {"type": "number"},
        "wind_speed":   {"type": "number"},
        "wind_dir":     {"type": "string"},
        "uv_index":     {"type": "number"},
        "sunrise":      {"type": "string"},
        "sunset":       {"type": "string"}
      }
    },
    "daily": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["date", "condition", "temp_max", "temp_min", "precip_prob_pct", "uv_index_max"],
        "properties": {
          "date":            {"type": "string"},
          "condition":       {"type": "string"},
          "temp_max":        {"type": "number"},
          "temp_min":        {"type": "number"},
          "precip_prob_pct": {"type": "integer"},
          "uv_index_max":    {"type": "number"}
        }
      }
    }
  }
}`)

// -------------------- registration --------------------

func init() {
	agent.RegisterTool(agent.Tool{
		Name:            ToolName,
		Description:     "Look up current conditions or a short-horizon forecast for a single location via the configured weather provider; cache hits preserve the original retrieved_at timestamp for accurate attribution.",
		InputSchema:     inputSchema,
		OutputSchema:    outputSchema,
		SideEffectClass: agent.SideEffectExternal,
		OwningPackage:   "internal/agent/tools/weather",
		// A single lookup is geocode + forecast, two sequential HTTPS
		// round trips to open-meteo. Measured worst case from the
		// self-hosted container is ~2s per call (so ~4s end-to-end on a
		// cold cache). The previous 2000 ms cap was tighter than a
		// single HTTP call and made /weather fail with
		// `provider_unavailable` on most cold-cache invocations. 8s
		// gives ~2x headroom over the observed worst case while still
		// failing fast if open-meteo or DNS is degraded.
		PerCallTimeoutMs: 8000,
		Handler:          handleWeatherLookup,
	})
}

// -------------------- handler --------------------

type weatherInput struct {
	Location       string `json:"location"`
	ForecastWindow string `json:"forecast_window,omitempty"`
}

type weatherOutput struct {
	ForecastLine string            `json:"forecast_line"`
	Current      CurrentConditions `json:"current"`
	Daily        []DailyForecast   `json:"daily"`
	Units        ForecastUnits     `json:"units"`
	ProviderName string            `json:"provider_name"`
	RetrievedAt  string            `json:"retrieved_at"`
}

func handleWeatherLookup(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	var in weatherInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("weather_lookup_bad_input: %w", err)
	}
	return LookupForecast(ctx, in.Location, ForecastWindow(in.ForecastWindow))
}

// LookupForecast performs a single deterministic weather lookup for an
// explicit location and window, reusing the exact provider + cache +
// attribution invariants of the weather_lookup agent tool. It is the
// shared core of the tool handler AND the capability-layer /weather
// shortcut fast-path (internal/assistant): an explicit `/weather <loc>`
// command dispatches here directly, so it never depends on the LLM
// emitting the weather_lookup tool call. An empty window defaults to
// "now". Returns the marshaled Forecast JSON (identical to the tool's
// output_schema) or a classified error (weather_tools_not_configured /
// weather_lookup_empty_location / weather_lookup_invalid_window /
// weather_lookup_provider_error).
func LookupForecast(ctx context.Context, location string, window ForecastWindow) (json.RawMessage, error) {
	svc, err := loadServices()
	if err != nil {
		return nil, err
	}
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, errors.New("weather_lookup_empty_location")
	}
	if window == "" {
		window = WindowNow
	}
	switch window {
	case WindowNow, WindowToday, WindowTomorrow, WindowWeekend:
		// ok
	default:
		return nil, fmt.Errorf("weather_lookup_invalid_window: %q", window)
	}

	providerName := svc.Provider.Name()
	if cached, ok := svc.Cache.Get(providerName, location, window); ok {
		// Cache hit — return the cached Forecast unchanged.
		// RetrievedAt is the ORIGINAL upstream response time, not now.
		return marshalForecast(cached)
	}

	fresh, err := svc.Provider.Lookup(ctx, location, window)
	if err != nil {
		return nil, fmt.Errorf("weather_lookup_provider_error: %w", err)
	}
	if fresh.ProviderName == "" {
		fresh.ProviderName = providerName
	}
	if fresh.RetrievedAt.IsZero() {
		fresh.RetrievedAt = time.Now().UTC()
	}
	svc.Cache.Put(providerName, location, window, fresh)
	return marshalForecast(fresh)
}

func marshalForecast(f Forecast) (json.RawMessage, error) {
	// Daily is always non-nil on the success path (forecast_days >= 1),
	// but coalesce a nil slice to an empty array so the strict
	// output_schema ("daily": {"type":"array"}) never sees a JSON null.
	daily := f.Daily
	if daily == nil {
		daily = []DailyForecast{}
	}
	return json.Marshal(weatherOutput{
		ForecastLine: f.ForecastLine,
		Current:      f.Current,
		Daily:        daily,
		Units:        f.Units,
		ProviderName: f.ProviderName,
		RetrievedAt:  f.RetrievedAt.UTC().Format(time.RFC3339),
	})
}
