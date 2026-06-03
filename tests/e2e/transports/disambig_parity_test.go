//go:build e2e

// Spec 076 SCOPE-7c — TP-076-07c-01 / SCN-073-A04.
//
// Cross-surface render-descriptor parity for the disambiguation
// prompt. The facade emits one AssistantResponse with a
// DisambiguationPrompt; each transport projects that response into a
// canonical render-descriptor whose (kind, ref, label, choice index)
// tuples MUST be byte-identical across web, Telegram, and WhatsApp.
//
// Sources of truth:
//
//   - Web: tests/fixtures/assistant_response_v1/disambiguation.input.json
//     paired with disambiguation.descriptor.json. The JS render CLI
//     (web/pwa/lib/render_descriptor_v1_cli.js) projects the input
//     into that descriptor; the unit canary
//     (tests/unit/clients/render_descriptor_canary_test.go) already
//     proves JS == golden.
//   - Telegram: internal/telegram/assistant_adapter.NewAdapter().RenderToChat
//     pushes a tgbotapi.MessageConfig whose body carries the prompt
//     line + numbered choice labels and whose inline keyboard carries
//     callback_data "a:d:<ref>:<number>".
//   - WhatsApp: internal/whatsapp/assistant_adapter.Render returns an
//     OutboundMessage whose interactive body == prompt body and whose
//     buttons carry EncodeDisambigPayload(ref, number) as ID with the
//     human label as Title.
//
// Adversarial coverage: if any transport drifted (button order
// swapped, ref renamed, choice number off-by-one, label munged), the
// canonical projection would diverge from the web descriptor and
// reflect.DeepEqual would trip.

package transports_e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	telegramadapter "github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	whatsappadapter "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// canonicalAction is the cross-transport projection of one
// actionable element in a render descriptor.
type canonicalAction struct {
	Kind  string // "disambiguation_choice" | "confirm_accept" | "confirm_decline"
	Ref   string
	Label string
	Index int // disambig choice number; 0 for confirm
}

// canonicalRender is the projected render-descriptor each transport
// must produce equivalently for parity.
type canonicalRender struct {
	PromptBody string
	Actions    []canonicalAction
}

// disambigButtonLine matches a Telegram body line "1. Palm Springs, CA".
var disambigButtonLine = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)

type captureSenderDisambig struct {
	lastText     string
	lastKeyboard *tgbotapi.InlineKeyboardMarkup
}

func (s *captureSenderDisambig) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if m, ok := c.(tgbotapi.MessageConfig); ok {
		s.lastText = m.Text
		if k, ok := m.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup); ok {
			s.lastKeyboard = &k
		}
	}
	return tgbotapi.Message{MessageID: 1}, nil
}

// TestDisambigParity_WebTelegramWhatsApp covers TP-076-07c-01 /
// SCN-073-A04.
func TestDisambigParity_WebTelegramWhatsApp(t *testing.T) {
	repoRoot := mustFindRepoRoot(t)
	inputPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/disambiguation.input.json")
	descriptorPath := filepath.Join(repoRoot, "tests/fixtures/assistant_response_v1/disambiguation.descriptor.json")

	resp := mustLoadAssistantResponseFromFixture(t, inputPath)
	if resp.DisambiguationPrompt == nil {
		t.Fatalf("fixture %s missing disambiguation_prompt", inputPath)
	}

	webRender := mustProjectWebDescriptor(t, descriptorPath)
	tgRender := projectTelegramDisambig(t, resp)
	waRender := projectWhatsAppDisambig(t, resp)

	if !reflect.DeepEqual(webRender, tgRender) {
		t.Fatalf("web vs telegram disambig render mismatch:\nweb=%+v\ntelegram=%+v",
			webRender, tgRender)
	}
	if !reflect.DeepEqual(webRender, waRender) {
		t.Fatalf("web vs whatsapp disambig render mismatch:\nweb=%+v\nwhatsapp=%+v",
			webRender, waRender)
	}
	if !reflect.DeepEqual(tgRender, waRender) {
		t.Fatalf("telegram vs whatsapp disambig render mismatch:\ntelegram=%+v\nwhatsapp=%+v",
			tgRender, waRender)
	}
}

// mustLoadAssistantResponseFromFixture decodes a spec 069
// assistant_turn_v1 input fixture into an internal AssistantResponse
// suitable for driving the Go transport renderers.
func mustLoadAssistantResponseFromFixture(t *testing.T, path string) contracts.AssistantResponse {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	var f fixtureInput
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("unmarshal fixture %s: %v", path, err)
	}
	resp := contracts.AssistantResponse{
		Body: f.Body,
	}
	if f.DisambiguationPrompt != nil {
		dp := contracts.DisambiguationPrompt{
			DisambiguationRef: f.DisambiguationPrompt.DisambiguationRef,
		}
		for _, c := range f.DisambiguationPrompt.Choices {
			dp.Choices = append(dp.Choices, contracts.DisambiguationChoice{
				Number:   c.Number,
				ID:       c.ID,
				Label:    c.Label,
				Shortcut: c.Shortcut,
			})
		}
		resp.DisambiguationPrompt = &dp
	}
	if f.ConfirmCard != nil {
		resp.ConfirmCard = &contracts.ConfirmCard{
			ProposedAction: f.ConfirmCard.ProposedAction,
			ConfirmRef:     f.ConfirmCard.ConfirmRef,
			PositiveLabel:  f.ConfirmCard.PositiveLabel,
			NegativeLabel:  f.ConfirmCard.NegativeLabel,
		}
	}
	switch f.Status {
	case "captured":
		resp.Status = contracts.StatusSavedAsIdea
		resp.CaptureRoute = true
	}
	return resp
}

type fixtureInput struct {
	Status               string                       `json:"status"`
	Body                 string                       `json:"body"`
	ConfirmCard          *fixtureConfirmCard          `json:"confirm_card"`
	DisambiguationPrompt *fixtureDisambiguationPrompt `json:"disambiguation_prompt"`
	CaptureRoute         bool                         `json:"capture_route"`
}

type fixtureConfirmCard struct {
	ProposedAction string `json:"proposed_action"`
	ConfirmRef     string `json:"confirm_ref"`
	PositiveLabel  string `json:"positive_label"`
	NegativeLabel  string `json:"negative_label"`
}

type fixtureDisambiguationPrompt struct {
	DisambiguationRef string             `json:"disambiguation_ref"`
	Choices           []fixtureChoiceRow `json:"choices"`
}

type fixtureChoiceRow struct {
	Number   int    `json:"number"`
	ID       string `json:"id"`
	Label    string `json:"label"`
	Shortcut string `json:"shortcut"`
}

// descriptor JSON shape — minimal subset needed for parity projection.
type descriptorFile struct {
	Nodes []descriptorNode `json:"nodes"`
}

type descriptorNode struct {
	Kind        string `json:"kind"`
	Text        string `json:"text"`
	ActionKind  string `json:"action_kind"`
	Ref         string `json:"ref"`
	Label       string `json:"label"`
	ChoiceIndex int    `json:"choice_index"`
}

func mustProjectWebDescriptor(t *testing.T, path string) canonicalRender {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read descriptor %s: %v", path, err)
	}
	var d descriptorFile
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("unmarshal descriptor %s: %v", path, err)
	}
	r := canonicalRender{}
	for _, n := range d.Nodes {
		switch n.Kind {
		case "text":
			if r.PromptBody == "" {
				r.PromptBody = n.Text
			}
		case "action":
			r.Actions = append(r.Actions, canonicalAction{
				Kind:  n.ActionKind,
				Ref:   n.Ref,
				Label: n.Label,
				Index: n.ChoiceIndex,
			})
		}
	}
	return r
}

func projectTelegramDisambig(t *testing.T, resp contracts.AssistantResponse) canonicalRender {
	t.Helper()
	sender := &captureSenderDisambig{}
	adapter, err := telegramadapter.NewAdapter(telegramadapter.Options{
		Sender:          sender,
		Capture:         func(context.Context, *tgbotapi.Message, string) {},
		ResolveUser:     func(int64) (string, error) { return "test-user", nil },
		MarkdownMode:    telegramadapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter(telegram): %v", err)
	}
	if err := adapter.RenderToChat(context.Background(), 12345, resp); err != nil {
		t.Fatalf("Telegram RenderToChat: %v", err)
	}
	if sender.lastKeyboard == nil {
		t.Fatalf("Telegram render produced no inline keyboard for disambig")
	}

	// Body lines: first paragraph is the prompt body; following
	// numbered lines carry the choice labels.
	lines := strings.Split(sender.lastText, "\n")
	if len(lines) == 0 {
		t.Fatalf("Telegram rendered text is empty")
	}
	promptBody := lines[0]

	labels := map[int]string{}
	for _, line := range lines[1:] {
		m := disambigButtonLine.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		labels[n] = strings.TrimSpace(m[2])
	}

	r := canonicalRender{PromptBody: promptBody}
	for _, row := range sender.lastKeyboard.InlineKeyboard {
		for _, btn := range row {
			if btn.CallbackData == nil {
				continue
			}
			data := *btn.CallbackData
			const prefix = "a:d:"
			if !strings.HasPrefix(data, prefix) {
				t.Fatalf("Telegram callback_data %q lacks disambig prefix %q", data, prefix)
			}
			rest := strings.TrimPrefix(data, prefix)
			idx := strings.LastIndex(rest, ":")
			if idx <= 0 {
				t.Fatalf("Telegram callback_data %q malformed", data)
			}
			ref := rest[:idx]
			num, err := strconv.Atoi(rest[idx+1:])
			if err != nil {
				t.Fatalf("Telegram callback_data %q number not numeric: %v", data, err)
			}
			label, ok := labels[num]
			if !ok {
				t.Fatalf("Telegram disambig choice %d has button but no body label line in %q",
					num, sender.lastText)
			}
			r.Actions = append(r.Actions, canonicalAction{
				Kind:  "disambiguation_choice",
				Ref:   ref,
				Label: label,
				Index: num,
			})
		}
	}
	return r
}

func projectWhatsAppDisambig(t *testing.T, resp contracts.AssistantResponse) canonicalRender {
	t.Helper()
	out, err := whatsappadapter.Render(resp, 4096)
	if err != nil {
		t.Fatalf("WhatsApp Render: %v", err)
	}
	if out.Interactive == nil {
		t.Fatalf("WhatsApp disambig render produced no interactive message (got kind=%q)", out.Kind)
	}
	r := canonicalRender{PromptBody: out.Interactive.Body}
	switch out.Interactive.Kind {
	case whatsappadapter.OutboundInteractiveButtons:
		for _, b := range out.Interactive.Buttons {
			ref, num, ok := whatsappadapter.DecodeDisambigPayload(b.ID)
			if !ok {
				t.Fatalf("WhatsApp button ID %q is not a disambig payload", b.ID)
			}
			r.Actions = append(r.Actions, canonicalAction{
				Kind:  "disambiguation_choice",
				Ref:   ref,
				Label: b.Title,
				Index: num,
			})
		}
	case whatsappadapter.OutboundInteractiveList:
		for _, section := range out.Interactive.ListSections {
			for _, row := range section.Rows {
				ref, num, ok := whatsappadapter.DecodeDisambigPayload(row.ID)
				if !ok {
					t.Fatalf("WhatsApp list row ID %q is not a disambig payload", row.ID)
				}
				r.Actions = append(r.Actions, canonicalAction{
					Kind:  "disambiguation_choice",
					Ref:   ref,
					Label: row.Title,
					Index: num,
				})
			}
		}
	default:
		t.Fatalf("WhatsApp disambig render produced unexpected kind %q", out.Interactive.Kind)
	}
	return r
}

func mustFindRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate repo root (go.mod) walking up from %s", cwd)
		}
		dir = parent
	}
}

// silence unused linter warnings for shared imports — fmt is used in
// error formatting in sibling parity tests in the same package.
var _ = fmt.Sprintf
