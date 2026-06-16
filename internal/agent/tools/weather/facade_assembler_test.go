// Spec 061 SCOPE-07 — unit tests for the weather facade-side
// source-assembler adapter. Proves:
//
//   - happy path: parses weather-query-v1 Final, emits Body =
//     forecast_line and one Source{Kind: SourceExternalProvider,
//     Ref: ExternalProviderRef{ProviderName, RetrievedAt}}.
//   - RetrievedAt is the ORIGINAL provider timestamp parsed from
//     the scenario's RFC3339 string (design §5.2 — the assembler
//     does NOT substitute time.Now()).
//   - non-OK outcome returns zero-value SourceAssembly (provider
//     unavailable / tool-error paths route through the gate as
//     "no sources" and the BS-006 capture path takes over).
//   - empty Final returns zero-value SourceAssembly.
//   - malformed Final JSON returns zero-value SourceAssembly (the
//     gate then refuses the response for requires_provenance
//     weather_query).
//   - missing provider_name or retrieved_at returns zero-value
//     SourceAssembly (refuses to fabricate attribution).
//   - nil InvocationResult returns zero-value SourceAssembly
//     (defensive).
//   - constructor panics on non-positive sourcesMax.

package weather

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestNewFacadeAssembler_HappyPath_EmitsExternalProviderSource(t *testing.T) {
	asm := NewFacadeAssembler(8)
	ctx := context.Background()
	// RFC3339 timestamp from the upstream provider — the assembler
	// MUST preserve this verbatim (no time.Now() substitution).
	retrievedAt := time.Date(2026, 5, 28, 14, 3, 0, 0, time.UTC)
	result := &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"forecast_line":"Seattle: 12°C, light rain.","provider_name":"open-meteo","retrieved_at":"2026-05-28T14:03:00Z"}`),
	}
	got := asm(ctx, result)

	if got.Body != "Seattle: 12°C, light rain." {
		t.Errorf("Body = %q; want forecast_line verbatim", got.Body)
	}
	if got.OverflowCount != 0 {
		t.Errorf("OverflowCount = %d; want 0", got.OverflowCount)
	}
	if len(got.Sources) != 1 {
		t.Fatalf("len(Sources) = %d; want exactly 1 external_provider source", len(got.Sources))
	}
	src := got.Sources[0]
	if src.Kind != contracts.SourceExternalProvider {
		t.Errorf("Sources[0].Kind = %q; want %q", src.Kind, contracts.SourceExternalProvider)
	}
	if src.ID != "open-meteo" {
		t.Errorf("Sources[0].ID = %q; want provider name as canonical ID", src.ID)
	}
	if src.Title != "open-meteo" {
		t.Errorf("Sources[0].Title = %q; want provider name as title", src.Title)
	}
	ref, ok := src.Ref.(contracts.ExternalProviderRef)
	if !ok {
		t.Fatalf("Sources[0].Ref type = %T; want contracts.ExternalProviderRef", src.Ref)
	}
	if ref.ProviderName != "open-meteo" {
		t.Errorf("ExternalProviderRef.ProviderName = %q; want open-meteo", ref.ProviderName)
	}
	if !ref.RetrievedAt.Equal(retrievedAt) {
		t.Errorf("ExternalProviderRef.RetrievedAt = %s; want %s (original provider timestamp, NOT time.Now)", ref.RetrievedAt, retrievedAt)
	}
}

func TestNewFacadeAssembler_NonOKOutcome_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	cases := []agent.Outcome{
		agent.OutcomeToolError,
		agent.OutcomeUnknownIntent,
		agent.OutcomeAllowlistViolation,
	}
	for _, oc := range cases {
		t.Run(string(oc), func(t *testing.T) {
			got := asm(context.Background(), &agent.InvocationResult{
				Outcome: oc,
				Final:   []byte(`{"forecast_line":"x","provider_name":"p","retrieved_at":"2026-05-28T14:03:00Z"}`),
			})
			if got.Body != "" || len(got.Sources) != 0 || got.OverflowCount != 0 {
				t.Errorf("non-OK outcome %q: assembly = %+v; want zero value", oc, got)
			}
		})
	}
}

func TestNewFacadeAssembler_EmptyFinal_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: nil})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("empty Final: assembly = %+v; want zero value", got)
	}
}

func TestNewFacadeAssembler_MalformedFinal_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`not json at all`),
	})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("malformed Final: assembly = %+v; want zero value (gate refuses requires_provenance)", got)
	}
}

func TestNewFacadeAssembler_MissingProviderName_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	// provider_name is mandatory for attribution. If absent the
	// assembler MUST NOT fabricate one — it returns zero-value so
	// the provenance gate refuses (weather_query requires_provenance).
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"forecast_line":"Seattle clear","retrieved_at":"2026-05-28T14:03:00Z"}`),
	})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("missing provider_name: assembly = %+v; want zero value", got)
	}
}

func TestNewFacadeAssembler_MissingRetrievedAt_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	// retrieved_at is mandatory for attribution. Absent → zero
	// value so the gate refuses the response.
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"forecast_line":"Seattle clear","provider_name":"open-meteo"}`),
	})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("missing retrieved_at: assembly = %+v; want zero value", got)
	}
}

func TestNewFacadeAssembler_MissingForecastLine_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(`{"provider_name":"open-meteo","retrieved_at":"2026-05-28T14:03:00Z"}`),
	})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("missing forecast_line: assembly = %+v; want zero value", got)
	}
}

func TestNewFacadeAssembler_NilResult_ReturnsZeroValue(t *testing.T) {
	asm := NewFacadeAssembler(8)
	got := asm(context.Background(), nil)
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("nil result: assembly = %+v; want zero value (defensive)", got)
	}
}

func TestNewFacadeAssembler_PanicsOnZeroSourcesMax(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on sourcesMax <= 0; got no panic")
		}
	}()
	NewFacadeAssembler(0)
}

func TestNewFacadeAssembler_PanicsOnNegativeSourcesMax(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on negative sourcesMax; got no panic")
		}
	}()
	NewFacadeAssembler(-1)
}

// ---------------------------------------------------------------------
// Spec 094 — the facade is unchanged at the provenance gate: the
// additive current/daily/units fields ride through (ignored here) and
// attribution is still mandatory.
// ---------------------------------------------------------------------

// richWeatherFinal is a spec-094 tool Final with the structured blocks
// present. provider_name/retrieved_at are toggled by the callers.
const richWeatherFinal = `{` +
	`"forecast_line":"Barcelona, ES — clear, 18°C (feels 17°C)\nnext 1 days:\nThu 28: clear, 14–22°C, rain 10%, UV 5",` +
	`"current":{"condition":"clear","temp":18.4,"feels_like":17.1,"humidity_pct":55,"precip":0.2,"wind_speed":12.3,"wind_dir":"NE","uv_index":5,"sunrise":"07:12","sunset":"21:25"},` +
	`"daily":[{"date":"2026-05-28","condition":"clear","temp_max":22,"temp_min":14,"precip_prob_pct":10,"uv_index_max":5}],` +
	`"units":{"temperature":"°C","wind_speed":"km/h","precipitation":"mm"},` +
	`"provider_name":"open-meteo","retrieved_at":"2026-05-28T14:03:00Z"}`

// SCN-094-A09 (adversarial) — even with the rich current/daily/units
// blocks present, a Final missing provider_name MUST NOT assemble; the
// extra data does not trick the gate into fabricating attribution.
func TestFacade_MissingProvider_RefusesAssembly(t *testing.T) {
	asm := NewFacadeAssembler(8)
	noProvider := strings.Replace(richWeatherFinal, `"provider_name":"open-meteo",`, ``, 1)
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(noProvider),
	})
	if got.Body != "" || len(got.Sources) != 0 {
		t.Errorf("rich payload missing provider_name MUST refuse; got %+v", got)
	}
}

// Spec 094 — a complete rich Final assembles Body=forecast_line + one
// external-provider Source; the additive blocks are correctly ignored
// at the gate (not surfaced as Body or Sources).
func TestFacade_RichPayload_AssemblesBodyAndSource(t *testing.T) {
	asm := NewFacadeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   []byte(richWeatherFinal),
	})
	if !strings.HasPrefix(got.Body, "Barcelona, ES — clear") {
		t.Errorf("Body must be forecast_line verbatim; got %q", got.Body)
	}
	if len(got.Sources) != 1 {
		t.Fatalf("want exactly 1 provider Source; got %d", len(got.Sources))
	}
	ref, ok := got.Sources[0].Ref.(contracts.ExternalProviderRef)
	if !ok || ref.ProviderName != "open-meteo" {
		t.Errorf("Source must carry the open-meteo ExternalProviderRef; got %+v", got.Sources[0].Ref)
	}
}
