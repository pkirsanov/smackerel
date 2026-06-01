// Spec 072 SCOPE-2 — WhatsApp Business outbound renderer.
//
// Render is a pure function that maps an AssistantResponse into one
// of three WhatsApp message families (text, interactive buttons,
// interactive list) per design.md §"Outbound Render Mapping". It
// performs zero I/O so the golden tests (render_golden_test.go) can
// drive it directly without a CloudClient. The adapter's Render
// method (adapter.go) is the I/O wrapper that dispatches the
// rendered OutboundMessage through the configured CloudClient.
//
// Template messages are intentionally NOT representable from this
// renderer: there is no template kind in OutboundMessageKind, and
// no field on TextMessage/InteractiveMessage that carries a template
// name. SCN-072-A09 ("no silent template wrapping") is enforced
// structurally — the only way to send a template would be a new
// surface added by an explicit operator-runbook flow (out of scope
// for this spec).

package assistant_adapter

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// OutboundMessageKind discriminates the three rendered WhatsApp
// message families. Closed vocabulary.
type OutboundMessageKind string

const (
	OutboundText               OutboundMessageKind = "text"
	OutboundInteractiveButtons OutboundMessageKind = "interactive_buttons"
	OutboundInteractiveList    OutboundMessageKind = "interactive_list"
)

// TextMessage is a plain WhatsApp text message.
type TextMessage struct {
	// Body is the rendered user-facing string already truncated to
	// MaxTextChars.
	Body string
}

// Button is one interactive-button choice. ID is the opaque
// round-trip payload carried back by the user's reply; Title is the
// short human-readable label.
type Button struct {
	ID    string
	Title string
}

// ListRow is one selectable row inside a list section.
type ListRow struct {
	ID    string
	Title string
}

// ListSection groups list rows under a header.
type ListSection struct {
	Title string
	Rows  []ListRow
}

// InteractiveMessage is the buttons- or list-shaped WhatsApp
// interactive message.
type InteractiveMessage struct {
	Kind         OutboundMessageKind // OutboundInteractiveButtons | OutboundInteractiveList
	Body         string
	Buttons      []Button      // Kind == OutboundInteractiveButtons
	ListButton   string        // Kind == OutboundInteractiveList — CTA label
	ListSections []ListSection // Kind == OutboundInteractiveList
}

// OutboundMessage is the tagged-union return of Render.
type OutboundMessage struct {
	Kind        OutboundMessageKind
	Text        *TextMessage
	Interactive *InteractiveMessage
}

// ErrNothingToRender is returned when the response has no body, no
// confirm card, no disambiguation prompt, and no error to surface —
// a structurally-empty response that must not be silently dropped
// per design.md "unknown response shape: text rendering of Body; if
// body empty, fail render with observable error".
var ErrNothingToRender = errors.New("whatsapp_adapter: response has no body, prompt, confirm, or error to render")

// payload-id encoding constants — opaque to WhatsApp, decoded by
// Translate on the inbound round-trip.
const (
	disambigPayloadPrefix = "d:"
	confirmPayloadPrefix  = "c:"
	resetPayloadPrefix    = "r:"
	confirmChoiceYes      = "y"
	confirmChoiceNo       = "n"
	listButtonCTA         = "Choose"
	defaultListSection    = "Options"
)

// ResetTextCommand is the canonical inbound text token that mirrors
// the Telegram `/reset` command. Comparison is case-insensitive and
// trims surrounding whitespace.
const ResetTextCommand = "/reset"

// EncodeResetPayload produces the opaque button id for a reset
// shortcut. The optional ref carries a renderer-supplied
// correlation token; callers MAY pass "" when no correlation is
// required.
func EncodeResetPayload(ref string) string {
	return resetPayloadPrefix + ref
}

// DecodeResetPayload reverses EncodeResetPayload. Returns
// (ref, true) on success; (_, false) when the payload does not look
// like a reset round-trip id.
func DecodeResetPayload(id string) (string, bool) {
	if !strings.HasPrefix(id, resetPayloadPrefix) {
		return "", false
	}
	return id[len(resetPayloadPrefix):], true
}

// EncodeDisambigPayload produces the opaque button/list-row id used
// to carry (disambiguation_ref, 1-indexed choice number) back through
// WhatsApp without exposing user text.
func EncodeDisambigPayload(ref string, choiceNumber int) string {
	return disambigPayloadPrefix + ref + ":" + strconv.Itoa(choiceNumber)
}

// DecodeDisambigPayload reverses EncodeDisambigPayload. Returns
// (ref, choice, true) on success; (_, _, false) when the payload
// does not look like a disambiguation round-trip id.
func DecodeDisambigPayload(id string) (string, int, bool) {
	if !strings.HasPrefix(id, disambigPayloadPrefix) {
		return "", 0, false
	}
	rest := id[len(disambigPayloadPrefix):]
	idx := strings.LastIndex(rest, ":")
	if idx <= 0 || idx == len(rest)-1 {
		return "", 0, false
	}
	ref := rest[:idx]
	n, err := strconv.Atoi(rest[idx+1:])
	if err != nil || n < 1 {
		return "", 0, false
	}
	return ref, n, true
}

// EncodeConfirmPayload produces the opaque button id for a confirm
// card response.
func EncodeConfirmPayload(ref string, positive bool) string {
	choice := confirmChoiceNo
	if positive {
		choice = confirmChoiceYes
	}
	return confirmPayloadPrefix + ref + ":" + choice
}

// DecodeConfirmPayload reverses EncodeConfirmPayload. Returns
// (ref, positive, true) on success.
func DecodeConfirmPayload(id string) (string, bool, bool) {
	if !strings.HasPrefix(id, confirmPayloadPrefix) {
		return "", false, false
	}
	rest := id[len(confirmPayloadPrefix):]
	idx := strings.LastIndex(rest, ":")
	if idx <= 0 || idx == len(rest)-1 {
		return "", false, false
	}
	ref := rest[:idx]
	switch rest[idx+1:] {
	case confirmChoiceYes:
		return ref, true, true
	case confirmChoiceNo:
		return ref, false, true
	default:
		return "", false, false
	}
}

// Render maps an AssistantResponse to an OutboundMessage. maxTextChars
// MUST be > 0 (SST-supplied per-message cap).
//
// Mapping (design.md):
//
//   - StatusUnavailable                 → text (single-line error)
//   - DisambiguationPrompt 1..3 choices → interactive_buttons
//   - DisambiguationPrompt 4..10        → interactive_list
//   - DisambiguationPrompt >10          → text body with numbered choices
//   - ConfirmCard                       → interactive_buttons (positive/negative)
//   - CaptureRoute / plain body         → text
//   - empty body + nothing else         → ErrNothingToRender
func Render(resp contracts.AssistantResponse, maxTextChars int) (OutboundMessage, error) {
	if maxTextChars <= 0 {
		return OutboundMessage{}, fmt.Errorf("whatsapp_adapter: maxTextChars must be > 0 (got %d)", maxTextChars)
	}
	if resp.ConfirmCard != nil && resp.DisambiguationPrompt != nil {
		return OutboundMessage{}, errors.New("whatsapp_adapter: response has both ConfirmCard and DisambiguationPrompt")
	}

	var (
		out OutboundMessage
		err error
	)
	switch {
	case resp.Status == contracts.StatusUnavailable:
		body := strings.TrimSpace(resp.Body)
		if body == "" {
			body = "Something went wrong and I couldn't complete that."
		}
		out = textOutbound(body, resp.Sources, resp.SourcesOverflowCount, maxTextChars)

	case resp.DisambiguationPrompt != nil:
		out, err = renderDisambiguation(resp, maxTextChars)

	case resp.ConfirmCard != nil:
		out, err = renderConfirm(resp, maxTextChars)

	default:
		body := strings.TrimSpace(resp.Body)
		if body == "" {
			return OutboundMessage{}, ErrNothingToRender
		}
		out = textOutbound(body, resp.Sources, resp.SourcesOverflowCount, maxTextChars)
	}
	if err != nil {
		return OutboundMessage{}, err
	}
	// Spec 075 SCOPE-6.4 (TP-075-21) — WhatsApp legacy-retirement
	// notice addendum. Appended as a one-line short-message tail
	// to the rendered body (text or interactive). Non-blocking:
	// the primary response surface is preserved.
	if resp.LegacyRetirementNotice != nil {
		appendLegacyRetirementNoticeAddendum(&out, resp.LegacyRetirementNotice, maxTextChars)
	}
	return out, nil
}

// LegacyRetirementNoticeAddendum returns the canonical short-message
// addendum derived from a NoticePayload. Exported so tests can pin
// the rendered shape across transports.
func LegacyRetirementNoticeAddendum(np *contracts.NoticePayload) string {
	if np == nil {
		return ""
	}
	cmd := strings.TrimSpace(np.Command)
	ex := strings.TrimSpace(np.ReplacementExample)
	if cmd == "" || ex == "" {
		return ""
	}
	return fmt.Sprintf("Heads up: %s is retiring — try \"%s\" instead.", cmd, ex)
}

func appendLegacyRetirementNoticeAddendum(out *OutboundMessage, np *contracts.NoticePayload, maxTextChars int) {
	addendum := LegacyRetirementNoticeAddendum(np)
	if addendum == "" {
		return
	}
	switch out.Kind {
	case OutboundText:
		if out.Text != nil {
			out.Text.Body = truncateBody(out.Text.Body+"\n\n"+addendum, maxTextChars)
		}
	case OutboundInteractiveButtons, OutboundInteractiveList:
		if out.Interactive != nil {
			out.Interactive.Body = truncateBody(out.Interactive.Body+"\n\n"+addendum, maxTextChars)
		}
	}
}

func renderDisambiguation(resp contracts.AssistantResponse, maxTextChars int) (OutboundMessage, error) {
	prompt := resp.DisambiguationPrompt
	if len(prompt.Choices) == 0 {
		return OutboundMessage{}, errors.New("whatsapp_adapter: DisambiguationPrompt has zero choices")
	}

	body := strings.TrimSpace(resp.Body)
	if body == "" {
		body = "Which one?"
	}

	switch {
	case len(prompt.Choices) <= 3:
		buttons := make([]Button, 0, len(prompt.Choices))
		for _, c := range prompt.Choices {
			buttons = append(buttons, Button{
				ID:    EncodeDisambigPayload(prompt.DisambiguationRef, c.Number),
				Title: truncateLabel(c.Label, 20),
			})
		}
		return OutboundMessage{
			Kind: OutboundInteractiveButtons,
			Interactive: &InteractiveMessage{
				Kind:    OutboundInteractiveButtons,
				Body:    truncateBody(body, maxTextChars),
				Buttons: buttons,
			},
		}, nil

	case len(prompt.Choices) <= 10:
		rows := make([]ListRow, 0, len(prompt.Choices))
		for _, c := range prompt.Choices {
			rows = append(rows, ListRow{
				ID:    EncodeDisambigPayload(prompt.DisambiguationRef, c.Number),
				Title: truncateLabel(c.Label, 24),
			})
		}
		return OutboundMessage{
			Kind: OutboundInteractiveList,
			Interactive: &InteractiveMessage{
				Kind:         OutboundInteractiveList,
				Body:         truncateBody(body, maxTextChars),
				ListButton:   listButtonCTA,
				ListSections: []ListSection{{Title: defaultListSection, Rows: rows}},
			},
		}, nil

	default:
		var b strings.Builder
		b.WriteString(body)
		b.WriteString("\n")
		for _, c := range prompt.Choices {
			fmt.Fprintf(&b, "\n%d. %s", c.Number, c.Label)
		}
		b.WriteString("\n\nReply with the number of your choice.")
		return textOutbound(b.String(), nil, 0, maxTextChars), nil
	}
}

func renderConfirm(resp contracts.AssistantResponse, maxTextChars int) (OutboundMessage, error) {
	card := resp.ConfirmCard
	positive := strings.TrimSpace(card.PositiveLabel)
	negative := strings.TrimSpace(card.NegativeLabel)
	if positive == "" || negative == "" {
		return OutboundMessage{}, errors.New("whatsapp_adapter: ConfirmCard requires non-empty PositiveLabel and NegativeLabel")
	}

	var b strings.Builder
	if body := strings.TrimSpace(resp.Body); body != "" {
		b.WriteString(body)
	}
	if propose := strings.TrimSpace(card.ProposedAction); propose != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(propose)
	}
	if b.Len() == 0 {
		return OutboundMessage{}, errors.New("whatsapp_adapter: ConfirmCard has empty body and proposed action")
	}

	return OutboundMessage{
		Kind: OutboundInteractiveButtons,
		Interactive: &InteractiveMessage{
			Kind: OutboundInteractiveButtons,
			Body: truncateBody(b.String(), maxTextChars),
			Buttons: []Button{
				{ID: EncodeConfirmPayload(card.ConfirmRef, true), Title: truncateLabel(positive, 20)},
				{ID: EncodeConfirmPayload(card.ConfirmRef, false), Title: truncateLabel(negative, 20)},
			},
		},
	}, nil
}

func textOutbound(body string, sources []contracts.Source, overflow int, maxTextChars int) OutboundMessage {
	rendered := body
	if block := renderSourcesBlock(sources, overflow); block != "" {
		rendered = rendered + "\n\n" + block
	}
	return OutboundMessage{
		Kind: OutboundText,
		Text: &TextMessage{Body: truncateBody(rendered, maxTextChars)},
	}
}

func renderSourcesBlock(sources []contracts.Source, overflow int) string {
	if len(sources) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Sources:")
	for i, s := range sources {
		fmt.Fprintf(&b, "\n%d. %s", i+1, s.Title)
	}
	if overflow > 0 {
		fmt.Fprintf(&b, "\n…and %d more.", overflow)
	}
	return b.String()
}

// truncateBody trims the body to fit maxTextChars (rune-counted).
// On overflow it appends an ellipsis inside the budget so the message
// stays valid WhatsApp text.
func truncateBody(body string, maxTextChars int) string {
	if utf8.RuneCountInString(body) <= maxTextChars {
		return body
	}
	const ellipsis = "…"
	budget := maxTextChars - utf8.RuneCountInString(ellipsis)
	if budget <= 0 {
		runes := []rune(body)
		return string(runes[:maxTextChars])
	}
	runes := []rune(body)
	return string(runes[:budget]) + ellipsis
}

// truncateLabel trims interactive button/row titles to WhatsApp's
// per-element limit (20 for buttons, 24 for list rows).
func truncateLabel(label string, maxRunes int) string {
	if utf8.RuneCountInString(label) <= maxRunes {
		return label
	}
	runes := []rune(label)
	return string(runes[:maxRunes])
}
