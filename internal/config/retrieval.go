// Package config — Spec 095 SCOPE-01: Retrieval-Strategy Routing +
// Freshness-Aware Retrieval SST.
//
// RetrievalConfig governs the top-level `retrieval.*` block in
// config/smackerel.yaml. Every field originates there and flows through
// scripts/commands/config.sh into the generated env file as RETRIEVAL_*
// variables. There are NO in-source defaults (Gate G028,
// smackerel-no-defaults): every key is REQUIRED at startup and a missing,
// empty, out-of-range, or (for contracts) unparseable value aborts Load()
// fail-loud with the [F095-SST-MISSING] prefix.
//
// Unlike spec 064's open_knowledge block, retrieval validation is
// UNCONDITIONAL — it does NOT short-circuit on Enabled=false. The routing
// and evergreen behaviours operate over the SINGLE existing pgvector +
// knowledge-graph + structured store (Principle 5); these keys only select a
// read-path strategy and a lifecycle weighting, they never create a store.
//
// Design source: specs/095-retrieval-strategy-routing/design.md §10 +
// scopes.md SCOPE-01.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Closed query-shape vocabulary (Idea 3, R7/R8). A RetrievalContract may only
// declare shapes from this set; an unrecognized shape in SST is rejected at
// startup. The typed routing layer (spec 095 SCOPE-02/03) re-references these
// canonical strings so the SST and the in-code registry never drift.
const (
	QueryShapeWholeDocumentSummary = "whole_document_summary"
	QueryShapeAggregateSpend       = "aggregate_spend"
	QueryShapeDossier              = "dossier"
	QueryShapeVagueRecall          = "vague_recall"
)

// allQueryShapes is the closed vocabulary used by SST validation.
var allQueryShapes = map[string]struct{}{
	QueryShapeWholeDocumentSummary: {},
	QueryShapeAggregateSpend:       {},
	QueryShapeDossier:              {},
	QueryShapeVagueRecall:          {},
}

// Closed evergreen judgment-source vocabulary (Idea 2, design §6). `scenario`
// is the canonical LLM-driven judgment; `tier_signals` is the deterministic
// fallback judged purely from the existing TierSignals.
const (
	EvergreenJudgmentScenario    = "scenario"
	EvergreenJudgmentTierSignals = "tier_signals"
)

// RetrievalConfig is the SST surface for spec 095 SCOPE-01.
type RetrievalConfig struct {
	Routing   RetrievalRoutingConfig
	Evergreen RetrievalEvergreenConfig
}

// RetrievalRoutingConfig holds the Idea 1 / Idea 3 routing SST.
type RetrievalRoutingConfig struct {
	// Enabled gates the retrieval-strategy router. When false, callers keep
	// the existing single §9.2 vector+graph+rerank path; the keys are still
	// REQUIRED so the contract is explicit (NO-DEFAULTS).
	Enabled bool

	// IntentConfidenceThreshold is the floor on CompiledIntent.Confidence
	// below which the router falls back to vague_recall (R5). MUST be in
	// (0, 1].
	IntentConfidenceThreshold float64

	// Per-strategy enablement (Idea 1a/1b/1c).
	WholeDocumentEnabled       bool
	StructuredAggregateEnabled bool
	// VagueRecallEnabled is structurally pinned true — the router's safe
	// fallback MUST always exist. Validation rejects false with a named
	// error (design §10).
	VagueRecallEnabled bool

	// Contracts is the parsed per-artifact-type admissible query-shape map
	// (Idea 3, R7/R8). Parsed from the single-line JSON object in SST. Each
	// value is a non-empty list drawn from the closed QueryShape vocabulary.
	// A type absent here resolves to [vague_recall] at the registry layer
	// (R9 fail-safe; spec 095 SCOPE-02). Keyed by lowercase artifact type.
	Contracts map[string][]string
}

// RetrievalEvergreenConfig holds the Idea 2 evergreen-signal SST. Following
// the cooling.go precedent (docs §3.6), only OPERATIONAL bounds live here;
// the evergreen-vs-ephemeral JUDGMENT is scenario-driven (or the deterministic
// tier_signals fallback) — never a hardcoded Go cutoff.
type RetrievalEvergreenConfig struct {
	Enabled bool
	// JudgmentSource selects the canonical scenario judgment or the
	// deterministic tier_signals fallback. Closed vocabulary.
	JudgmentSource string
	// ConfidenceFloor is the operational decision-confidence safety gate
	// (NOT a business cutoff). MUST be in [0, 1].
	ConfidenceFloor float64
	// PerTickBudget caps evergreen judgments per ingestion tick (NFR-2
	// throughput bound). MUST be > 0.
	PerTickBudget int
	// DedupWindowDays is the re-judge dedup window. MUST be > 0.
	DedupWindowDays int
	// Pool-eligibility switches (R12).
	SynthesisExcludesLowEvergreen bool
	DigestExcludesLowEvergreen    bool
}

// LoadRetrieval reads every RETRIEVAL_* env var and returns a populated,
// validated RetrievalConfig. Missing env vars (LookupEnv == false), empty
// values, out-of-range numbers, an unrecognized judgment_source, an
// unrecognized query shape, vague_recall disabled, or unparseable contracts
// JSON are ALL fail-loud errors. Validation is unconditional (no Enabled
// short-circuit) per design §10.
func LoadRetrieval() (RetrievalConfig, error) {
	var cfg RetrievalConfig
	var errs []string

	mustBool := func(key string, dst *bool) {
		v, ok := os.LookupEnv(key)
		if !ok {
			errs = append(errs, key+" (env var not set)")
			return
		}
		switch v {
		case "true":
			*dst = true
		case "false":
			*dst = false
		default:
			errs = append(errs, fmt.Sprintf("%s (must be exactly %q or %q, got %q)", key, "true", "false", v))
		}
	}
	mustString := func(key string, dst *string) {
		v, ok := os.LookupEnv(key)
		if !ok {
			errs = append(errs, key+" (env var not set)")
			return
		}
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (empty)")
			return
		}
		*dst = v
	}
	mustFloatRange := func(key string, dst *float64, lo, hi float64, loInclusive bool) {
		v, ok := os.LookupEnv(key)
		if !ok {
			errs = append(errs, key+" (env var not set)")
			return
		}
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (empty)")
			return
		}
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be a float, got %q)", key, v))
			return
		}
		loOK := f > lo || (loInclusive && f == lo)
		if !loOK || f > hi {
			bound := "("
			if loInclusive {
				bound = "["
			}
			errs = append(errs, fmt.Sprintf("%s (must be in %s%g, %g], got %g)", key, bound, lo, hi, f))
			return
		}
		*dst = f
	}
	mustIntMin := func(key string, dst *int, minVal int) {
		v, ok := os.LookupEnv(key)
		if !ok {
			errs = append(errs, key+" (env var not set)")
			return
		}
		if strings.TrimSpace(v) == "" {
			errs = append(errs, key+" (empty)")
			return
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s (must be an integer, got %q)", key, v))
			return
		}
		if n < minVal {
			errs = append(errs, fmt.Sprintf("%s (must be >= %d, got %d)", key, minVal, n))
			return
		}
		*dst = n
	}

	// --- retrieval.routing.* ---
	mustBool("RETRIEVAL_ROUTING_ENABLED", &cfg.Routing.Enabled)
	mustFloatRange("RETRIEVAL_ROUTING_INTENT_CONFIDENCE_THRESHOLD", &cfg.Routing.IntentConfidenceThreshold, 0, 1, false)
	mustBool("RETRIEVAL_ROUTING_STRATEGY_WHOLE_DOCUMENT_ENABLED", &cfg.Routing.WholeDocumentEnabled)
	mustBool("RETRIEVAL_ROUTING_STRATEGY_STRUCTURED_AGGREGATE_ENABLED", &cfg.Routing.StructuredAggregateEnabled)
	mustBool("RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED", &cfg.Routing.VagueRecallEnabled)

	// vague_recall is the router's safe fallback; it MUST always be enabled.
	if _, ok := os.LookupEnv("RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED"); ok && !cfg.Routing.VagueRecallEnabled {
		errs = append(errs, "RETRIEVAL_ROUTING_STRATEGY_VAGUE_RECALL_ENABLED (must be true — the router's safe fallback cannot be disabled)")
	}

	// --- retrieval.routing.contracts (JSON object) ---
	cfg.Routing.Contracts = parseRetrievalContracts("RETRIEVAL_ROUTING_CONTRACTS", &errs)

	// --- retrieval.evergreen.* ---
	mustBool("RETRIEVAL_EVERGREEN_ENABLED", &cfg.Evergreen.Enabled)
	mustString("RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE", &cfg.Evergreen.JudgmentSource)
	switch cfg.Evergreen.JudgmentSource {
	case "", EvergreenJudgmentScenario, EvergreenJudgmentTierSignals:
		// "" already reported by mustString; valid values pass.
	default:
		errs = append(errs, fmt.Sprintf("RETRIEVAL_EVERGREEN_JUDGMENT_SOURCE (must be %q or %q, got %q)",
			EvergreenJudgmentScenario, EvergreenJudgmentTierSignals, cfg.Evergreen.JudgmentSource))
	}
	mustFloatRange("RETRIEVAL_EVERGREEN_CONFIDENCE_FLOOR", &cfg.Evergreen.ConfidenceFloor, 0, 1, true)
	mustIntMin("RETRIEVAL_EVERGREEN_PER_TICK_BUDGET", &cfg.Evergreen.PerTickBudget, 1)
	mustIntMin("RETRIEVAL_EVERGREEN_DEDUP_WINDOW_DAYS", &cfg.Evergreen.DedupWindowDays, 1)
	mustBool("RETRIEVAL_EVERGREEN_POOLS_SYNTHESIS_EXCLUDES_LOW_EVERGREEN", &cfg.Evergreen.SynthesisExcludesLowEvergreen)
	mustBool("RETRIEVAL_EVERGREEN_POOLS_DIGEST_EXCLUDES_LOW_EVERGREEN", &cfg.Evergreen.DigestExcludesLowEvergreen)

	if len(errs) > 0 {
		return RetrievalConfig{}, fmt.Errorf("[F095-SST-MISSING] missing or invalid required retrieval configuration: %s", strings.Join(errs, ", "))
	}
	return cfg, nil
}

// parseRetrievalContracts reads the single-line JSON contracts object and
// validates every declared shape against the closed QueryShape vocabulary and
// that every contract is a non-empty list. Errors are appended to errs; the
// returned map is nil on any error (the joined error is fail-loud at the
// caller). The map is normalized to lowercase artifact-type keys.
func parseRetrievalContracts(key string, errs *[]string) map[string][]string {
	raw, ok := os.LookupEnv(key)
	if !ok {
		*errs = append(*errs, key+" (env var not set)")
		return nil
	}
	if strings.TrimSpace(raw) == "" {
		*errs = append(*errs, key+" (empty)")
		return nil
	}
	var parsed map[string][]string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		*errs = append(*errs, fmt.Sprintf("%s (invalid JSON object: %v)", key, err))
		return nil
	}
	if len(parsed) == 0 {
		*errs = append(*errs, key+" (must declare at least one artifact-type contract)")
		return nil
	}
	out := make(map[string][]string, len(parsed))
	for typ, shapes := range parsed {
		t := strings.ToLower(strings.TrimSpace(typ))
		if t == "" {
			*errs = append(*errs, key+" (empty artifact-type key)")
			continue
		}
		if len(shapes) == 0 {
			*errs = append(*errs, fmt.Sprintf("%s.%s (must declare at least one query shape)", key, t))
			continue
		}
		for _, s := range shapes {
			if _, valid := allQueryShapes[s]; !valid {
				*errs = append(*errs, fmt.Sprintf("%s.%s (unknown query shape %q; closed vocabulary is %s|%s|%s|%s)",
					key, t, s, QueryShapeWholeDocumentSummary, QueryShapeAggregateSpend, QueryShapeDossier, QueryShapeVagueRecall))
			}
		}
		out[t] = shapes
	}
	return out
}
