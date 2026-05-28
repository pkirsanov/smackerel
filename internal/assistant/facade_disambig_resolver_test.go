// Spec 061 SCOPE-09 — DISAMBIG-OUTCOMES-EMISSION verification.
//
// Drives the facade through the four terminal paths of the Step-1.5
// pending-disambiguation resolver and asserts that
// smackerel_assistant_disambiguation_outcomes_total observes exactly
// one increment per turn under the right (outcome, transport) label
// pair. Also asserts that the BandBorderline post-processor persists
// PendingDisambig so the subsequent turn can be resolved.
//
// Outcomes covered (closed vocabulary, design §3.2):
//
//	resolved_user                — typed disambig reply maps to a choice
//	resolved_user                — text reply "2" maps to a choice (fallback)
//	resolved_timeout_capture     — emittedAt > PendingDisambig.ExpiresAt
//	resolved_non_matching_reply  — pending disambig present but no match
//
// Paired CaptureFallbackTotal increments are verified on the two
// capture paths (borderline_timeout / low_confidence). The save_as_note
// branch is exercised via the typed-reply path and asserted NOT to
// increment CaptureFallbackTotal (explicit user choice, not a fallback).

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	assistantmetrics "github.com/smackerel/smackerel/internal/assistant/metrics"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// readCounter returns the current value of the (outcome, transport)
// pair on DisambiguationOutcomesTotal. Returns 0 when the label pair
// has not yet been recorded.
func readDisambigOutcome(t *testing.T, outcome, transport string) float64 {
	t.Helper()
	m, err := assistantmetrics.DisambiguationOutcomesTotal.GetMetricWithLabelValues(outcome, transport)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q,%q): %v", outcome, transport, err)
	}
	var pb dto.Metric
	if err := m.(prometheus.Counter).Write(&pb); err != nil {
		t.Fatalf("counter.Write: %v", err)
	}
	return pb.GetCounter().GetValue()
}

// readCaptureFallback returns the current value of the (cause, transport)
// pair on CaptureFallbackTotal.
func readCaptureFallback(t *testing.T, cause, transport string) float64 {
	t.Helper()
	m, err := assistantmetrics.CaptureFallbackTotal.GetMetricWithLabelValues(cause, transport)
	if err != nil {
		t.Fatalf("GetMetricWithLabelValues(%q,%q): %v", cause, transport, err)
	}
	var pb dto.Metric
	if err := m.(prometheus.Counter).Write(&pb); err != nil {
		t.Fatalf("counter.Write: %v", err)
	}
	return pb.GetCounter().GetValue()
}

// borderlineFacade builds a facade whose router returns a borderline
// decision with two manifest-enabled candidates, so the BandBorderline
// branch fires on the first turn and persists PendingDisambig with the
// shape {1: weather_query, 2: retrieval_qa, 3: save_as_note}.
func borderlineFacade(t *testing.T, now time.Time) (*Facade, *memContextStore) {
	t.Helper()
	weather := &agent.Scenario{ID: "weather_query"}
	retrieval := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": weather,
		"retrieval_qa":  retrieval,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
		"retrieval_qa": {
			UserFacingLabel: "search my notes", SlashShortcut: "/ask",
			EnableSSTKey: "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{
		chosen: weather,
		decision: agent.RoutingDecision{
			Reason:   agent.ReasonSimilarityMatch,
			Chosen:   "weather_query",
			TopScore: 0.60, // BandBorderline: AgentConfidenceFloor (0.50) <= 0.60 < BorderlineFloor (0.75)
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.60},
				{ScenarioID: "retrieval_qa", Score: 0.55},
			},
		},
		ok: true,
	}
	cfg := defaultFacadeConfig(now)
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)
	return facade, store
}

// emitBorderlinePrompt fires the first turn that drives the facade
// into BandBorderline. Returns the prompt so the test can read the
// DisambiguationRef for the second-turn reply.
func emitBorderlinePrompt(t *testing.T, facade *Facade, userID string) *contracts.DisambiguationPrompt {
	t.Helper()
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    userID,
		Transport: "telegram",
		Text:      "tell me about barcelona",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("turn 1 (borderline): %v", err)
	}
	if resp.DisambiguationPrompt == nil {
		t.Fatalf("turn 1 MUST emit a DisambiguationPrompt; got nil. status=%s body=%q",
			resp.Status, resp.Body)
	}
	return resp.DisambiguationPrompt
}

func TestFacade_BandBorderline_PersistsPendingDisambig(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 2, 1, 9, 0, 0, 0, time.UTC)
	facade, store := borderlineFacade(t, now)

	prompt := emitBorderlinePrompt(t, facade, "u-persist")

	conv, ok, _ := store.Load(context.Background(), "u-persist", "telegram")
	if !ok {
		t.Fatalf("conversation row not persisted after BandBorderline turn")
	}
	if conv.PendingDisambig == nil {
		t.Fatalf("BandBorderline MUST persist PendingDisambig; got nil")
	}
	if conv.PendingDisambig.DisambiguationRef != prompt.DisambiguationRef {
		t.Errorf("PendingDisambig.DisambiguationRef = %q; want %q",
			conv.PendingDisambig.DisambiguationRef, prompt.DisambiguationRef)
	}
	wantExpires := now.Add(30 * time.Second)
	if !conv.PendingDisambig.ExpiresAt.Equal(wantExpires) {
		t.Errorf("PendingDisambig.ExpiresAt = %v; want %v",
			conv.PendingDisambig.ExpiresAt, wantExpires)
	}
	if len(conv.PendingDisambig.Choices) != 3 {
		t.Fatalf("PendingDisambig.Choices len = %d; want 3 (2 enabled + save_as_note)",
			len(conv.PendingDisambig.Choices))
	}
	if conv.PendingDisambig.Choices[0].ID != "weather_query" || conv.PendingDisambig.Choices[0].Number != 1 {
		t.Errorf("Choice[0] = {%d,%q}; want {1,weather_query}",
			conv.PendingDisambig.Choices[0].Number, conv.PendingDisambig.Choices[0].ID)
	}
	if conv.PendingDisambig.Choices[1].ID != "retrieval_qa" || conv.PendingDisambig.Choices[1].Number != 2 {
		t.Errorf("Choice[1] = {%d,%q}; want {2,retrieval_qa}",
			conv.PendingDisambig.Choices[1].Number, conv.PendingDisambig.Choices[1].ID)
	}
	if conv.PendingDisambig.Choices[2].ID != contracts.SaveAsNoteChoiceID {
		t.Errorf("Choice[2].ID = %q; want save_as_note sentinel", conv.PendingDisambig.Choices[2].ID)
	}
}

// NOTE: The five tests below (TypedReply/TextNumericFallback/SaveAsNote/
// NonMatchingReply/NonNumericText) all assert before/after deltas on the
// shared global Prometheus counters DisambiguationOutcomesTotal and
// CaptureFallbackTotal under transport="telegram". They MUST run serially —
// running them in parallel torpedoes the sandwich because sibling increments
// from concurrently-executing tests leak into each other's window, yielding
// non-deterministic "increment = 2.0; want 1" failures. The persistence-only
// test TestFacade_BandBorderline_PersistsPendingDisambig stays parallel
// because it does not read or assert counter values. Production code
// increments each (outcome,transport) pair exactly once per resolved turn
// at facade.go:resolvePendingDisambig — verified across all 7 emission
// branches by reading the resolver helper end-to-end.
func TestFacade_DisambigResolved_TypedReply_EmitsResolvedUser(t *testing.T) {
	now := time.Date(2025, 2, 1, 10, 0, 0, 0, time.UTC)
	facade, store := borderlineFacade(t, now)
	prompt := emitBorderlinePrompt(t, facade, "u-user")

	before := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")

	// Second turn: typed disambig reply selecting choice 2 (retrieval_qa).
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:               "u-user",
		Transport:            "telegram",
		Kind:                 contracts.KindDisambiguation,
		DisambiguationRef:    prompt.DisambiguationRef,
		DisambiguationChoice: 2,
	})
	if err != nil {
		t.Fatalf("turn 2 (typed disambig): %v", err)
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("resp.Status = %q; want StatusSavedAsIdea (resolver should acknowledge and ask re-send)", resp.Status)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; want false for resolved_user scenario selection")
	}

	after := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")
	if after-before != 1 {
		t.Errorf("DisambigOutcomeResolvedUser increment = %.1f; want 1", after-before)
	}

	// PendingDisambig MUST be cleared.
	conv, _, _ := store.Load(context.Background(), "u-user", "telegram")
	if conv.PendingDisambig != nil {
		t.Errorf("PendingDisambig MUST be cleared after resolved_user; got %+v", conv.PendingDisambig)
	}
}

func TestFacade_DisambigResolved_TextNumericFallback_EmitsResolvedUser(t *testing.T) {
	now := time.Date(2025, 2, 1, 11, 0, 0, 0, time.UTC)
	facade, _ := borderlineFacade(t, now)
	_ = emitBorderlinePrompt(t, facade, "u-text")

	before := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")

	// Second turn: plain text "1" — fallback numeric match should
	// resolve to weather_query and emit resolved_user.
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-text",
		Transport: "telegram",
		Text:      " 1 ",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("turn 2 (text fallback): %v", err)
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("resp.Status = %q; want StatusSavedAsIdea", resp.Status)
	}

	after := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")
	if after-before != 1 {
		t.Errorf("DisambigOutcomeResolvedUser increment = %.1f; want 1", after-before)
	}
}

func TestFacade_DisambigResolved_SaveAsNote_EmitsResolvedUserNoFallback(t *testing.T) {
	now := time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC)
	facade, _ := borderlineFacade(t, now)
	prompt := emitBorderlinePrompt(t, facade, "u-note")

	beforeUser := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")
	beforeFallback := readCaptureFallback(t, assistantmetrics.CauseLowConfidence, "telegram")

	// The save_as_note sentinel is at position 3 in our prompt (after
	// the two enabled scenarios). Verify and select it.
	if len(prompt.Choices) != 3 || prompt.Choices[2].ID != contracts.SaveAsNoteChoiceID {
		t.Fatalf("prompt.Choices[2] expected to be save_as_note; got %+v", prompt.Choices)
	}
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:               "u-note",
		Transport:            "telegram",
		Kind:                 contracts.KindDisambiguation,
		DisambiguationRef:    prompt.DisambiguationRef,
		DisambiguationChoice: 3,
	})
	if err != nil {
		t.Fatalf("turn 2 (save_as_note): %v", err)
	}
	if !resp.CaptureRoute {
		t.Errorf("save_as_note MUST set CaptureRoute=true; got false")
	}

	afterUser := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedUser, "telegram")
	afterFallback := readCaptureFallback(t, assistantmetrics.CauseLowConfidence, "telegram")
	if afterUser-beforeUser != 1 {
		t.Errorf("DisambigOutcomeResolvedUser increment for save_as_note = %.1f; want 1",
			afterUser-beforeUser)
	}
	if afterFallback-beforeFallback != 0 {
		t.Errorf("save_as_note MUST NOT increment CaptureFallback(low_confidence); got delta %.1f",
			afterFallback-beforeFallback)
	}
}

func TestFacade_DisambigResolved_TTLExpired_EmitsTimeoutCapture(t *testing.T) {
	now := time.Date(2025, 2, 1, 13, 0, 0, 0, time.UTC)

	// Build a mutable clock: turn 1 at `now`, turn 2 at `now+60s`
	// (past the 30s DisambigTimeout).
	clock := &mutableClock{t: now}
	weather := &agent.Scenario{ID: "weather_query"}
	retrieval := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": weather,
		"retrieval_qa":  retrieval,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
		"retrieval_qa": {
			UserFacingLabel: "search my notes", SlashShortcut: "/ask",
			EnableSSTKey: "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{
		chosen: weather,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.60,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.60},
				{ScenarioID: "retrieval_qa", Score: 0.55},
			},
		},
		ok: true,
	}
	cfg := defaultFacadeConfig(now)
	cfg.Now = clock.Now
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-ttl", Transport: "telegram",
		Text: "tell me about barcelona", Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("turn 1: %v", err)
	}

	beforeOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedTimeoutCapture, "telegram")
	beforeFallback := readCaptureFallback(t, assistantmetrics.CauseBorderlineTimeout, "telegram")

	// Advance the clock past the TTL.
	clock.t = now.Add(60 * time.Second)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-ttl", Transport: "telegram",
		Text: "1", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("turn 2 (ttl): %v", err)
	}
	if !resp.CaptureRoute {
		t.Errorf("TTL-expired turn MUST set CaptureRoute=true; got false")
	}

	afterOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedTimeoutCapture, "telegram")
	afterFallback := readCaptureFallback(t, assistantmetrics.CauseBorderlineTimeout, "telegram")
	if afterOutcome-beforeOutcome != 1 {
		t.Errorf("DisambigOutcomeResolvedTimeoutCapture increment = %.1f; want 1", afterOutcome-beforeOutcome)
	}
	if afterFallback-beforeFallback != 1 {
		t.Errorf("CaptureFallback(borderline_timeout) increment = %.1f; want 1 (paired with TTL outcome)",
			afterFallback-beforeFallback)
	}

	conv, _, _ := store.Load(context.Background(), "u-ttl", "telegram")
	if conv.PendingDisambig != nil {
		t.Errorf("PendingDisambig MUST be cleared after TTL-capture; got %+v", conv.PendingDisambig)
	}
}

func TestFacade_DisambigResolved_NonMatchingReply_EmitsNonMatching(t *testing.T) {
	now := time.Date(2025, 2, 1, 14, 0, 0, 0, time.UTC)
	facade, store := borderlineFacade(t, now)
	prompt := emitBorderlinePrompt(t, facade, "u-nomatch")

	beforeOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedNonMatchingReply, "telegram")
	beforeFallback := readCaptureFallback(t, assistantmetrics.CauseLowConfidence, "telegram")

	// Typed disambig with out-of-range number — counts as
	// non-matching reply (matching ref but no choice number match).
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:               "u-nomatch",
		Transport:            "telegram",
		Kind:                 contracts.KindDisambiguation,
		DisambiguationRef:    prompt.DisambiguationRef,
		DisambiguationChoice: 99,
	})
	if err != nil {
		t.Fatalf("turn 2 (non-match): %v", err)
	}
	if !resp.CaptureRoute {
		t.Errorf("non-matching reply MUST set CaptureRoute=true; got false")
	}

	afterOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedNonMatchingReply, "telegram")
	afterFallback := readCaptureFallback(t, assistantmetrics.CauseLowConfidence, "telegram")
	if afterOutcome-beforeOutcome != 1 {
		t.Errorf("DisambigOutcomeResolvedNonMatchingReply increment = %.1f; want 1", afterOutcome-beforeOutcome)
	}
	if afterFallback-beforeFallback != 1 {
		t.Errorf("CaptureFallback(low_confidence) increment = %.1f; want 1 (paired with non-match)",
			afterFallback-beforeFallback)
	}

	conv, _, _ := store.Load(context.Background(), "u-nomatch", "telegram")
	if conv.PendingDisambig != nil {
		t.Errorf("PendingDisambig MUST be cleared after non-matching reply; got %+v", conv.PendingDisambig)
	}
}

func TestFacade_DisambigResolved_NonNumericText_EmitsNonMatching(t *testing.T) {
	now := time.Date(2025, 2, 1, 15, 0, 0, 0, time.UTC)
	facade, _ := borderlineFacade(t, now)
	_ = emitBorderlinePrompt(t, facade, "u-words")

	beforeOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedNonMatchingReply, "telegram")

	// Plain-text reply that doesn't parse to a number — also a
	// non-matching reply.
	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-words", Transport: "telegram",
		Text: "hmm not sure", Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("turn 2 (words): %v", err)
	}

	afterOutcome := readDisambigOutcome(t, assistantmetrics.DisambigOutcomeResolvedNonMatchingReply, "telegram")
	if afterOutcome-beforeOutcome != 1 {
		t.Errorf("DisambigOutcomeResolvedNonMatchingReply increment for non-numeric text = %.1f; want 1",
			afterOutcome-beforeOutcome)
	}
}

// mutableClock is a Now() source whose t can be reassigned between
// turns to exercise time-dependent paths (TTL expiry).
type mutableClock struct {
	t time.Time
}

func (c *mutableClock) Now() time.Time { return c.t }
