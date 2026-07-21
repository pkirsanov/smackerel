package assistant

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/microtools"
	"github.com/smackerel/smackerel/internal/agent/tools/weather"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
)

func TestFacadeResolvedCompiledWeatherConsumesStructuredLocationAndReturnsSource(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	compiled := compiledWeatherIntent(intent.ActionExternalLookup, "Barcelona")
	locationCalls := 0
	weatherCalls := 0

	facade, executor, _ := newCompiledWeatherTestFacade(
		t,
		now,
		"what is the weather in Barcelona",
		compiled,
		func(_ context.Context, input string) (microtools.Envelope, error) {
			locationCalls++
			if input != "Barcelona" {
				t.Fatalf("location input = %q, want compiler slot Barcelona", input)
			}
			return resolvedLocationEnvelope(now, map[string]any{
				"name": "Barcelona", "admin1": "Catalonia", "country": "Spain",
				"lat": 41.3888, "lon": 2.159,
			}), nil
		},
		func(_ context.Context, location string, window weather.ForecastWindow) (weather.Forecast, error) {
			weatherCalls++
			if location != "Barcelona, Catalonia, Spain" {
				t.Fatalf("weather location = %q, want canonical resolved location", location)
			}
			if window != weather.WindowNow {
				t.Fatalf("weather window = %q, want %q", window, weather.WindowNow)
			}
			return weather.Forecast{
				ForecastLine: "Barcelona: 24 C, clear.",
				ProviderName: "open-meteo",
				RetrievedAt:  now.Add(-time.Minute),
			}, nil
		},
	)

	response, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "compiled-weather-user", Transport: "web",
		TransportMessageID: "compiled-weather-turn", Kind: contracts.KindText,
		Text: "what is the weather in Barcelona",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if locationCalls != 1 || weatherCalls != 1 {
		t.Fatalf("location/weather calls = %d/%d, want 1/1", locationCalls, weatherCalls)
	}
	if executor.invocations != 0 {
		t.Fatalf("generic executor invocations = %d, want 0", executor.invocations)
	}
	if response.Status != contracts.StatusCheckingWeather || response.CaptureRoute {
		t.Fatalf("weather response status/capture = %q/%t, want checking_weather/false", response.Status, response.CaptureRoute)
	}
	if response.Body == "" {
		t.Fatal("weather response body is empty")
	}
	if len(response.Sources) != 1 {
		t.Fatalf("weather sources = %d, want 1", len(response.Sources))
	}
	if response.Sources[0].Kind != contracts.SourceExternalProvider {
		t.Fatalf("weather source kind = %q, want %q", response.Sources[0].Kind, contracts.SourceExternalProvider)
	}
	ref, ok := response.Sources[0].Ref.(contracts.ExternalProviderRef)
	if !ok || ref.ProviderName != "open-meteo" || ref.RetrievedAt.IsZero() {
		t.Fatalf("weather source ref = %#v, want nonempty open-meteo attribution", response.Sources[0].Ref)
	}
}

func TestFacadeResolvedCompiledWeatherSourceFailuresCaptureSafely(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)

	t.Run("provider unavailable", func(t *testing.T) {
		facade, executor, _ := newCompiledWeatherTestFacade(
			t,
			now,
			"weather in Barcelona",
			compiledWeatherIntent(intent.ActionExternalLookup, "Barcelona"),
			func(_ context.Context, _ string) (microtools.Envelope, error) {
				return resolvedLocationEnvelope(now, map[string]any{
					"name": "Barcelona", "country": "Spain", "lat": 41.3888, "lon": 2.159,
				}), nil
			},
			func(_ context.Context, _ string, _ weather.ForecastWindow) (weather.Forecast, error) {
				return weather.Forecast{}, errors.New("weather provider unavailable")
			},
		)

		response, err := facade.Handle(context.Background(), contracts.AssistantMessage{
			UserID: "compiled-weather-error-user", Transport: "web",
			TransportMessageID: "compiled-weather-error-turn", Kind: contracts.KindText,
			Text: "weather in Barcelona",
		})
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		assertCompiledWeatherCapture(t, response, contracts.ErrProviderUnavailable)
		if executor.invocations != 0 {
			t.Fatalf("generic executor invocations = %d, want 0", executor.invocations)
		}
	})

	t.Run("invalid resolved location", func(t *testing.T) {
		weatherCalls := 0
		facade, executor, _ := newCompiledWeatherTestFacade(
			t,
			now,
			"weather in Barcelona",
			compiledWeatherIntent(intent.ActionExternalLookup, "Barcelona"),
			func(_ context.Context, _ string) (microtools.Envelope, error) {
				return resolvedLocationEnvelope(now, map[string]any{"country": "Spain"}), nil
			},
			func(_ context.Context, _ string, _ weather.ForecastWindow) (weather.Forecast, error) {
				weatherCalls++
				return weather.Forecast{}, nil
			},
		)

		response, err := facade.Handle(context.Background(), contracts.AssistantMessage{
			UserID: "compiled-weather-invalid-user", Transport: "web",
			TransportMessageID: "compiled-weather-invalid-turn", Kind: contracts.KindText,
			Text: "weather in Barcelona",
		})
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		assertCompiledWeatherCapture(t, response, contracts.ErrSlotMissing)
		if weatherCalls != 0 {
			t.Fatalf("weather calls = %d, want 0 for invalid canonical location", weatherCalls)
		}
		if executor.invocations != 0 {
			t.Fatalf("generic executor invocations = %d, want 0", executor.invocations)
		}
	})
}

func TestFacadeResolvedCompiledWeatherAmbiguousLocationDoesNotLookupBeforeSelection(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	weatherCalls := 0
	facade, executor, store := newCompiledWeatherTestFacade(
		t,
		now,
		"weather in Springfield",
		compiledWeatherIntent(intent.ActionClarify, "Springfield"),
		func(_ context.Context, input string) (microtools.Envelope, error) {
			if input != "Springfield" {
				t.Fatalf("location input = %q, want Springfield", input)
			}
			return ambiguousSpringfieldEnvelope(now), nil
		},
		func(_ context.Context, _ string, _ weather.ForecastWindow) (weather.Forecast, error) {
			weatherCalls++
			return weather.Forecast{}, nil
		},
	)

	response, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "compiled-weather-ambiguous-user", Transport: "web",
		TransportMessageID: "compiled-weather-ambiguous-turn", Kind: contracts.KindText,
		Text: "weather in Springfield",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if response.DisambiguationPrompt == nil || len(response.DisambiguationPrompt.Choices) < 2 {
		t.Fatalf("disambiguation prompt = %#v, want at least two choices", response.DisambiguationPrompt)
	}
	if weatherCalls != 0 {
		t.Fatalf("weather calls = %d, want 0 before location selection", weatherCalls)
	}
	if executor.invocations != 0 {
		t.Fatalf("generic executor invocations = %d, want 0", executor.invocations)
	}
	conversation, found, err := store.Load(context.Background(), "compiled-weather-ambiguous-user", "web")
	if err != nil || !found || conversation.PendingDisambig == nil {
		t.Fatalf("pending disambiguation missing: found=%t err=%v conversation=%+v", found, err, conversation)
	}
}

func newCompiledWeatherTestFacade(
	t *testing.T,
	now time.Time,
	text string,
	compiled intent.CompiledIntent,
	locationResolver IntentLocationResolver,
	weatherResolver IntentWeatherResolver,
) (*Facade, *stubExecutor, *memContextStore) {
	t.Helper()
	scenario := &agent.Scenario{ID: "weather_query"}
	executor := &stubExecutor{}
	store := newMemContextStore()
	facade := mustFacade(
		defaultFacadeConfig(now),
		&stubRouter{chosen: scenario, decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.99,
		}, ok: true},
		executor,
		mapRegistry{scenarios: map[string]*agent.Scenario{"weather_query": scenario}},
		newTestManifest(map[string]manifestEntry{"weather_query": {
			UserFacingLabel: "check weather", RequiresProvenance: true,
			EnableSSTKey: "assistant.skills.weather.enabled", Enabled: true,
		}}),
		store,
		&recordingAudit{},
	)
	facade.WithIntentCompiler(scriptedCompiler{byText: map[string]intent.CompiledIntent{text: compiled}})
	facade.compiledInteractions = &compiledInteractions{
		locationResolver: locationResolver,
		weatherResolver:  weatherResolver,
	}
	return facade, executor, store
}

func compiledWeatherIntent(action intent.ActionClass, location string) intent.CompiledIntent {
	hint := "weather_query"
	compiled := intent.CompiledIntent{
		Version: "v1", Language: "en", UserGoal: "check weather",
		ActionClass: action, SideEffectClass: intent.SideEffectExternalRead,
		ScenarioHint: &hint, Confidence: 0.99,
		Slots: map[string]any{"location": map[string]any{"raw": location}},
		SourcePolicy: intent.SourcePolicy{
			RequiresCitations:  true,
			AllowedSourceKinds: []string{"external_provider"},
		},
	}
	if action == intent.ActionClarify {
		compiled.MissingSlots = []string{"location_selection"}
	}
	return compiled
}

func resolvedLocationEnvelope(now time.Time, value map[string]any) microtools.Envelope {
	return microtools.Envelope{
		SchemaVersion: microtools.CurrentSchemaVersion,
		Status:        microtools.StatusResolved,
		Value:         value,
		Confidence:    1,
		Source: microtools.Source{
			Provider: "open-meteo", Kind: microtools.SourceKindHTTPProvider,
			RetrievedAt: now, Attribution: "Data: open-meteo",
		},
	}
}

func ambiguousSpringfieldEnvelope(now time.Time) microtools.Envelope {
	return microtools.Envelope{
		SchemaVersion: microtools.CurrentSchemaVersion,
		Status:        microtools.StatusAmbiguous,
		Candidates: []microtools.Candidate{
			{Rank: 1, Label: "Springfield, Illinois, United States", Value: map[string]any{
				"name": "Springfield", "admin1": "Illinois", "country": "United States",
				"lat": 39.7817, "lon": -89.6501,
			}, Confidence: 0.5},
			{Rank: 2, Label: "Springfield, Missouri, United States", Value: map[string]any{
				"name": "Springfield", "admin1": "Missouri", "country": "United States",
				"lat": 37.209, "lon": -93.2923,
			}, Confidence: 0.5},
		},
		Source: microtools.Source{
			Provider: "open-meteo", Kind: microtools.SourceKindHTTPProvider,
			RetrievedAt: now, Attribution: "Data: open-meteo",
		},
	}
}

func assertCompiledWeatherCapture(t *testing.T, response contracts.AssistantResponse, cause contracts.ErrorCause) {
	t.Helper()
	if response.Status != contracts.StatusSavedAsIdea || !response.CaptureRoute {
		t.Fatalf("failure status/capture = %q/%t, want saved_as_idea/true", response.Status, response.CaptureRoute)
	}
	if response.ErrorCause != cause {
		t.Fatalf("failure cause = %q, want %q", response.ErrorCause, cause)
	}
	if len(response.Sources) != 0 {
		t.Fatalf("failure sources = %d, want 0", len(response.Sources))
	}
}
