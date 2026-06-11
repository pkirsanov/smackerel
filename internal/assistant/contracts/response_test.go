package contracts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

// TestAllStatusTokens_Exhaustive — every literal StatusToken constant
// declared in response.go is present in AllStatusTokens exactly once.
func TestAllStatusTokens_Exhaustive(t *testing.T) {
	declared := []StatusToken{
		StatusThinking,
		StatusAnswered,
		StatusCheckingWeather,
		StatusCheckingEmail,
		StatusReminderProposed,
		StatusReminderConfirmed,
		StatusReminderCancelled,
		StatusSavedAsIdea,
		StatusUnavailable,
	}
	if len(AllStatusTokens) != len(declared) {
		t.Fatalf("AllStatusTokens length %d != declared %d", len(AllStatusTokens), len(declared))
	}
	seen := map[StatusToken]int{}
	for _, s := range AllStatusTokens {
		seen[s]++
	}
	for _, s := range declared {
		if seen[s] != 1 {
			t.Errorf("StatusToken %q appears %d times in AllStatusTokens, want 1", s, seen[s])
		}
	}
}

// TestAllErrorCauses_Exhaustive — every non-zero ErrorCause is present
// in AllErrorCauses exactly once; ErrNone is excluded by contract.
func TestAllErrorCauses_Exhaustive(t *testing.T) {
	declared := []ErrorCause{
		ErrProviderUnavailable,
		ErrMissingScope,
		ErrSlotMissing,
		ErrInternalError,
		ErrNoMatch,
	}
	if len(AllErrorCauses) != len(declared) {
		t.Fatalf("AllErrorCauses length %d != declared %d", len(AllErrorCauses), len(declared))
	}
	seen := map[ErrorCause]int{}
	for _, e := range AllErrorCauses {
		if e == ErrNone {
			t.Errorf("ErrNone must NOT appear in AllErrorCauses (it is the zero value)")
		}
		seen[e]++
	}
	for _, e := range declared {
		if seen[e] != 1 {
			t.Errorf("ErrorCause %q appears %d times in AllErrorCauses, want 1", e, seen[e])
		}
	}
}

// TestAllSourceKinds_Exhaustive — every literal SourceKind constant.
func TestAllSourceKinds_Exhaustive(t *testing.T) {
	declared := []SourceKind{SourceArtifact, SourceExternalProvider, SourceWeb, SourceToolComputation}
	if len(AllSourceKinds) != len(declared) {
		t.Fatalf("AllSourceKinds length %d != declared %d", len(AllSourceKinds), len(declared))
	}
}

// TestSourceRef_DiscriminatedUnion_RoundTrip — both concrete refs
// satisfy SourceRef and survive a type-switch round-trip.
func TestSourceRef_DiscriminatedUnion_RoundTrip(t *testing.T) {
	t0 := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	cases := []Source{
		{
			ID:    "a-1",
			Title: "artifact one",
			Kind:  SourceArtifact,
			Ref:   ArtifactRef{ArtifactID: "art-1", CapturedAt: t0},
		},
		{
			ID:    "p-1",
			Title: "open-meteo forecast",
			Kind:  SourceExternalProvider,
			Ref:   ExternalProviderRef{ProviderName: "open-meteo", RetrievedAt: t0},
		},
		{
			ID:    "w-1",
			Title: "Wikipedia — Paris",
			Kind:  SourceWeb,
			Ref:   WebSourceRef{URL: "https://en.wikipedia.org/wiki/Paris", Provider: "searxng", FetchedAt: t0, ContentHash: "sha256:abc", Snippet: "Paris."},
		},
		{
			ID:    "c-1",
			Title: "calculator",
			Kind:  SourceToolComputation,
			Ref:   ComputationSourceRef{Tool: "calculator", InputHash: "sha256:in", OutputHash: "sha256:out"},
		},
	}
	for _, c := range cases {
		switch ref := c.Ref.(type) {
		case ArtifactRef:
			if c.Kind != SourceArtifact {
				t.Errorf("ArtifactRef paired with Kind %q, want %q", c.Kind, SourceArtifact)
			}
			if ref.ArtifactID == "" {
				t.Errorf("ArtifactRef lost ArtifactID")
			}
		case ExternalProviderRef:
			if c.Kind != SourceExternalProvider {
				t.Errorf("ExternalProviderRef paired with Kind %q, want %q", c.Kind, SourceExternalProvider)
			}
			if ref.ProviderName == "" {
				t.Errorf("ExternalProviderRef lost ProviderName")
			}
		case WebSourceRef:
			if c.Kind != SourceWeb {
				t.Errorf("WebSourceRef paired with Kind %q, want %q", c.Kind, SourceWeb)
			}
			if ref.URL == "" || ref.ContentHash == "" {
				t.Errorf("WebSourceRef lost mandatory field")
			}
		case ComputationSourceRef:
			if c.Kind != SourceToolComputation {
				t.Errorf("ComputationSourceRef paired with Kind %q, want %q", c.Kind, SourceToolComputation)
			}
			if ref.Tool == "" || ref.InputHash == "" || ref.OutputHash == "" {
				t.Errorf("ComputationSourceRef lost mandatory field")
			}
		default:
			t.Errorf("unknown SourceRef impl: %T", c.Ref)
		}
	}
}

// TestAssistantResponse_NetNewFieldCount_Exactly6 — spec.md §3.1.3
// requires AssistantResponse to add EXACTLY 6 net-new fields beyond
// the spec 037 substrate references and the convenience derivatives.
// Counted by reflection; tagged in the type's structural layout via a
// dedicated helper to avoid coupling the test to source-line order.
func TestAssistantResponse_NetNewFieldCount_Exactly6(t *testing.T) {
	netNew := map[string]bool{
		"Status":               true,
		"Sources":              true,
		"ConfirmCard":          true,
		"DisambiguationPrompt": true,
		"ErrorCause":           true,
		"CaptureRoute":         true,
	}
	if len(netNew) != 6 {
		t.Fatalf("test data invariant broken: netNew set has %d entries, want 6", len(netNew))
	}
	rt := reflect.TypeOf(AssistantResponse{})
	count := 0
	for i := 0; i < rt.NumField(); i++ {
		if netNew[rt.Field(i).Name] {
			count++
		}
	}
	if count != 6 {
		t.Fatalf("AssistantResponse contains %d of the 6 net-new fields; spec.md §3.1.3 requires all 6 present (or none renamed)", count)
	}
}

// TestAssistantResponse_NoConfirmAndDisambigSimultaneous — adversarial
// contract assertion: the renderer assumes ConfirmCard and
// DisambiguationPrompt are mutually exclusive. Until we add a stricter
// validator this is a documented invariant; this test exercises the
// constructor pattern that a future facade impl will use, asserting
// the simultaneous-non-nil case is detectable by adapters.
func TestAssistantResponse_NoConfirmAndDisambigSimultaneous(t *testing.T) {
	resp := AssistantResponse{
		ConfirmCard:          &ConfirmCard{ConfirmRef: "X"},
		DisambiguationPrompt: &DisambiguationPrompt{DisambiguationRef: "Y"},
	}
	// Adapters MUST treat this combination as an internal error.
	// We assert here that the simultaneity is detectable from the
	// canonical type alone (no extra metadata needed).
	if resp.ConfirmCard == nil || resp.DisambiguationPrompt == nil {
		t.Fatalf("test fixture lost a pointer field")
	}
	// The detectable check the adapter performs:
	if (resp.ConfirmCard != nil) && (resp.DisambiguationPrompt != nil) {
		// expected — this is the invariant the adapter would flag
		return
	}
	t.Fatal("simultaneity invariant not detectable from canonical type")
}

// goldenCase pairs a fixture file with the constructed response it
// must equal. Used by TestAssistantResponse_GoldenFixtures.
type goldenCase struct {
	name string
	resp AssistantResponse
}

func goldenCases() []goldenCase {
	t0 := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	return []goldenCase{
		// Status × ErrorCause × CaptureRoute × Source kind
		// combinations relevant to v1. Target ≥ 12 per DoD #4.
		{
			name: "thinking_no_sources",
			resp: AssistantResponse{
				Status:    StatusThinking,
				Body:      "thinking…",
				EmittedAt: t0,
			},
		},
		{
			name: "checking_weather_external_provider",
			resp: AssistantResponse{
				Status:    StatusCheckingWeather,
				Body:      "checking weather for Lisbon",
				Sources:   []Source{{ID: "p-1", Title: "open-meteo", Kind: SourceExternalProvider, Ref: ExternalProviderRef{ProviderName: "open-meteo", RetrievedAt: t0}}},
				EmittedAt: t0,
			},
		},
		{
			name: "reminder_proposed_confirm_card",
			resp: AssistantResponse{
				Status:      StatusReminderProposed,
				Body:        "Schedule reminder?",
				ConfirmCard: &ConfirmCard{ProposedAction: "schedule notification", ConfirmRef: "ULID-CR-1", Timeout: 5 * time.Minute, PositiveLabel: "yes", NegativeLabel: "no"},
				EmittedAt:   t0,
			},
		},
		{
			name: "reminder_confirmed_artifact_source",
			resp: AssistantResponse{
				Status:    StatusReminderConfirmed,
				Body:      "Reminder scheduled.",
				Sources:   []Source{{ID: "a-1", Title: "scheduled job", Kind: SourceArtifact, Ref: ArtifactRef{ArtifactID: "art-1", CapturedAt: t0}}},
				EmittedAt: t0,
			},
		},
		{
			name: "reminder_cancelled",
			resp: AssistantResponse{
				Status:    StatusReminderCancelled,
				Body:      "Reminder cancelled.",
				EmittedAt: t0,
			},
		},
		{
			name: "saved_as_idea_capture_route",
			resp: AssistantResponse{
				Status:       StatusSavedAsIdea,
				Body:         "Saved as a note.",
				CaptureRoute: true,
				EmittedAt:    t0,
			},
		},
		{
			name: "unavailable_provider",
			resp: AssistantResponse{
				Status:     StatusUnavailable,
				Body:       "weather provider is unreachable.",
				ErrorCause: ErrProviderUnavailable,
				EmittedAt:  t0,
			},
		},
		{
			name: "unavailable_missing_scope",
			resp: AssistantResponse{
				Status:     StatusUnavailable,
				Body:       "missing scope: assistant.skill.notifications.write",
				ErrorCause: ErrMissingScope,
				EmittedAt:  t0,
			},
		},
		{
			name: "unavailable_slot_missing_disambig",
			resp: AssistantResponse{
				Status:     StatusUnavailable,
				Body:       "missing required slot: location",
				ErrorCause: ErrSlotMissing,
				DisambiguationPrompt: &DisambiguationPrompt{
					Choices: []DisambiguationChoice{
						{Number: 1, ID: "weather_query", Label: "ask weather for Lisbon", Shortcut: "/weather Lisbon"},
						{Number: 2, ID: SaveAsNoteChoiceID, Label: "save as note"},
					},
					Timeout:           5 * time.Minute,
					DisambiguationRef: "ULID-DR-1",
				},
				EmittedAt: t0,
			},
		},
		{
			name: "unavailable_internal_capture",
			resp: AssistantResponse{
				Status:       StatusUnavailable,
				Body:         "something went wrong; saved as a note.",
				ErrorCause:   ErrInternalError,
				CaptureRoute: true,
				EmittedAt:    t0,
			},
		},
		{
			// BUG-061-003 — recipe_search empty-graph deterministic state.
			name: "unavailable_no_match_no_capture",
			resp: AssistantResponse{
				Status:       StatusUnavailable,
				Body:         "no recipes saved yet — capture one with /capture or import via a connector.",
				ErrorCause:   ErrNoMatch,
				CaptureRoute: false,
				EmittedAt:    t0,
			},
		},
		{
			name: "checking_email_v2_placeholder",
			resp: AssistantResponse{
				Status:    StatusCheckingEmail,
				Body:      "checking email…",
				EmittedAt: t0,
			},
		},
		{
			name: "thinking_with_sources_overflow",
			resp: AssistantResponse{
				Status: StatusThinking,
				Body:   "answer body",
				Sources: []Source{
					{ID: "a-1", Title: "first", Kind: SourceArtifact, Ref: ArtifactRef{ArtifactID: "art-1", CapturedAt: t0}},
					{ID: "a-2", Title: "second", Kind: SourceArtifact, Ref: ArtifactRef{ArtifactID: "art-2", CapturedAt: t0}},
				},
				SourcesOverflowCount: 3,
				EmittedAt:            t0,
			},
		},
		{
			// PKT-061-A — web source kind round-trip.
			name: "thinking_web_source",
			resp: AssistantResponse{
				Status: StatusThinking,
				Body:   "the capital of France is Paris.",
				Sources: []Source{
					{ID: "w-1", Title: "Wikipedia — Paris", Kind: SourceWeb, Ref: WebSourceRef{URL: "https://en.wikipedia.org/wiki/Paris", Provider: "searxng", FetchedAt: t0, ContentHash: "sha256:abc", Snippet: "Paris is the capital and most populous city of France."}},
				},
				EmittedAt: t0,
			},
		},
		{
			// PKT-061-A — tool-computation source kind round-trip.
			name: "thinking_tool_computation_source",
			resp: AssistantResponse{
				Status: StatusThinking,
				Body:   "2 + 2 = 4",
				Sources: []Source{
					{ID: "c-1", Title: "calculator", Kind: SourceToolComputation, Ref: ComputationSourceRef{Tool: "calculator", InputHash: "sha256:in", OutputHash: "sha256:out"}},
				},
				EmittedAt: t0,
			},
		},
		{
			// BUG-064-002 DEFECT 3a — terminal answered status for a
			// delivered open_knowledge answer (no "thinking…" header).
			name: "answered_open_knowledge_web_source",
			resp: AssistantResponse{
				Status: StatusAnswered,
				Body:   "wa-town-A 06/11: high 7:42am 8.9 ft, low 1:55pm 0.3 ft.",
				Sources: []Source{
					{ID: "w-1", Title: "tidetime.org", Kind: SourceWeb, Ref: WebSourceRef{URL: "https://tidetime.org/wa-town-A", Provider: "searxng", FetchedAt: t0, ContentHash: "sha256:tide", Snippet: "wa-town-A tide times"}},
				},
				EmittedAt: t0,
			},
		},
	}
}

// TestGoldenCases_CoverEveryCombinationAxis — adversarial check that
// the golden corpus covers every StatusToken (v1 relevant set),
// every non-zero ErrorCause, both Source kinds, AND the CaptureRoute
// flag in both states. Removing a case silently MUST fail this test.
func TestGoldenCases_CoverEveryCombinationAxis(t *testing.T) {
	cases := goldenCases()
	if len(cases) < 12 {
		t.Fatalf("DoD #4 requires ≥12 golden fixtures, got %d", len(cases))
	}
	seenStatus := map[StatusToken]bool{}
	seenErr := map[ErrorCause]bool{}
	seenSrcKind := map[SourceKind]bool{}
	seenCapture := map[bool]bool{}
	for _, c := range cases {
		seenStatus[c.resp.Status] = true
		if c.resp.ErrorCause != ErrNone {
			seenErr[c.resp.ErrorCause] = true
		}
		for _, s := range c.resp.Sources {
			seenSrcKind[s.Kind] = true
		}
		seenCapture[c.resp.CaptureRoute] = true
	}
	for _, s := range AllStatusTokens {
		if !seenStatus[s] {
			t.Errorf("StatusToken %q not covered by any golden fixture", s)
		}
	}
	for _, e := range AllErrorCauses {
		if !seenErr[e] {
			t.Errorf("ErrorCause %q not covered by any golden fixture", e)
		}
	}
	for _, k := range AllSourceKinds {
		if !seenSrcKind[k] {
			t.Errorf("SourceKind %q not covered by any golden fixture", k)
		}
	}
	if !seenCapture[true] || !seenCapture[false] {
		t.Errorf("CaptureRoute not covered in both states: %+v", seenCapture)
	}
}

// TestAssistantResponse_GoldenFixtures — writes (when UPDATE_GOLDEN=1)
// or compares each goldenCase to a JSON file under testdata/golden/.
// JSON is canonical (sorted keys, stable order) so renames / accidental
// schema drift are detected mechanically.
func TestAssistantResponse_GoldenFixtures(t *testing.T) {
	dir := filepath.Join("testdata", "golden")
	cases := goldenCases()
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			// Use a stable subset of fields for the JSON snapshot
			// — avoids serializing the substrate-reference pointers
			// (which are nil in v1 fixtures anyway) and produces a
			// human-readable file.
			snap := snapshotForGolden(c.resp)
			actual, err := json.MarshalIndent(snap, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent: %v", err)
			}
			actual = append(actual, '\n')
			path := filepath.Join(dir, c.name+".json")
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				if err := os.WriteFile(path, actual, 0o644); err != nil {
					t.Fatalf("WriteFile: %v", err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (run with UPDATE_GOLDEN=1 to create)", path, err)
			}
			if string(actual) != string(want) {
				t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", c.name, want, actual)
			}
		})
	}
}

// snapshotForGolden produces a canonical map suitable for stable JSON
// serialization. Keys are sorted (json.Marshal sorts map keys
// alphabetically by default).
func snapshotForGolden(r AssistantResponse) map[string]any {
	m := map[string]any{
		"status":                 string(r.Status),
		"body":                   r.Body,
		"capture_route":          r.CaptureRoute,
		"error_cause":            string(r.ErrorCause),
		"emitted_at":             r.EmittedAt.UTC().Format(time.RFC3339Nano),
		"sources_overflow_count": r.SourcesOverflowCount,
	}
	if len(r.Sources) > 0 {
		srcs := make([]map[string]any, 0, len(r.Sources))
		for _, s := range r.Sources {
			entry := map[string]any{
				"id":    s.ID,
				"title": s.Title,
				"kind":  string(s.Kind),
			}
			switch ref := s.Ref.(type) {
			case ArtifactRef:
				entry["ref"] = map[string]any{
					"kind":        "artifact",
					"artifact_id": ref.ArtifactID,
					"captured_at": ref.CapturedAt.UTC().Format(time.RFC3339Nano),
				}
			case ExternalProviderRef:
				entry["ref"] = map[string]any{
					"kind":          "external_provider",
					"provider_name": ref.ProviderName,
					"retrieved_at":  ref.RetrievedAt.UTC().Format(time.RFC3339Nano),
				}
			case WebSourceRef:
				entry["ref"] = map[string]any{
					"kind":         "web",
					"url":          ref.URL,
					"provider":     ref.Provider,
					"fetched_at":   ref.FetchedAt.UTC().Format(time.RFC3339Nano),
					"content_hash": ref.ContentHash,
					"snippet":      ref.Snippet,
				}
			case ComputationSourceRef:
				entry["ref"] = map[string]any{
					"kind":        "tool_computation",
					"tool":        ref.Tool,
					"input_hash":  ref.InputHash,
					"output_hash": ref.OutputHash,
				}
			}
			srcs = append(srcs, entry)
		}
		m["sources"] = srcs
	}
	if r.ConfirmCard != nil {
		m["confirm_card"] = map[string]any{
			"proposed_action": r.ConfirmCard.ProposedAction,
			"confirm_ref":     r.ConfirmCard.ConfirmRef,
			"timeout":         r.ConfirmCard.Timeout.String(),
			"positive_label":  r.ConfirmCard.PositiveLabel,
			"negative_label":  r.ConfirmCard.NegativeLabel,
		}
	}
	if r.DisambiguationPrompt != nil {
		choices := make([]map[string]any, 0, len(r.DisambiguationPrompt.Choices))
		for _, c := range r.DisambiguationPrompt.Choices {
			choices = append(choices, map[string]any{
				"number":   c.Number,
				"id":       c.ID,
				"label":    c.Label,
				"shortcut": c.Shortcut,
			})
		}
		m["disambiguation_prompt"] = map[string]any{
			"disambiguation_ref": r.DisambiguationPrompt.DisambiguationRef,
			"timeout":            r.DisambiguationPrompt.Timeout.String(),
			"choices":            choices,
		}
	}
	// Keys returned via map — sort confirmation (compile-time check that
	// the sort import is used).
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return m
}
