package assistant_adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// renderOutbound is the pure function under TransportAdapter.Render
// (chat-id-aware via the supplied chatID). It is decoupled from the
// Adapter type so golden tests can drive it directly without
// constructing an Adapter.
//
// Contract (spec.md §14.B.1 — binding Telegram rendering table):
//
//   - First line: status prefix when applicable (see statusPrefix).
//   - Body: MarkdownV2-escaped when MarkdownMode is MarkdownV2.
//   - Sources block: trailing numbered list, see render_sources.go.
//   - ConfirmCard: inline keyboard [✅ pos][❌ neg]; callback_data
//     carries the ConfirmRef.
//   - DisambiguationPrompt: numbered choices in body, optional inline
//     keyboard when len(choices) ≤ 3.
//   - StatusUnavailable: single-line error rendering.
//   - CaptureRoute: invoke the bot-side CaptureFn (legacy
//     handleTextCapture path) AND skip the user-facing send when the
//     body is empty (silent capture); when body is non-empty, send
//     the body AND capture (capture-as-fallback for low-confidence
//     intent that still wants a friendly ack).
func renderOutbound(
	ctx context.Context,
	sender Sender,
	chatID int64,
	mode MarkdownMode,
	maxMessageChars int,
	resp contracts.AssistantResponse,
) error {
	if sender == nil {
		return errors.New("assistant_adapter: renderOutbound called with nil sender")
	}
	if chatID == 0 {
		return errors.New("assistant_adapter: renderOutbound called with zero chatID")
	}
	if maxMessageChars <= 0 {
		return fmt.Errorf("assistant_adapter: maxMessageChars must be > 0 (got %d)", maxMessageChars)
	}

	// CaptureRoute dispatch is owned by HandleUpdate (it has access to
	// the inbound text); renderOutbound is responsible only for the
	// user-facing send. When the capability layer chose to capture
	// silently with no body, the renderer skips the send entirely
	// (see the empty-rendered short-circuit below).

	rendered, keyboard, err := buildTelegramRendering(resp, mode, maxMessageChars)
	if err != nil {
		return err
	}

	// Silent path: nothing to send when the capability layer chose to
	// capture without surfacing a status to the user.
	if strings.TrimSpace(rendered) == "" && keyboard == nil {
		return nil
	}

	msg := tgbotapi.NewMessage(chatID, rendered)
	switch mode {
	case MarkdownV2:
		msg.ParseMode = tgbotapi.ModeMarkdownV2
	case HTML:
		msg.ParseMode = tgbotapi.ModeHTML
	case PlainText:
		// leave unset
	}
	if keyboard != nil {
		msg.ReplyMarkup = *keyboard
	}
	if _, err := sender.Send(msg); err != nil {
		return fmt.Errorf("telegram send: %w", err)
	}
	return nil
}

// buildTelegramRendering composes the message body + (optional)
// inline keyboard for an AssistantResponse. It is pure (no I/O) so
// golden tests can compare its output verbatim against spec.md §14.B.1
// expected strings.
func buildTelegramRendering(
	resp contracts.AssistantResponse,
	mode MarkdownMode,
	maxMessageChars int,
) (string, *tgbotapi.InlineKeyboardMarkup, error) {
	if resp.ConfirmCard != nil && resp.DisambiguationPrompt != nil {
		return "", nil, errors.New("assistant_adapter: response has both ConfirmCard and DisambiguationPrompt (capability contract violation)")
	}

	// Spec 064 SCOPE-13 — open_knowledge dispatch. Routed strictly by
	// AssistantResponse content composition so the AssistantResponse
	// shape (spec 061) is not extended:
	//
	//   - ErrorCause string matching a non-default spec 064
	//     RefusalCause → RenderRefusalWithCapture.
	//   - Otherwise, when Sources contains at least one
	//     non-SourceArtifact kind → RenderSourcedAnswer (all
	//     non-artifact) or RenderHybridAnswer (mixed with artifact).
	//
	// All-artifact source sets and existing spec 061 ErrorCause
	// values fall through to the unchanged default rendering below
	// (backward compatibility).
	if cause, ok := openKnowledgeRefusalCauseFromError(resp.ErrorCause); ok {
		body := RenderRefusalWithCapture(cause)
		rendered := budgetTruncate(escapeForMode(body, mode), maxMessageChars, mode)
		return rendered, nil, nil
	}
	if resp.Status != contracts.StatusUnavailable &&
		resp.ConfirmCard == nil && resp.DisambiguationPrompt == nil &&
		hasNonArtifactSources(resp.Sources) {
		var (
			okOut string
			err   error
		)
		if hasArtifactSource(resp.Sources) {
			okOut, err = RenderHybridAnswer(resp.Body, resp.Sources)
		} else {
			okOut, err = RenderSourcedAnswer(resp.Body, resp.Sources)
		}
		if err != nil {
			return "", nil, err
		}
		var headParts []string
		if prefix := statusPrefix(resp); prefix != "" {
			headParts = append(headParts, prefix)
		}
		headParts = append(headParts, escapeForMode(okOut, mode))
		rendered := joinAndBudget(headParts, "", maxMessageChars, mode)
		return rendered, nil, nil
	}

	var parts []string
	if prefix := statusPrefix(resp); prefix != "" {
		parts = append(parts, prefix)
	}

	switch {
	case resp.Status == contracts.StatusUnavailable:
		// Single-line error rendering, no body, no sources.
		errLine := renderError(resp)
		if errLine != "" {
			parts = append(parts, errLine)
		}
	case resp.DisambiguationPrompt != nil:
		body := strings.TrimSpace(resp.Body)
		if body != "" {
			parts = append(parts, escapeForMode(body, mode))
		}
		parts = append(parts, renderDisambigBody(resp.DisambiguationPrompt, mode))
	case resp.ConfirmCard != nil:
		body := strings.TrimSpace(resp.Body)
		if body != "" {
			parts = append(parts, escapeForMode(body, mode))
		}
		propose := strings.TrimSpace(resp.ConfirmCard.ProposedAction)
		if propose != "" {
			parts = append(parts, escapeForMode(propose, mode))
		}
	default:
		body := strings.TrimSpace(resp.Body)
		if body != "" {
			parts = append(parts, escapeForMode(body, mode))
		}
	}

	sourcesBlock := renderSourcesBlock(resp.Sources, resp.SourcesOverflowCount, mode)
	rendered := joinAndBudget(parts, sourcesBlock, maxMessageChars, mode)

	var keyboard *tgbotapi.InlineKeyboardMarkup
	switch {
	case resp.ConfirmCard != nil:
		k := renderConfirmKeyboard(resp.ConfirmCard)
		keyboard = &k
	case resp.DisambiguationPrompt != nil && len(resp.DisambiguationPrompt.Choices) <= 3 && len(resp.DisambiguationPrompt.Choices) > 0:
		k := renderDisambigKeyboard(resp.DisambiguationPrompt)
		keyboard = &k
	}

	return rendered, keyboard, nil
}

// statusPrefix returns the first-line status string for in-flight
// status tokens per spec.md §14.B.1. Terminal-of-turn tokens
// (reminder_proposed, reminder_confirmed, reminder_cancelled,
// saved_as_idea, unavailable) render no prefix — they are
// communicated via body/keyboard/error.
func statusPrefix(resp contracts.AssistantResponse) string {
	switch resp.Status {
	case contracts.StatusThinking:
		return "thinking…"
	case contracts.StatusAnswered:
		// BUG-064-002 DEFECT 3a — terminal answer: NO status prefix. A
		// delivered open_knowledge answer must not show a "thinking…"
		// header.
		return ""
	case contracts.StatusCheckingWeather:
		return "checking weather…"
	case contracts.StatusCheckingEmail:
		return "checking email…"
	case contracts.StatusSavedAsIdea:
		// terminal: rendered as a short ack body line; no prefix.
		return ""
	default:
		return ""
	}
}

// renderError formats StatusUnavailable as the single-line
// "<skill>: <cause>" form. When the response carries no Routing
// (skill name), the cause alone is rendered.
func renderError(resp contracts.AssistantResponse) string {
	cause := string(resp.ErrorCause)
	if cause == "" {
		cause = "unavailable"
	}
	skill := ""
	if resp.Routing != nil {
		skill = strings.TrimSpace(resp.Routing.Chosen)
	}
	if skill == "" {
		return cause
	}
	return fmt.Sprintf("%s: %s", skill, cause)
}

// escapeForMode runs Telegram MarkdownV2 escaping when needed.
// Per the official Telegram bot API: the chars `_*[]()~`>#+-=|{}.!`
// MUST be escaped with a leading backslash inside a MarkdownV2
// message body. We deliberately do NOT escape backticks here so the
// caller can embed an inline code block when desired; v1 renderers
// never do.
func escapeForMode(s string, mode MarkdownMode) string {
	if mode != MarkdownV2 {
		return s
	}
	return escapeMarkdownV2(s)
}

// markdownV2EscapeChars is the closed character set per Telegram bot
// API docs. Order is irrelevant; a single pass with strings.NewReplacer
// is sufficient.
var markdownV2EscapeChars = []string{
	"_", "\\_",
	"*", "\\*",
	"[", "\\[",
	"]", "\\]",
	"(", "\\(",
	")", "\\)",
	"~", "\\~",
	"`", "\\`",
	">", "\\>",
	"#", "\\#",
	"+", "\\+",
	"-", "\\-",
	"=", "\\=",
	"|", "\\|",
	"{", "\\{",
	"}", "\\}",
	".", "\\.",
	"!", "\\!",
}

var markdownV2Replacer = strings.NewReplacer(markdownV2EscapeChars...)

func escapeMarkdownV2(s string) string {
	return markdownV2Replacer.Replace(s)
}

// joinAndBudget composes the final outbound text from the head
// segments (status prefix, body, etc.) and the trailing sources
// block, then enforces the maxMessageChars budget per §14.B.1:
//
//   - The sources block is preserved verbatim (truncation of
//     provenance is forbidden).
//   - When the head + sources block overflows the budget, the head
//     is truncated with a trailing "…" character.
//   - When the sources block alone overflows the budget, the head
//     is dropped and the sources block is sent on its own (extreme
//     edge case; preserves provenance per §14.A.4).
func joinAndBudget(headParts []string, sourcesBlock string, maxMessageChars int, mode MarkdownMode) string {
	head := strings.Join(filterEmpty(headParts), "\n")
	if sourcesBlock == "" {
		return budgetTruncate(head, maxMessageChars, mode)
	}
	separator := "\n\n"
	totalChars := runeLen(head) + runeLen(separator) + runeLen(sourcesBlock)
	if totalChars <= maxMessageChars {
		if head == "" {
			return sourcesBlock
		}
		return head + separator + sourcesBlock
	}
	// Need to truncate the head. Compute available budget.
	available := maxMessageChars - runeLen(sourcesBlock) - runeLen(separator)
	if available <= 0 {
		// Sources block alone is at or over the budget — drop head.
		return sourcesBlock
	}
	truncated := budgetTruncate(head, available, mode)
	return truncated + separator + sourcesBlock
}

// budgetTruncate trims s to at most maxChars runes, appending the
// MarkdownV2-safe ellipsis "…" when truncation occurs. When
// maxChars <= 1 the function returns at most a single "…" rune.
func budgetTruncate(s string, maxChars int, mode MarkdownMode) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	if maxChars == 1 {
		return "…"
	}
	// Reserve 1 rune for the ellipsis.
	return string(runes[:maxChars-1]) + "…"
}

func runeLen(s string) int { return len([]rune(s)) }

func filterEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// hasNonArtifactSources reports whether sources contains at least one
// Source whose Kind is not SourceArtifact. Used by the open_knowledge
// dispatch (SCOPE-13) — spec 061 scenarios only ever produce
// SourceArtifact, so any non-artifact kind unambiguously signals an
// open_knowledge response.
func hasNonArtifactSources(sources []contracts.Source) bool {
	for _, s := range sources {
		if s.Kind != contracts.SourceArtifact {
			return true
		}
	}
	return false
}

// hasArtifactSource reports whether sources contains at least one
// SourceArtifact. Combined with hasNonArtifactSources it lets the
// dispatch pick RenderHybridAnswer (mixed) vs RenderSourcedAnswer
// (all non-artifact).
func hasArtifactSource(sources []contracts.Source) bool {
	for _, s := range sources {
		if s.Kind == contracts.SourceArtifact {
			return true
		}
	}
	return false
}

// openKnowledgeRefusalCauseFromError reports whether the ErrorCause
// string matches a spec 064 RefusalCause value (excluding
// RefusalDefault, which has the same body as the legacy spec 061
// canonical refusal and does not warrant the captured-refusal UX).
// Returns the matching RefusalCause and true on hit; zero-value and
// false otherwise.
func openKnowledgeRefusalCauseFromError(ec contracts.ErrorCause) (contracts.RefusalCause, bool) {
	s := string(ec)
	if s == "" {
		return "", false
	}
	for _, c := range contracts.AllRefusalCauses {
		if c == contracts.RefusalDefault {
			continue
		}
		if string(c) == s {
			return c, true
		}
	}
	return "", false
}
