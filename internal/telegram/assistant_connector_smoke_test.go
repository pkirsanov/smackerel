// assistant_connector_smoke_test.go — connector-only Telegram round-trip smoke.
//
// WHY THIS EXISTS (operator request 2026-07-23): "we must have a way to test
// telegram via connector only, so we don't need a telegram client for that."
//
// Spec 104 shipped self-knowledge grounding for /ask and BUG-061-009 shipped the
// honest-refusal invariant, but the final "did the DEPLOYED bot render it right?"
// step was a MANUAL operator action (open Telegram, send /ask, eyeball the reply).
// This test makes that behavior AUTOMATABLE with NO real Telegram client / bot
// token: it injects a synthetic inbound Telegram Update straight into the REAL
// bot dispatch (bot.handleMessage — the exact path the webhook handler drives on
// the deployed bot), routes it through the REAL assistant adapter, and CAPTURES
// the rendered OUTBOUND via a recording Sender. The message asserted on is
// byte-for-byte the message the user would see.
//
// The two behaviors it locks down are the operator smoke test itself:
//
//   - GROUNDED: /ask <meta-question> whose answer cites a self-knowledge
//     (artifact) source renders the answer body + a "sources:" citation block —
//     and NEVER reads as "saved as an idea".
//   - UNGROUNDABLE: /ask <no-ground> renders the honest refusal body
//     ("I don't have a sourced answer for that.") verbatim — NEVER the capture
//     acknowledgement (BUG-061-009 INV-HB-REFUSAL, now verified at the exact
//     connector boundary the user sees, not just inside the facade).
//
// SEPARATION OF CONCERNS (no duplication):
//   - The facade's honesty DECISION (grounded → answer, ungroundable → honest
//     refusal, never "saved as an idea") is exhaustively tested in
//     internal/assistant/facade_execution_error_honesty_test.go.
//   - The agent's REAL self-knowledge grounding over pgvector is tested in
//     tests/e2e/openknowledge/self_knowledge_ask_e2e_test.go.
//   - THIS test closes the one remaining gap those two do NOT cover: the Telegram
//     CONNECTOR round-trip (bot dispatch -> adapter translate -> adapter render ->
//     outbound). The assistant behind the adapter is therefore a controllable
//     double returning the exact contracts.AssistantResponse shapes the real
//     facade emits for these two cases — the connector is the thing under test.
//
// It is a plain unit test (no build tag, no external stack) so it runs in every
// `./smackerel.sh test unit --go` and guards the deployed /ask render on every
// change — the manual operator step is now a standing regression.

package telegram

import (
	"context"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// capturingSender records the rendered outbound text the adapter would send to
// Telegram, so a connector round-trip can assert on the exact message the user
// would receive — with NO real Telegram client / bot token. It is the
// connector-only counterpart to recordingSenderForBot (which discards).
type capturingSender struct {
	mu   sync.Mutex
	sent []string
}

func (c *capturingSender) Send(m tgbotapi.Chattable) (tgbotapi.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if mc, ok := m.(tgbotapi.MessageConfig); ok {
		c.sent = append(c.sent, mc.Text)
	}
	return tgbotapi.Message{MessageID: len(c.sent)}, nil
}

func (c *capturingSender) messages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sent))
	copy(out, c.sent)
	return out
}

func (c *capturingSender) joined() string { return strings.Join(c.messages(), "\n---\n") }

// newConnectorSmokeBot wires the REAL bot dispatch + REAL assistant adapter with
// a capturingSender (outbound) and a controllable stub assistant (returning
// resp). This is the reusable connector-only test harness: inject an inbound
// update via injectAsk and read the captured outbound — no Telegram client
// required. It mirrors newAssistantInterceptBot but swaps the discarding Sender
// for a capturing one so the RENDERED reply can be asserted.
func newConnectorSmokeBot(t *testing.T, resp contracts.AssistantResponse) (*Bot, *capturingSender, *asstStubAssistant) {
	t.Helper()
	capRec := newAsstCaptureRecorder(t) // capture HTTP server for the CaptureFn hook (unused on the /ask answer path)
	sender := &capturingSender{}
	asst := &asstStubAssistant{resp: resp}

	bot := &Bot{
		captureURL:  capRec.server.URL,
		httpClient:  capRec.server.Client(),
		replyFunc:   func(int64, string) {},
		userMapping: map[int64]string{99: "u-99"},
		environment: "test",
	}

	adapter, err := assistant_adapter.NewAdapter(assistant_adapter.Options{
		Sender:          sender,
		Capture:         NewBotCaptureFn(bot),
		ResolveUser:     NewBotChatResolver(bot),
		MarkdownMode:    assistant_adapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("NewAdapter: %v", err)
	}
	if err := adapter.Start(context.Background(), asst); err != nil {
		t.Fatalf("adapter Start: %v", err)
	}
	bot.SetAssistantAdapter(adapter)
	return bot, sender, asst
}

// injectAsk feeds a synthetic inbound "/ask <text>" Telegram update through the
// REAL bot dispatch — the exact path bot.go drives from the webhook handler on
// the deployed bot. No network, no Telegram API, no bot token.
func injectAsk(bot *Bot, chatID int64, text string) {
	msg := &tgbotapi.Message{
		Chat:      &tgbotapi.Chat{ID: chatID},
		Text:      "/ask " + text,
		MessageID: 1,
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: len("/ask")},
		},
	}
	bot.handleMessage(context.Background(), msg, 1)
}

// TestTelegramConnector_AskSelfKnowledge_RendersGroundedCitedAnswer — the
// grounded half of the operator smoke, automated. A /ask meta-question whose
// answer cites a self-knowledge artifact source must render, through the real
// connector, the answer body PLUS a citation block — and must never read as
// "saved as an idea".
func TestTelegramConnector_AskSelfKnowledge_RendersGroundedCitedAnswer(t *testing.T) {
	const body = "Smackerel is a passive second brain that captures, connects, and answers about your own knowledge."
	const sourceTitle = "capabilities overview"
	bot, sender, asst := newConnectorSmokeBot(t, contracts.AssistantResponse{
		Status: contracts.StatusAnswered,
		Body:   body,
		Sources: []contracts.Source{
			{
				Kind:  contracts.SourceArtifact,
				ID:    "smackerel-self-caps-0001",
				Title: sourceTitle,
				Ref:   contracts.ArtifactRef{ArtifactID: "smackerel-self-caps-0001"},
			},
		},
	})

	injectAsk(bot, 99, "what can you do?")

	if len(asst.calls) != 1 {
		t.Fatalf("/ask must reach the assistant exactly once via the connector; got %d calls", len(asst.calls))
	}
	if !strings.Contains(asst.calls[0].Text, "what can you do?") {
		t.Fatalf("connector swallowed the /ask question text; assistant saw %q", asst.calls[0].Text)
	}

	out := sender.joined()
	if out == "" {
		t.Fatal("connector produced NO outbound message for a grounded /ask (a silent drop is itself a defect)")
	}
	if !strings.Contains(out, "second brain") {
		t.Fatalf("grounded answer body missing from the rendered outbound: %q", out)
	}
	if !strings.Contains(out, "sources:") || !strings.Contains(out, sourceTitle) {
		t.Fatalf("grounded /ask must render a citation (a 'sources:' block naming the artifact); got: %q", out)
	}
	if lc := strings.ToLower(out); strings.Contains(lc, "saved as an idea") {
		t.Fatalf("MASKING REGRESSION: a grounded /ask answer must NEVER read as 'saved as an idea'; got: %q", out)
	}
}

// TestTelegramConnector_AskUngroundable_RendersHonestRefusal_NotSavedAsIdea —
// the ungroundable half of the operator smoke, automated. This is the exact
// class the whole /ask arc (BUG-061-006..009 + spec 104) exists to protect: a
// matched, executed /ask that cannot be grounded must render an HONEST refusal
// through the connector, never the band-low capture acknowledgement.
func TestTelegramConnector_AskUngroundable_RendersHonestRefusal_NotSavedAsIdea(t *testing.T) {
	refusal := contracts.CanonicalRefusalBodyFor(contracts.RefusalDefault) // "I don't have a sourced answer for that."
	bot, sender, asst := newConnectorSmokeBot(t, contracts.AssistantResponse{
		Status:     contracts.StatusUnavailable,
		ErrorCause: contracts.ErrNoGroundedAnswer,
		Body:       refusal,
	})

	injectAsk(bot, 99, "what does my private research say about an unindexed topic?")

	if len(asst.calls) != 1 {
		t.Fatalf("/ask must reach the assistant exactly once via the connector; got %d calls", len(asst.calls))
	}

	msgs := sender.messages()
	if len(msgs) != 1 {
		t.Fatalf("ungroundable /ask must render exactly one honest reply; got %d messages: %v", len(msgs), msgs)
	}
	if msgs[0] != refusal {
		t.Fatalf("ungroundable /ask must render the honest refusal verbatim.\n got: %q\nwant: %q", msgs[0], refusal)
	}
	// The core invariant of the entire /ask arc, asserted at the boundary the
	// operator actually sees: a high-band refusal is NEVER masked as a capture.
	if lc := strings.ToLower(msgs[0]); strings.Contains(lc, "saved as an idea") || strings.Contains(lc, "(idea)") {
		t.Fatalf("MASKING REGRESSION (BUG-061-009 INV-HB-REFUSAL): an ungroundable /ask must NOT read as 'saved as an idea'; got: %q", msgs[0])
	}
}
