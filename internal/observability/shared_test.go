package observability

import (
	"strings"
	"testing"
)

// lookupFrom builds an injectable env lookup from a fixed map so the fail-loud
// contract is exercised deterministically without touching process env.
func lookupFrom(pairs map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := pairs[key]
		return v, ok
	}
}

func validContract() Config {
	return Config{
		OTLPTracesEndpoint:        "http://otel-collector:4317",
		OTLPLogsEndpoint:          "http://otel-collector:4318",
		MetricsScrapeLabelProduct: "smackerel",
	}
}

func TestValidate_AcceptsAllNonEmpty(t *testing.T) {
	if err := validContract().Validate(); err != nil {
		t.Fatalf("Validate() on a fully-populated contract must succeed, got: %v", err)
	}
}

func TestValidate_RejectsEmptyTracesEndpoint(t *testing.T) {
	c := validContract()
	c.OTLPTracesEndpoint = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() must fail loud when OTLP_TRACES_ENDPOINT is empty")
	}
	if !strings.Contains(err.Error(), EnvOTLPTracesEndpoint) {
		t.Fatalf("error must name %s, got: %v", EnvOTLPTracesEndpoint, err)
	}
}

func TestValidate_RejectsEmptyLogsEndpoint(t *testing.T) {
	c := validContract()
	c.OTLPLogsEndpoint = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() must fail loud when OTLP_LOGS_ENDPOINT is empty")
	}
	if !strings.Contains(err.Error(), EnvOTLPLogsEndpoint) {
		t.Fatalf("error must name %s, got: %v", EnvOTLPLogsEndpoint, err)
	}
}

func TestValidate_RejectsEmptyScrapeLabelProduct(t *testing.T) {
	c := validContract()
	c.MetricsScrapeLabelProduct = ""
	err := c.Validate()
	if err == nil {
		t.Fatal("Validate() must fail loud when METRICS_SCRAPE_LABEL_PRODUCT is empty")
	}
	if !strings.Contains(err.Error(), EnvMetricsScrapeLabelProduct) {
		t.Fatalf("error must name %s, got: %v", EnvMetricsScrapeLabelProduct, err)
	}
}

func TestValidate_RejectsWhitespaceOnly(t *testing.T) {
	c := validContract()
	c.OTLPTracesEndpoint = "   \t "
	if err := c.Validate(); err == nil {
		t.Fatal("Validate() must reject a whitespace-only value (trim-then-check), no silent acceptance")
	}
}

func TestFromLookup_AllPresentSucceeds(t *testing.T) {
	got, err := fromLookup(lookupFrom(map[string]string{
		EnvOTLPTracesEndpoint:        "http://otel-collector:4317",
		EnvOTLPLogsEndpoint:          "http://otel-collector:4318",
		EnvMetricsScrapeLabelProduct: "smackerel",
	}))
	if err != nil {
		t.Fatalf("fromLookup with all three vars present must succeed, got: %v", err)
	}
	if got != validContract() {
		t.Fatalf("fromLookup produced %+v, want %+v", got, validContract())
	}
}

func TestFromLookup_MissingOneFailsLoud(t *testing.T) {
	// OTLP_LOGS_ENDPOINT absent from the map entirely (unset).
	_, err := fromLookup(lookupFrom(map[string]string{
		EnvOTLPTracesEndpoint:        "http://otel-collector:4317",
		EnvMetricsScrapeLabelProduct: "smackerel",
	}))
	if err == nil {
		t.Fatal("fromLookup must fail loud when a contract var is unset — no default, no fallback")
	}
	if !strings.Contains(err.Error(), EnvOTLPLogsEndpoint) {
		t.Fatalf("error must name the missing var %s, got: %v", EnvOTLPLogsEndpoint, err)
	}
}

func TestFromLookup_EmptyOneFailsLoud(t *testing.T) {
	// METRICS_SCRAPE_LABEL_PRODUCT present but empty-string (the dev placeholder
	// shape) — under the enabled posture this MUST still fail loud.
	_, err := fromLookup(lookupFrom(map[string]string{
		EnvOTLPTracesEndpoint:        "http://otel-collector:4317",
		EnvOTLPLogsEndpoint:          "http://otel-collector:4318",
		EnvMetricsScrapeLabelProduct: "",
	}))
	if err == nil {
		t.Fatal("fromLookup must fail loud when a contract var is set-but-empty — no default, no fallback")
	}
	if !strings.Contains(err.Error(), EnvMetricsScrapeLabelProduct) {
		t.Fatalf("error must name the empty var %s, got: %v", EnvMetricsScrapeLabelProduct, err)
	}
}

// TestConstants_MatchKnbCanonicalNames pins the env-var identifiers to the knb
// spec-014 scope-03 canonical contract so a rename cannot silently drift away
// from what the knb adapter injects.
func TestConstants_MatchKnbCanonicalNames(t *testing.T) {
	cases := map[string]string{
		EnvOTLPTracesEndpoint:        "OTLP_TRACES_ENDPOINT",
		EnvOTLPLogsEndpoint:          "OTLP_LOGS_ENDPOINT",
		EnvMetricsScrapeLabelProduct: "METRICS_SCRAPE_LABEL_PRODUCT",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("env var constant drift: got %q, want %q (must match knb spec-014 scope-03)", got, want)
		}
	}
}
