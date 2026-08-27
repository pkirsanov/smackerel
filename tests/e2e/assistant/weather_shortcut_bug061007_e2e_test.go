//go:build e2e

// BUG-061-007 — own-scenario E2E regression coverage.
//
// The defect: `/weather <loc>` was swallowed by the generic capture path and
// acknowledged as "saved as an idea" instead of dispatching to the weather
// skill. The fix added a deterministic Step 3.9 fast-path
// (facade.handleWeatherShortcut, reached through the WithWeatherLookup seam)
// that runs BEFORE any model involvement.
//
// WHY THIS TEST IS NOT VACUOUS.
// The e2e stack deliberately runs without a usable LLM, so every turn that
// reaches the model path returns status="unavailable" with
// error_cause="provider_unavailable" (measured: a generic open-ended turn on
// this stack returns exactly that). The weather fast-path is therefore the one
// text turn that can reach a terminal "answered" — precisely because it does
// not depend on the model. That asymmetry is the discriminator: if the
// fast-path regressed, `/weather` would fall through to the model path and
// return "unavailable", or fall into capture and return "saved_as_idea", and
// the first subtest would fail. A test that merely asserted "not saved_as_idea"
// would pass on a fully broken stack; this one asserts the positive outcome and
// pins the control alongside it.
//
// Live-stack inputs come from the SST-managed e2e environment. The only
// legitimate skip lives in the shared helper (CORE_EXTERNAL_URL unset = no live
// stack here); the bodies below contain no skip of their own.

package assistant_e2e

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// postWeatherProbe drives one text turn through the live route and returns the
// decoded v1 response.
func postWeatherProbe(t *testing.T, stack httpTurnLiveStack, idPrefix, text string) httpadapter.TurnResponse {
	t.Helper()
	turnID := idPrefix + time.Now().UTC().Format("20060102T150405.000000")
	resp, raw := postAssistantTurn(t, stack, httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               text,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/assistant/turn for %q: status = %d, want 200; body=%s", text, resp.StatusCode, string(raw))
	}
	var out httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response for %q: %v\nbody=%s", text, err, string(raw))
	}
	if !out.FacadeInvoked {
		t.Fatalf("facade_invoked = false for %q; the turn never reached the assistant pipeline, so nothing below would be meaningful; body=%s", text, string(raw))
	}
	return out
}

// TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured is the
// scenario-specific regression test for BUG-061-007, driven over the real HTTP
// ingress against the running stack.
func TestAssistantHTTPE2E_WeatherShortcutDispatchesAndIsNeverCaptured(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	weather := postWeatherProbe(t, stack, "e2e-bug061007-weather-", "/weather Paris")

	t.Run("weather_shortcut_reaches_a_terminal_answer", func(t *testing.T) {
		if weather.Status != string(contracts.StatusAnswered) {
			t.Fatalf("status = %q, want %q — the deterministic weather fast-path did not dispatch. "+
				"error_cause=%q body=%q. A regression that removes the Step 3.9 fast-path lands here, "+
				"because the turn then falls through to the model path (which is unavailable on this stack) "+
				"or into capture.",
				weather.Status, contracts.StatusAnswered, weather.ErrorCause, weather.Body)
		}
		if strings.TrimSpace(weather.ErrorCause) != "" {
			t.Errorf("error_cause = %q, want empty on a dispatched weather answer", weather.ErrorCause)
		}
	})

	t.Run("weather_shortcut_is_never_acknowledged_as_a_captured_idea", func(t *testing.T) {
		if weather.Status == string(contracts.StatusSavedAsIdea) {
			t.Errorf("status = %q — this is the exact defect BUG-061-007 fixed: a weather command rendered as a captured idea", weather.Status)
		}
		if weather.CaptureRoute {
			t.Errorf("capture_route = true for a /weather command; the shortcut must never route to capture")
		}
		if strings.Contains(strings.ToLower(weather.Body), "saved as an idea") {
			t.Errorf("body claims the command was saved as an idea: %q", weather.Body)
		}
	})

	t.Run("the_answer_carries_real_forecast_content", func(t *testing.T) {
		// Guards against a regression that returns a bare "answered" with an
		// empty or generic body, which would satisfy the status assertion while
		// delivering nothing.
		if !strings.Contains(weather.Body, "°C") {
			t.Errorf("body carries no temperature reading, so the answer is not a real forecast: %q", weather.Body)
		}
	})

	t.Run("control_a_generic_turn_does_not_produce_a_forecast", func(t *testing.T) {
		// Non-vacuity control. If EVERY turn on this stack returned a forecast,
		// the assertions above would prove nothing about the shortcut. This
		// pins that a forecast is specific to the weather fast-path.
		generic := postWeatherProbe(t, stack, "e2e-bug061007-control-", "some random musing about mesh routing")
		if strings.Contains(generic.Body, "°C") {
			t.Errorf("a generic turn also returned forecast-shaped content (%q); the weather assertions above are therefore not specific to the shortcut", generic.Body)
		}
		if generic.Status == string(contracts.StatusAnswered) && strings.Contains(generic.Body, "°C") {
			t.Errorf("control turn was answered with a forecast; the discriminator this test relies on has been lost")
		}
	})
}
