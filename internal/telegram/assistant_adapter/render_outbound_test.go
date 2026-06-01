package assistant_adapter

import (
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestBuildTelegramRendering_StatusPrefix exercises the in-flight
// status tokens that produce a first-line prefix per spec.md §14.B.1.
func TestBuildTelegramRendering_StatusPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status contracts.StatusToken
		want   string // expected prefix
	}{
		{"thinking", contracts.StatusThinking, "thinking…"},
		{"weather", contracts.StatusCheckingWeather, "checking weather…"},
		{"email", contracts.StatusCheckingEmail, "checking email…"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rendered, keyboard, err := buildTelegramRendering(contracts.AssistantResponse{
				Status: tc.status,
				Body:   "hello",
			}, PlainText, 4096)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if keyboard != nil {
				t.Errorf("keyboard = %v; want nil", keyboard)
			}
			if !strings.HasPrefix(rendered, tc.want) {
				t.Errorf("rendered = %q; want prefix %q", rendered, tc.want)
			}
		})
	}
}

// TestBuildTelegramRendering_ErrorSingleLine asserts the single-line
// "<skill>: <cause>" error rendering for StatusUnavailable.
func TestBuildTelegramRendering_ErrorSingleLine(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Status:     contracts.StatusUnavailable,
		ErrorCause: contracts.ErrProviderUnavailable,
		Routing:    &agent.RoutingDecision{Chosen: "weather"},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rendered != "weather: provider_unavailable" {
		t.Errorf("rendered = %q; want %q", rendered, "weather: provider_unavailable")
	}
}

// TestBuildTelegramRendering_ErrorNoRouting falls back to the cause
// alone when the capability layer never called the router.
func TestBuildTelegramRendering_ErrorNoRouting(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Status:     contracts.StatusUnavailable,
		ErrorCause: contracts.ErrInternalError,
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rendered != "internal_error" {
		t.Errorf("rendered = %q; want %q", rendered, "internal_error")
	}
}

// TestBuildTelegramRendering_ConfirmHasKeyboard asserts a confirm
// response produces both the body and the [✅ pos][❌ neg] inline
// keyboard with callback_data carrying the ConfirmRef.
func TestBuildTelegramRendering_ConfirmHasKeyboard(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Status: contracts.StatusReminderProposed,
		Body:   "I'll remind you tomorrow at 9am.",
		ConfirmCard: &contracts.ConfirmCard{
			ProposedAction: "Schedule reminder?",
			ConfirmRef:     "01HCONFIRMREFXYZ",
			PositiveLabel:  "Confirm",
			NegativeLabel:  "Cancel",
		},
	}
	rendered, keyboard, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if keyboard == nil {
		t.Fatal("keyboard = nil; want non-nil")
	}
	if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 2 {
		t.Fatalf("keyboard shape = %v; want 1×2 row", keyboard.InlineKeyboard)
	}
	posBtn := keyboard.InlineKeyboard[0][0]
	negBtn := keyboard.InlineKeyboard[0][1]
	if !strings.HasPrefix(posBtn.Text, "✅ ") || !strings.Contains(posBtn.Text, "Confirm") {
		t.Errorf("positive button text = %q; want ✅ Confirm", posBtn.Text)
	}
	if !strings.HasPrefix(negBtn.Text, "❌ ") || !strings.Contains(negBtn.Text, "Cancel") {
		t.Errorf("negative button text = %q; want ❌ Cancel", negBtn.Text)
	}
	if posBtn.CallbackData == nil || !strings.HasPrefix(*posBtn.CallbackData, "a:c:01HCONFIRMREFXYZ:") {
		t.Errorf("positive callback_data = %v; want a:c:<ref>:pos shape", posBtn.CallbackData)
	}
	if !strings.Contains(rendered, "I'll remind you tomorrow at 9am.") {
		t.Errorf("rendered = %q; want to include body", rendered)
	}
	if !strings.Contains(rendered, "Schedule reminder?") {
		t.Errorf("rendered = %q; want to include proposed action", rendered)
	}
}

// TestBuildTelegramRendering_DisambigKeyboardThreeChoices asserts
// up to 3 disambiguation choices produce a single-row keyboard with
// numbered callback_data.
func TestBuildTelegramRendering_DisambigKeyboardThreeChoices(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "Which did you mean?",
		DisambiguationPrompt: &contracts.DisambiguationPrompt{
			DisambiguationRef: "01HDISAMBIG12345",
			Choices: []contracts.DisambiguationChoice{
				{Number: 1, ID: "weather", Label: "Check the weather", Shortcut: "/weather"},
				{Number: 2, ID: "find", Label: "Search saved notes", Shortcut: "/find"},
				{Number: 3, ID: contracts.SaveAsNoteChoiceID, Label: "Save as note"},
			},
		},
	}
	rendered, keyboard, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if keyboard == nil {
		t.Fatal("keyboard = nil; want non-nil")
	}
	if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 3 {
		t.Fatalf("keyboard shape = %v; want 1×3 row", keyboard.InlineKeyboard)
	}
	for i, btn := range keyboard.InlineKeyboard[0] {
		wantPrefix := "a:d:01HDISAMBIG12345:"
		if btn.CallbackData == nil || !strings.HasPrefix(*btn.CallbackData, wantPrefix) {
			t.Errorf("button %d callback_data = %v; want prefix %q", i, btn.CallbackData, wantPrefix)
		}
	}
	if !strings.Contains(rendered, "Which did you mean?") {
		t.Errorf("rendered = %q; want body", rendered)
	}
	if !strings.Contains(rendered, "1. Check the weather [/weather]") {
		t.Errorf("rendered = %q; want numbered choice 1 with shortcut", rendered)
	}
	if !strings.Contains(rendered, "3. Save as note") {
		t.Errorf("rendered = %q; want save-as-note choice last", rendered)
	}
}

// TestBuildTelegramRendering_MutuallyExclusiveConfirmAndDisambig
// asserts the capability-contract violation (both ConfirmCard and
// DisambiguationPrompt set) is refused with an error rather than
// silently rendering one and dropping the other.
func TestBuildTelegramRendering_MutuallyExclusiveConfirmAndDisambig(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body:                 "broken",
		ConfirmCard:          &contracts.ConfirmCard{ConfirmRef: "x"},
		DisambiguationPrompt: &contracts.DisambiguationPrompt{DisambiguationRef: "y"},
	}
	_, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err == nil {
		t.Fatal("err = nil; want capability-contract-violation error")
	}
}

// TestBuildTelegramRendering_SourcesPreservedUnderBudget asserts the
// sources block is preserved verbatim and the body is truncated
// with "…" when the combined length exceeds the budget.
func TestBuildTelegramRendering_SourcesPreservedUnderBudget(t *testing.T) {
	t.Parallel()
	bigBody := strings.Repeat("x", 200)
	resp := contracts.AssistantResponse{
		Body: bigBody,
		Sources: []contracts.Source{
			{
				ID:    "a",
				Title: "preserved title",
				Kind:  contracts.SourceArtifact,
				Ref: contracts.ArtifactRef{
					ArtifactID: "abcdef0011112222",
					CapturedAt: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 100)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(rendered, "preserved title") {
		t.Errorf("rendered = %q; want sources block preserved", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Errorf("rendered = %q; want truncation indicator", rendered)
	}
	if runeLen(rendered) > 100 {
		t.Errorf("rendered length = %d; want ≤ 100", runeLen(rendered))
	}
}

// TestBuildTelegramRendering_MarkdownV2Escaping asserts MarkdownV2
// mode escapes the closed character set (per Telegram bot API docs).
func TestBuildTelegramRendering_MarkdownV2Escaping(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "weather: cloudy. high 12°C.",
	}
	rendered, _, err := buildTelegramRendering(resp, MarkdownV2, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Period and colon-period combinations must be escaped (period is
	// in the closed escape set; colon is NOT).
	if !strings.Contains(rendered, `12°C\.`) {
		t.Errorf("rendered = %q; want escaped period after 12°C", rendered)
	}
}

// TestBuildTelegramRendering_SilentCaptureNoBody asserts the silent
// capture path produces empty rendered output AND no keyboard, so
// renderOutbound short-circuits the Telegram send.
func TestBuildTelegramRendering_SilentCaptureNoBody(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		CaptureRoute: true,
		// Body intentionally empty: low-band capture with no user-facing ack.
	}
	rendered, keyboard, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rendered != "" {
		t.Errorf("rendered = %q; want empty (silent capture)", rendered)
	}
	if keyboard != nil {
		t.Errorf("keyboard = %v; want nil", keyboard)
	}
}

// --- SCOPE-13: open_knowledge dispatch ---

// TestBuildTelegramRendering_AllArtifact_BackCompat asserts an
// AssistantResponse whose Sources are all SourceArtifact renders
// through the existing path (body + trailing "sources:" block),
// NOT through the open_knowledge renderer. Guards the spec 061
// back-compat contract.
func TestBuildTelegramRendering_AllArtifact_BackCompat(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "From your notes.",
		Sources: []contracts.Source{
			{
				ID:    "11111111-2222",
				Title: "Note A",
				Kind:  contracts.SourceArtifact,
				Ref: contracts.ArtifactRef{
					ArtifactID: "11111111-2222",
					CapturedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
				},
			},
		},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(rendered, "sources:") {
		t.Errorf("rendered = %q; want existing trailing 'sources:' block", rendered)
	}
	if strings.Contains(rendered, "[1]") {
		t.Errorf("rendered = %q; must NOT contain '[1]' inline citation (open_knowledge renderer should not fire)", rendered)
	}
	if strings.Contains(rendered, "(saved as idea)") {
		t.Errorf("rendered = %q; must NOT contain refusal suffix", rendered)
	}
}

// TestBuildTelegramRendering_OpenKnowledge_AllWeb routes through
// RenderSourcedAnswer when every Source is SourceWeb.
func TestBuildTelegramRendering_OpenKnowledge_AllWeb(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "Pad Thai uses tamarind.",
		Sources: []contracts.Source{
			{
				ID:    "https://example.com/p",
				Title: "Pad Thai 101",
				Kind:  contracts.SourceWeb,
				Ref: contracts.WebSourceRef{
					URL:         "https://example.com/p",
					Provider:    "searxng",
					ContentHash: "h",
				},
			},
		},
	}
	rendered, kb, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if kb != nil {
		t.Errorf("keyboard = %v; want nil", kb)
	}
	if !strings.Contains(rendered, "[1] Pad Thai 101 — example.com (web)") {
		t.Errorf("rendered = %q; want open_knowledge inline citation", rendered)
	}
	if strings.Contains(rendered, "sources:") {
		t.Errorf("rendered = %q; must NOT contain legacy trailing 'sources:' block", rendered)
	}
}

// TestBuildTelegramRendering_OpenKnowledge_AllComputation routes
// through RenderSourcedAnswer when every Source is
// SourceToolComputation.
func TestBuildTelegramRendering_OpenKnowledge_AllComputation(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "Result is 42.",
		Sources: []contracts.Source{
			{
				ID:    "calculator",
				Title: "calculator",
				Kind:  contracts.SourceToolComputation,
				Ref: contracts.ComputationSourceRef{
					Tool:       "calculator",
					InputHash:  "ih",
					OutputHash: "oh",
				},
			},
		},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(rendered, "[1] computed via calculator (computed)") {
		t.Errorf("rendered = %q; want computation citation", rendered)
	}
	if strings.Contains(rendered, "sources:") {
		t.Errorf("rendered = %q; must NOT contain legacy 'sources:' block", rendered)
	}
}

// TestBuildTelegramRendering_OpenKnowledge_Hybrid routes through
// RenderHybridAnswer when Sources mixes SourceArtifact with at least
// one non-artifact kind.
func TestBuildTelegramRendering_OpenKnowledge_Hybrid(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "Mixed answer.",
		Sources: []contracts.Source{
			{
				ID:    "aaaa",
				Title: "My note",
				Kind:  contracts.SourceArtifact,
				Ref:   contracts.ArtifactRef{ArtifactID: "aaaa"},
			},
			{
				ID:    "https://example.com/x",
				Title: "Web title",
				Kind:  contracts.SourceWeb,
				Ref: contracts.WebSourceRef{
					URL: "https://example.com/x", Provider: "searxng", ContentHash: "h",
				},
			},
		},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(rendered, "from your graph:") {
		t.Errorf("rendered = %q; want hybrid 'from your graph:' group marker", rendered)
	}
	if !strings.Contains(rendered, "from the web:") {
		t.Errorf("rendered = %q; want hybrid 'from the web:' group marker", rendered)
	}
}

// TestBuildTelegramRendering_OpenKnowledge_RefusalCauses exhaustively
// covers every non-default RefusalCause and asserts the captured
// refusal body is rendered.
func TestBuildTelegramRendering_OpenKnowledge_RefusalCauses(t *testing.T) {
	t.Parallel()
	cases := []contracts.RefusalCause{
		contracts.RefusalBudgetExhausted,
		contracts.RefusalToolUnavailable,
		contracts.RefusalFabricatedSourceBlocked,
		contracts.RefusalInternalOnlyRestricted,
		contracts.RefusalAmbiguousNotClarified,
	}
	for _, c := range cases {
		c := c
		t.Run(string(c), func(t *testing.T) {
			t.Parallel()
			resp := contracts.AssistantResponse{
				Status:     contracts.StatusUnavailable,
				ErrorCause: contracts.ErrorCause(string(c)),
			}
			rendered, kb, err := buildTelegramRendering(resp, PlainText, 4096)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if kb != nil {
				t.Errorf("keyboard = %v; want nil", kb)
			}
			wantBody := contracts.CanonicalRefusalBodyFor(c)
			if !strings.Contains(rendered, wantBody) {
				t.Errorf("rendered = %q; want to contain canonical body %q", rendered, wantBody)
			}
			if !strings.HasSuffix(rendered, OpenKnowledgeCaptureSuffix) {
				t.Errorf("rendered = %q; want suffix %q", rendered, OpenKnowledgeCaptureSuffix)
			}
		})
	}
}

// TestBuildTelegramRendering_LegacyErrorCause_BackCompat asserts the
// spec 061 error vocabulary still routes through the single-line
// "<skill>: <cause>" renderer, NOT the captured refusal.
func TestBuildTelegramRendering_LegacyErrorCause_BackCompat(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Status:     contracts.StatusUnavailable,
		ErrorCause: contracts.ErrProviderUnavailable,
		Routing:    &agent.RoutingDecision{Chosen: "weather"},
	}
	rendered, _, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if rendered != "weather: provider_unavailable" {
		t.Errorf("rendered = %q; want existing single-line error rendering", rendered)
	}
	if strings.Contains(rendered, OpenKnowledgeCaptureSuffix) {
		t.Errorf("rendered = %q; must NOT contain refusal suffix (legacy cause)", rendered)
	}
}

// TestBuildTelegramRendering_G021_NoSourcesNoErrorCause is the
// adversarial back-compat guard: when an AssistantResponse arrives
// with neither sources nor any ErrorCause, the open_knowledge
// dispatch MUST NOT fire — the existing default body-only rendering
// remains. Without this assertion the dispatch could silently engage
// on any uncategorised response and corrupt spec 061 surfaces.
func TestBuildTelegramRendering_G021_NoSourcesNoErrorCauseFallsThroughDefault(t *testing.T) {
	t.Parallel()
	resp := contracts.AssistantResponse{
		Body: "ok",
		// Status zero-value, ErrorCause "", Sources nil — strictly
		// the legacy "default" case.
	}
	rendered, kb, err := buildTelegramRendering(resp, PlainText, 4096)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if kb != nil {
		t.Errorf("keyboard = %v; want nil", kb)
	}
	if rendered != "ok" {
		t.Errorf("rendered = %q; want %q (default body-only render)", rendered, "ok")
	}
	if strings.Contains(rendered, OpenKnowledgeCaptureSuffix) {
		t.Errorf("rendered must NOT contain refusal suffix when ErrorCause is unset")
	}
	if strings.Contains(rendered, "[1]") {
		t.Errorf("rendered must NOT contain inline citation markers when Sources is empty")
	}
}
