package cardrewards

// Card-rewards rotating-category extraction (spec 083 Scope 05, design §4).
//
// This file is the Go ORCHESTRATOR for strict-schema LLM category extraction —
// the replacement for CCManager's regex scraper and its silent fallback to
// stale / placeholder categories. The actual model gateway call lives ONLY in
// the Python ML sidecar route POST /extract-card-categories (Constitution C2);
// this package never speaks to a model backend directly (NFR-CR-001/008). The
// orchestrator's job is the trustworthy half of the contract:
//
//  1. send the cleaned page text + candidate card/issuer/period to the sidecar
//     over the existing Go↔sidecar HTTP contract (mirrors
//     internal/agent/embedder/sidecar's POST /embed: Bearer auth + timeout);
//  2. validate the sidecar's JSON against a strict schema with
//     santhosh-tekuri/jsonschema/v6 as defense-in-depth (§17.2 / NFR-CR-003);
//  3. apply the failure-mode contract that the CCManager scraper got wrong:
//     a response that does not parse/validate, echoes the wrong card/period, or
//     names an unknown card is DISCARDED or SKIPPED — never stored, never
//     mismapped, and never used to overwrite an existing reconciled record;
//  4. persist validated observations + an extract audit run, and flag (not
//     overwrite) any existing reconciled record whose refresh failed.
//
// validateExtraction is a pure function of (raw, input, knownCard, threshold)
// so the adversarial scenarios (SCN-083-E01..E07) are unit-testable with no DB
// and no mocks; Run wires it to a real Store for the audited live-PG path
// (SCN-083-E08).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// extractionDateLayout is the strict period date format the sidecar MUST emit.
const extractionDateLayout = "2006-01-02"

// extractionResponseSchemaJSON is the strict output contract for the sidecar's
// extraction response (design §4). additionalProperties:false rejects extra
// keys; categories is a non-empty array of non-empty strings; confidence is a
// bounded number; spend_limit is a non-negative integer (whole dollars) or
// null. Date strings are shape-checked here and re-parsed in Go for the actual
// calendar validity check.
const extractionResponseSchemaJSON = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "additionalProperties": false,
  "required": [
    "card_id", "period_label", "period_start", "period_end",
    "categories", "spend_limit", "activation_required",
    "confidence", "source_evidence"
  ],
  "properties": {
    "card_id":             { "type": "string", "minLength": 1 },
    "period_label":        { "type": "string", "minLength": 1 },
    "period_start":        { "type": "string", "minLength": 1 },
    "period_end":          { "type": "string", "minLength": 1 },
    "categories":          { "type": "array", "minItems": 1, "items": { "type": "string", "minLength": 1 } },
    "spend_limit":         { "type": ["integer", "null"], "minimum": 0 },
    "activation_required": { "type": "boolean" },
    "confidence":          { "type": "number", "minimum": 0, "maximum": 1 },
    "source_evidence":     { "type": "string", "minLength": 1 }
  }
}`

// compiledExtractionSchema is compiled once at package init; a malformed schema
// constant is a developer error and panics immediately.
var compiledExtractionSchema = mustCompileExtractionSchema()

func mustCompileExtractionSchema() *jsonschema.Schema {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(extractionResponseSchemaJSON))
	if err != nil {
		panic(fmt.Sprintf("cardrewards: extraction schema is not valid JSON: %v", err))
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("extraction.json", doc); err != nil {
		panic(fmt.Sprintf("cardrewards: add extraction schema: %v", err))
	}
	sch, err := c.Compile("extraction.json")
	if err != nil {
		panic(fmt.Sprintf("cardrewards: compile extraction schema: %v", err))
	}
	return sch
}

// ExtractInput is one extraction request: refresh card CardID's PeriodLabel
// rotating categories from the cleaned text of one source page. CardID and
// PeriodLabel are the TARGET — the sidecar response MUST echo both, which is
// the orchestrator's mismap / prompt-injection defense (a response that names a
// different card or period is discarded, never stored under the target).
type ExtractInput struct {
	CardID      string
	IssuerHint  string
	PeriodLabel string
	SourceName  string
	SourceURL   string
	PageText    string
}

// ExtractRequest is the JSON body posted to the sidecar. PageText is carried in
// a dedicated DATA field — the orchestrator never assembles a prompt — so the
// sidecar route can treat page content strictly as data, not instructions
// (prompt-injection defense, §17.2 / SCN-083-E06).
type ExtractRequest struct {
	CardID      string `json:"card_id"`
	IssuerHint  string `json:"issuer_hint"`
	PeriodLabel string `json:"period_label"`
	SourceName  string `json:"source_name"`
	SourceURL   string `json:"source_url"`
	PageText    string `json:"page_text"`
}

// SidecarExtractor sends one extraction request to the model-gateway sidecar
// and returns the raw JSON response body. It is an interface so the orchestrator
// can be exercised against a deterministic fixture in tests (no live model)
// while production uses HTTPSidecarExtractor.
type SidecarExtractor interface {
	Extract(ctx context.Context, req ExtractRequest) (json.RawMessage, error)
}

// extractedRecord is the decoded sidecar response (design §4 contract).
type extractedRecord struct {
	CardID             string   `json:"card_id"`
	PeriodLabel        string   `json:"period_label"`
	PeriodStart        string   `json:"period_start"`
	PeriodEnd          string   `json:"period_end"`
	Categories         []string `json:"categories"`
	SpendLimit         *int     `json:"spend_limit"`
	ActivationRequired bool     `json:"activation_required"`
	Confidence         float64  `json:"confidence"`
	SourceEvidence     string   `json:"source_evidence"`
}

// decisionAction is the orchestrator's verdict for one extraction response.
type decisionAction int

const (
	// actionDiscard: response did not parse/validate or echoed the wrong
	// card/period. Store NOTHING; flag the target record needs_verification;
	// mark the run partial. This is the anti-silent-fallback path.
	actionDiscard decisionAction = iota
	// actionSkip: response is valid but names a card not in card_catalog.
	// Store NOTHING; record an audit note; do NOT mismap to a known card.
	actionSkip
	// actionStore: response is valid for the target known card. Store the
	// observation (BelowThreshold marks low-confidence for the reconciler).
	actionStore
)

// decision is the pure verdict for one response. Observation is set only for
// actionStore and carries the domain fields; the orchestrator stamps ID /
// ExtractionRunID / ObservedAt before persisting.
type decision struct {
	Action         decisionAction
	Observation    *RotatingCategoryObservation
	BelowThreshold bool
	Reason         string
}

// validateExtraction applies the design §4 validation contract to one raw
// sidecar response. It is PURE — a function of its inputs only — so every
// adversarial scenario is unit-testable without a DB or a live model. knownCard
// reports whether input.CardID exists in card_catalog (the orchestrator
// resolves this from the store; tests pass it directly).
func validateExtraction(raw json.RawMessage, in ExtractInput, knownCard bool, threshold float64) decision {
	// (1) Must parse as JSON and match the strict schema.
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return decision{Action: actionDiscard, Reason: "response is not valid JSON: " + err.Error()}
	}
	if err := compiledExtractionSchema.Validate(doc); err != nil {
		return decision{Action: actionDiscard, Reason: "response failed strict-schema validation: " + err.Error()}
	}

	var rec extractedRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return decision{Action: actionDiscard, Reason: "response decode failed: " + err.Error()}
	}

	// (2a) The response MUST echo the requested card and period. A mismatch is
	// a hallucination / prompt-injection attempt — discard, never mismap.
	if rec.CardID != in.CardID {
		return decision{Action: actionDiscard, Reason: fmt.Sprintf("response card_id %q does not match requested %q", rec.CardID, in.CardID)}
	}
	if rec.PeriodLabel != in.PeriodLabel {
		return decision{Action: actionDiscard, Reason: fmt.Sprintf("response period_label %q does not match requested %q", rec.PeriodLabel, in.PeriodLabel)}
	}

	// (2b) Dates must be real and ordered.
	start, err := time.Parse(extractionDateLayout, rec.PeriodStart)
	if err != nil {
		return decision{Action: actionDiscard, Reason: "period_start is not a YYYY-MM-DD date: " + rec.PeriodStart}
	}
	end, err := time.Parse(extractionDateLayout, rec.PeriodEnd)
	if err != nil {
		return decision{Action: actionDiscard, Reason: "period_end is not a YYYY-MM-DD date: " + rec.PeriodEnd}
	}
	if end.Before(start) {
		return decision{Action: actionDiscard, Reason: fmt.Sprintf("period_end %s precedes period_start %s", rec.PeriodEnd, rec.PeriodStart)}
	}

	// (4) An unknown card id is skipped with an audit note — never mismapped.
	if !knownCard {
		return decision{Action: actionSkip, Reason: fmt.Sprintf("card_id %q is not in card_catalog", rec.CardID)}
	}

	// Build the observation (domain fields only). spend_limit is whole dollars
	// on the page; the column is integer cents.
	activation := rec.ActivationRequired
	evidence := rec.SourceEvidence
	startCopy, endCopy := start, end
	obs := &RotatingCategoryObservation{
		CardCatalogID:      rec.CardID,
		PeriodLabel:        rec.PeriodLabel,
		PeriodStart:        &startCopy,
		PeriodEnd:          &endCopy,
		Categories:         rec.Categories,
		ActivationRequired: &activation,
		Confidence:         rec.Confidence,
		SourceName:         in.SourceName,
		SourceURL:          in.SourceURL,
		SourceEvidence:     &evidence,
	}
	if rec.SpendLimit != nil {
		cents := *rec.SpendLimit * 100
		obs.LimitCents = &cents
	}

	// (3) Low confidence is still a valid observation; it is stored and marked
	// so the reconciler (Scope 06) flags needs_verification.
	return decision{
		Action:         actionStore,
		Observation:    obs,
		BelowThreshold: rec.Confidence < threshold,
		Reason:         fmt.Sprintf("valid extraction (confidence %.2f, threshold %.2f)", rec.Confidence, threshold),
	}
}

// Extractor orchestrates a batch of extractions against the model-gateway
// sidecar and persists the audited result to PostgreSQL.
type Extractor struct {
	store     *Store
	sidecar   SidecarExtractor
	threshold float64
	logger    *slog.Logger
}

// NewExtractor constructs an Extractor. threshold is
// card_rewards.extraction.confidence_threshold (SST, fail-loud at config load).
// A nil logger is replaced with slog.Default so call sites need not guard.
func NewExtractor(store *Store, sidecar SidecarExtractor, threshold float64, logger *slog.Logger) *Extractor {
	if logger == nil {
		logger = slog.Default()
	}
	return &Extractor{store: store, sidecar: sidecar, threshold: threshold, logger: logger}
}

// ExtractResult summarizes a completed extraction batch for callers/tests.
type ExtractResult struct {
	Run           *CardRun
	Stored        int
	Skipped       int
	Discarded     int
	LowConfidence int
	Flagged       int
}

// Run executes the extraction batch: for each input it calls the sidecar,
// validates the response, and accumulates observations / verification flags,
// then persists one extract audit run + the observations atomically (design §4,
// SCN-083-E08). A sidecar transport error or an invalid response flags the
// target (card, period) record needs_verification — the existing reconciled
// record is preserved, never overwritten (SCN-083-E03). The run is `success`
// only when every input produced a stored observation; any discard/skip/error
// makes it `partial`.
func (e *Extractor) Run(ctx context.Context, inputs []ExtractInput, trigger string) (*ExtractResult, error) {
	if !ValidRunTrigger(trigger) {
		return nil, fmt.Errorf("cardrewards: invalid run trigger %q", trigger)
	}
	runID := uuid.NewString()
	started := time.Now().UTC()

	var observations []RotatingCategoryObservation
	flagSet := map[CardPeriodRef]struct{}{}
	res := &ExtractResult{}

	for _, in := range inputs {
		raw, err := e.sidecar.Extract(ctx, ExtractRequest{
			CardID:      in.CardID,
			IssuerHint:  in.IssuerHint,
			PeriodLabel: in.PeriodLabel,
			SourceName:  in.SourceName,
			SourceURL:   in.SourceURL,
			PageText:    in.PageText,
		})
		if err != nil {
			res.Discarded++
			flagSet[CardPeriodRef{CardCatalogID: in.CardID, PeriodLabel: in.PeriodLabel}] = struct{}{}
			e.logger.Warn("card-rewards extraction sidecar error — flagging target for verification",
				"card_id", in.CardID, "period", in.PeriodLabel, "source", in.SourceName, "error", err)
			continue
		}

		known, err := e.knownCard(ctx, in.CardID)
		if err != nil {
			return nil, fmt.Errorf("cardrewards: resolve card_id %q: %w", in.CardID, err)
		}

		d := validateExtraction(raw, in, known, e.threshold)
		switch d.Action {
		case actionStore:
			obs := *d.Observation
			obs.ID = uuid.NewString()
			obs.ExtractionRunID = runID
			obs.ObservedAt = time.Now().UTC()
			observations = append(observations, obs)
			res.Stored++
			if d.BelowThreshold {
				res.LowConfidence++
				e.logger.Info("card-rewards extraction below confidence threshold — reconciler will flag",
					"card_id", in.CardID, "period", in.PeriodLabel, "source", in.SourceName)
			}
		case actionSkip:
			res.Skipped++
			e.logger.Info("card-rewards extraction skipped unknown card — not mismapped",
				"card_id", in.CardID, "source", in.SourceName, "reason", d.Reason)
		case actionDiscard:
			res.Discarded++
			flagSet[CardPeriodRef{CardCatalogID: in.CardID, PeriodLabel: in.PeriodLabel}] = struct{}{}
			e.logger.Warn("card-rewards extraction discarded (invalid) — flagging target for verification",
				"card_id", in.CardID, "period", in.PeriodLabel, "source", in.SourceName, "reason", d.Reason)
		}
	}

	categoriesExtracted := 0
	for i := range observations {
		categoriesExtracted += len(observations[i].Categories)
	}

	status := RunStatusSuccess
	if res.Discarded > 0 || res.Skipped > 0 {
		status = RunStatusPartial
	}
	finished := time.Now().UTC()
	run := &CardRun{
		ID:                  runID,
		RunType:             RunTypeExtract,
		Trigger:             trigger,
		Status:              status,
		SourcesAttempted:    len(inputs),
		SourcesSucceeded:    res.Stored,
		CategoriesExtracted: categoriesExtracted,
		StartedAt:           &started,
		FinishedAt:          &finished,
	}

	flags := make([]CardPeriodRef, 0, len(flagSet))
	for f := range flagSet {
		flags = append(flags, f)
	}

	flagged, err := e.store.PersistExtractionRun(ctx, run, observations, flags)
	if err != nil {
		return nil, fmt.Errorf("cardrewards: persist extraction run: %w", err)
	}
	res.Run = run
	res.Flagged = flagged
	return res, nil
}

func (e *Extractor) knownCard(ctx context.Context, cardID string) (bool, error) {
	c, err := e.store.GetCatalogCard(ctx, cardID)
	if err != nil {
		return false, err
	}
	return c != nil, nil
}

// ValidRunTrigger reports whether t is an allowed card_runs.trigger value.
func ValidRunTrigger(t string) bool {
	return t == RunTriggerScheduled || t == RunTriggerManual
}

// HTTPSidecarExtractor is the production SidecarExtractor: it POSTs the
// extraction request to the ML sidecar's /extract-card-categories route with
// Bearer auth and a per-call timeout. It mirrors internal/agent/embedder/
// sidecar — the established Go↔sidecar HTTP contract — and introduces NO direct
// model-backend client of its own (Constitution C2 / NFR-CR-001/008).
type HTTPSidecarExtractor struct {
	baseURL    string
	authToken  string
	httpClient *http.Client
}

// NewHTTPSidecarExtractor constructs the production extractor. baseURL is the ML
// sidecar root (cfg.MLSidecarURL, e.g. http://smackerel-ml:8081), authToken is
// the Bearer token the sidecar's verify_auth expects (cfg.AuthToken), and
// timeout bounds each call. All three are required (fail-loud, no defaults).
func NewHTTPSidecarExtractor(baseURL, authToken string, timeout time.Duration) (*HTTPSidecarExtractor, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, errors.New("cardrewards.NewHTTPSidecarExtractor: baseURL must be non-empty")
	}
	if strings.TrimSpace(authToken) == "" {
		return nil, errors.New("cardrewards.NewHTTPSidecarExtractor: authToken must be non-empty")
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("cardrewards.NewHTTPSidecarExtractor: timeout must be > 0, got %s", timeout)
	}
	return &HTTPSidecarExtractor{
		baseURL:    strings.TrimRight(baseURL, "/"),
		authToken:  authToken,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// Extract POSTs the request to /extract-card-categories and returns the raw
// JSON body. Any transport error, non-2xx status, or empty body is an error;
// the orchestrator treats that as a failed extraction (flag-for-verification),
// never a silent success.
func (h *HTTPSidecarExtractor) Extract(ctx context.Context, req ExtractRequest) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("cardrewards sidecar: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.baseURL+"/extract-card-categories", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("cardrewards sidecar: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+h.authToken)

	resp, err := h.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cardrewards sidecar: POST /extract-card-categories: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("cardrewards sidecar: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cardrewards sidecar: /extract-card-categories returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, errors.New("cardrewards sidecar: /extract-card-categories returned empty body")
	}
	return json.RawMessage(raw), nil
}
