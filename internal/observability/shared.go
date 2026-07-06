// Package observability implements smackerel's service-tier consumption of the
// knb spec-014 shared-observability instrumentation contract (scope 03).
//
// The home-lab shared observability stack (one Prometheus, one Grafana, one
// Tempo, one Loki, one OTLP collector) is owned by the knb adapter
// `shared/observability/home-lab/`. When an operator flips smackerel to the
// shared posture, the knb adapter (`smackerel/home-lab/apply.sh`) injects three
// env vars into smackerel's generated `app.env` bundle:
//
//	OTLP_TRACES_ENDPOINT          — OTLP/gRPC traces collector endpoint
//	OTLP_LOGS_ENDPOINT            — OTLP/gRPC logs collector endpoint
//	METRICS_SCRAPE_LABEL_PRODUCT  — the `product=` scrape label value
//
// This package reads and FAIL-LOUD validates those three values. It is the
// CONFIG CONTRACT only — it deliberately does NOT fork a second span exporter.
// smackerel already exports OTLP/gRPC spans through the mature pipeline in
// internal/assistant/tracing (spec 061 SCOPE-09a) and already exposes a
// Prometheus /metrics endpoint via internal/metrics + internal/api/router.go.
// Duplicating that machinery would fork a `done` subsystem and split the
// env-var contract (knb FINDING-014-03-1). Scope 03 therefore adds ONLY the
// three genuine gaps: the canonical 3-var contract, this fail-loud boot gate,
// and the com.bubbles.* container discovery labels.
//
// FAIL-LOUD: when the shared posture is active (OTEL_ENABLED=true) every value
// is REQUIRED. A missing OR empty value aborts startup — there is NO default
// and NO fallback (smackerel NO-DEFAULTS SST policy; knb spec 014 FR-002 /
// FR-013). The real endpoint strings live ONLY in the knb adapter params; this
// repo references env-var NAMES and an empty-string dev placeholder, so no real
// hostnames are committed here (spec 014 FR-013 / AC-009).
package observability

import (
	"fmt"
	"os"
	"strings"
)

// Env var names — the knb spec-014 scope-03 canonical instrumentation contract.
// These REPLACE smackerel's prior declared-but-not-consumed single
// OTEL_EXPORTER_ENDPOINT (operator decision option (a), knb scope-03 report).
const (
	// EnvOTLPTracesEndpoint is the OTLP/gRPC traces collector endpoint.
	EnvOTLPTracesEndpoint = "OTLP_TRACES_ENDPOINT"
	// EnvOTLPLogsEndpoint is the OTLP/gRPC logs collector endpoint.
	EnvOTLPLogsEndpoint = "OTLP_LOGS_ENDPOINT"
	// EnvMetricsScrapeLabelProduct is the `product=` Prometheus scrape label
	// value the shared stack scopes smackerel's metrics under. It is also the
	// SST source of the com.bubbles.product container label.
	EnvMetricsScrapeLabelProduct = "METRICS_SCRAPE_LABEL_PRODUCT"
)

// Config is the resolved shared-observability instrumentation contract for one
// smackerel service. Every field maps 1:1 to a knb-injected env var.
type Config struct {
	// OTLPTracesEndpoint is the OTLP/gRPC endpoint smackerel exports traces to.
	OTLPTracesEndpoint string
	// OTLPLogsEndpoint is the OTLP/gRPC endpoint smackerel exports logs to.
	OTLPLogsEndpoint string
	// MetricsScrapeLabelProduct is the `product=` label value attached to this
	// service's scraped metrics (and the com.bubbles.product container label).
	MetricsScrapeLabelProduct string
}

// Validate enforces the fail-loud contract: every field MUST be non-empty once
// trimmed of surrounding whitespace. It returns a named, actionable error on
// the first empty field. There is no default and no fallback (smackerel
// NO-DEFAULTS SST; knb spec 014 scope 03).
func (c Config) Validate() error {
	if err := requireNonEmpty(EnvOTLPTracesEndpoint, c.OTLPTracesEndpoint); err != nil {
		return err
	}
	if err := requireNonEmpty(EnvOTLPLogsEndpoint, c.OTLPLogsEndpoint); err != nil {
		return err
	}
	return requireNonEmpty(EnvMetricsScrapeLabelProduct, c.MetricsScrapeLabelProduct)
}

// FromEnv reads the three contract vars from the process environment and
// Validates them fail-loud. Callers use this when they want the contract
// sourced directly from the environment rather than from an already-parsed
// config struct.
func FromEnv() (Config, error) {
	return fromLookup(os.LookupEnv)
}

// fromLookup is the testable core: it builds a Config from an injectable env
// lookup and Validates it, so tests never mutate process-global environment
// state. Mirrors the wanderaide scope-05 from_lookup pattern.
func fromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key string) string {
		v, _ := lookup(key)
		return v
	}
	c := Config{
		OTLPTracesEndpoint:        get(EnvOTLPTracesEndpoint),
		OTLPLogsEndpoint:          get(EnvOTLPLogsEndpoint),
		MetricsScrapeLabelProduct: get(EnvMetricsScrapeLabelProduct),
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// requireNonEmpty returns an actionable fail-loud error naming the env var when
// value is unset/empty/whitespace-only. No default is ever substituted.
func requireNonEmpty(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf(
			"%s must be set to a non-empty value — the knb spec-014 scope-03 "+
				"shared-observability contract requires it when observability is "+
				"enabled (no default, no fallback; smackerel NO-DEFAULTS SST policy)",
			name,
		)
	}
	return nil
}
