//go:build integration

// BUG-061-006 — live-core regression for the single, honest capture-as-fallback
// acknowledgement.
//
// WHAT THIS FILE DRIVES (no fake of the system under test):
//
//	real assistant_adapter.HandleUpdate
//	  -> real telegram.NewBotCaptureFn  (the BUG-061-006 fix surface)
//	  -> real Bot.captureIdeaSilent
//	  -> real POST /api/capture on the LIVE core container
//	  -> real capture pipeline + real Postgres
//
// The assertions read live Postgres rows and a real outbound-message sink.
// The only substituted component is the ROUTER/EXECUTOR: the injected
// contracts.Assistant returns a fixed CaptureRoute=true response. That
// substitution is deliberate and is what makes this a regression test rather
// than a sampling of a language model — see the next section.
//
// ── WHY THIS IS `integration` AND NOT `e2e` (four measured blockers) ──
//
// An earlier draft of this file drove the live Telegram webhook end-to-end as
// `e2e-api`. It could not run honestly in ANY lane. Each blocker below was
// measured, not assumed:
//
//	B1  LANE SCOPE. The provider-dependent phase in smackerel.sh runs
//	    `go test -tags e2e_ollama ... ./tests/e2e/agent/...` — a hardcoded
//	    package list. An `e2e_ollama`-tagged file under tests/e2e/assistant/
//	    compiles in NO lane, so "move it to the opt-in phase" produces a dead
//	    test rather than a gated one.
//
//	B2  ENV WIRING. That same phase passes no `--env-file`; it exports 12
//	    explicit vars, none of which are the four Telegram webhook vars
//	    (ASSISTANT_TRANSPORTS_TELEGRAM_MODE / _WEBHOOK_PATH,
//	    ASSISTANT_TELEGRAM_WEBHOOK_SECRET, TELEGRAM_USER_MAPPING). A webhook
//	    test there fails on harness wiring, never reaching the behaviour.
//
//	B3  MODEL PIN. The opt-in pulls exactly OLLAMA_TEST_MODEL
//	    (qwen2.5:0.5b-instruct). The open_knowledge path requires
//	    ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID (gemma3:4b), which no test lane
//	    ever pulls. Enabling the opt-in therefore does NOT clear the provider
//	    error that blocked the webhook draft.
//
//	B4  PATH REACHABILITY. AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge
//	    makes the facade promote BandLow -> BandHigh (facade.go), so the
//	    CauseUnrouted capture path is unreachable for prose in this
//	    environment. The only reachable capture is the open_knowledge
//	    no-ground hook, which fires only when the MODEL emits an envelope
//	    parsing to {"status":"refused"}. That is a sampled property of a
//	    language model, not a deterministic regression signal.
//
// B3+B4 are not harness bugs — they are why the three scenarios cannot be
// driven end-to-end deterministically at all. Controlling the assistant here
// removes the one leg BUG-061-006 never touched (routing) and keeps every leg
// it did touch (capture hook, capture API, persistence, acknowledgement).
//
// ── WHAT THIS FILE PROVES THAT THE WEBHOOK DRAFT COULD NOT ──
//
// The webhook draft documented that the ack COUNT and TEXT were unobservable
// on the disposable stack (TELEGRAM_BOT_TOKEN="" makes both reply sinks
// no-ops) and therefore asserted only DB rows. Driving the adapter directly
// restores a real outbound sink, so each scenario below binds the STORE and
// the ACKNOWLEDGEMENT in the same assertion — which is precisely what
// BUG-061-006 is about: the two must never disagree.
//
// ── WHAT EACH TEST IS RED AGAINST ──
//
//	SCN-061-006-01  one capture turn -> exactly ONE row AND exactly ONE ack
//	                carrying the saved-as-idea body.
//	                RED at 0 rows — the fix replaced the replying
//	                handleTextCapture hook with the silent captureIdeaSilent;
//	                over-silencing it into a no-op is the most likely
//	                regression of this exact change (BS-001 durability).
//	                RED at 2 rows — any "de-duplicate the ack" edit that
//	                leaves two capture invocations on the fallback path.
//	                RED at 2 acks — the pre-fix duplicate-sink shape.
//
//	SCN-061-006-02  a bare "/ask" persists NOTHING and never claims it did.
//	                StripShortcutPrefix("/ask") == "" so captureIdeaSilent
//	                short-circuits with ErrNothingToCapture. RED against the
//	                tempting wrong fix — making the "saved as an idea" ack
//	                TRUE by persisting an empty/placeholder idea — and RED if
//	                the honest-ack override is reverted. Paired with a live
//	                control turn so the zero cannot be produced by a dead
//	                pipeline.
//
//	SCN-061-006-03  a re-sent (duplicate) capture still persists exactly ONE
//	                and still acknowledges honestly.
//	                The fix added the errDuplicate -> nil branch so an
//	                already-saved idea stays a benign single acknowledgement.
//	                RED at 2 rows if that branch regresses into a second
//	                insert; RED at 0 if it swallows the original persist; RED
//	                on the ack if a duplicate starts rendering the failure
//	                line.

package assistant_integration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/telegram"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

const (
	// bug061006SavedAsIdeaBody is the canonical capture acknowledgement the
	// facade emits and the adapter renders unchanged on a successful persist.
	bug061006SavedAsIdeaBody = "saved as an idea — i'll surface it later."

	// bug061006SettleWindow is how long an exact-count assertion keeps
	// watching after the expected row appears. Without it, "no second row"
	// would be a race the buggy code could win.
	bug061006SettleWindow = 5 * time.Second

	// bug061006BareShortcut is a shortcut with no body. StripShortcutPrefix
	// returns "" for it, which is the SCN-061-006-02 trigger.
	bug061006BareShortcut = "/ask"
)

type bug061006Stack struct {
	Adapter *assistant_adapter.Adapter
	Sender  *bug061006Sender
	ChatID  int64
	Pool    *pgxpool.Pool
}

// bug061006Sender records every outbound message the adapter renders. This is
// the sink the disposable e2e stack could not provide, and it is what lets the
// ack COUNT and TEXT be asserted alongside the persisted row.
type bug061006Sender struct {
	mu   sync.Mutex
	sent []string
}

func (s *bug061006Sender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	text := ""
	if mc, ok := c.(tgbotapi.MessageConfig); ok {
		text = mc.Text
	}
	s.sent = append(s.sent, text)
	return tgbotapi.Message{MessageID: len(s.sent)}, nil
}

func (s *bug061006Sender) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.sent))
	copy(out, s.sent)
	return out
}

// bug061006CaptureRouteAssistant is the controlled routing seam. It returns the
// canonical capture-as-fallback response for every turn, which is exactly the
// facade state BUG-061-006 governs. Substituting it removes the language model
// (see B3/B4 above) without touching any code the fix changed.
type bug061006CaptureRouteAssistant struct{}

func (bug061006CaptureRouteAssistant) Handle(
	_ context.Context, _ contracts.AssistantMessage,
) (contracts.AssistantResponse, error) {
	return contracts.AssistantResponse{
		Status:       contracts.StatusSavedAsIdea,
		CaptureRoute: true,
		Body:         bug061006SavedAsIdeaBody,
		EmittedAt:    time.Now().UTC(),
	}, nil
}

// loadBUG061006Stack wires the real capture hook against the live core.
//
// CORE_EXTERNAL_URL absent is the package-wide "no live stack here" gate (same
// contract as http_adapter_bind_test.go, and the integration-light lane
// deliberately exports it empty). Every OTHER missing value is a wiring bug and
// fails loud rather than skipping — a skip there would hide the assertion.
func loadBUG061006Stack(t *testing.T) bug061006Stack {
	t.Helper()

	coreURL := strings.TrimRight(os.Getenv("CORE_EXTERNAL_URL"), "/")
	if coreURL == "" {
		t.Skip("integration: CORE_EXTERNAL_URL not set — live core not available")
	}

	authToken := os.Getenv("SMACKEREL_AUTH_TOKEN")
	if authToken == "" {
		t.Fatal("SMACKEREL_AUTH_TOKEN not set; the live core is up but the capture POST has no bearer — wiring bug, not a legitimate skip")
	}

	chatID, mapping := bug061006MappedChat(t)

	bot, err := telegram.NewBotForWebhookTestMode(telegram.Config{
		CoreAPIURL:  coreURL,
		AuthToken:   authToken,
		Environment: "test",
		UserMapping: mapping,
	})
	if err != nil {
		t.Fatalf("build capture-capable bot against live core %s: %v", coreURL, err)
	}

	sender := &bug061006Sender{}
	adapter, err := assistant_adapter.NewAdapter(assistant_adapter.Options{
		Sender:          sender,
		Capture:         telegram.NewBotCaptureFn(bot),
		ResolveUser:     func(int64) (string, error) { return mapping[chatID], nil },
		MarkdownMode:    assistant_adapter.PlainText,
		MaxMessageChars: 4096,
	})
	if err != nil {
		t.Fatalf("build assistant adapter: %v", err)
	}
	if err := adapter.Start(context.Background(), bug061006CaptureRouteAssistant{}); err != nil {
		t.Fatalf("start assistant adapter: %v", err)
	}

	return bug061006Stack{
		Adapter: adapter,
		Sender:  sender,
		ChatID:  chatID,
		Pool:    openScope2Pool(t),
	}
}

// bug061006MappedChat returns the first chat_id declared in the SST
// TELEGRAM_USER_MAPPING plus the parsed mapping. The adapter's UserResolver
// refuses an unmapped chat, so an unmapped id would drop the turn before the
// capture path runs.
func bug061006MappedChat(t *testing.T) (int64, map[int64]string) {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv("TELEGRAM_USER_MAPPING"))
	if raw == "" {
		t.Fatal("TELEGRAM_USER_MAPPING not set; the assistant adapter refuses unmapped chats so no capture-fallback turn can be driven — wiring bug")
	}
	mapping := map[int64]string{}
	var first int64
	for i, entry := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(entry), ":", 2)
		if len(parts) != 2 {
			t.Fatalf("TELEGRAM_USER_MAPPING entry %q is not chat_id:user_id", entry)
		}
		id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
		if err != nil {
			t.Fatalf("parse chat_id from TELEGRAM_USER_MAPPING entry %q: %v", entry, err)
		}
		mapping[id] = strings.TrimSpace(parts[1])
		if i == 0 {
			first = id
		}
	}
	if first == 0 {
		t.Fatalf("TELEGRAM_USER_MAPPING first entry yielded chat_id 0 (raw=%q)", raw)
	}
	return first, mapping
}

// bug061006Probe builds a unique, unstructured probe body. The nonce scopes
// every row assertion to this one turn.
func bug061006Probe(marker string) string {
	return fmt.Sprintf("bug061006-%s-%d stray thought worth keeping", marker, time.Now().UnixNano())
}

// bug061006Turn delivers one synthetic Telegram text update through the real
// adapter. HandleUpdate is synchronous, so on return the capture path has
// already run to completion.
func bug061006Turn(t *testing.T, stack bug061006Stack, text string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	id := int(time.Now().UnixNano() % 2147483647)
	update := &tgbotapi.Update{
		UpdateID: id,
		Message: &tgbotapi.Message{
			MessageID: id,
			Date:      int(time.Now().Unix()),
			Chat:      &tgbotapi.Chat{ID: stack.ChatID, Type: "private"},
			From:      &tgbotapi.User{ID: stack.ChatID, FirstName: "BUG061006"},
			Text:      text,
		},
	}
	handled, err := stack.Adapter.HandleUpdate(ctx, update)
	if err != nil {
		t.Fatalf("HandleUpdate(%q): %v", text, err)
	}
	if !handled {
		t.Fatalf("HandleUpdate(%q) reported handled=false; the turn never reached the capture path", text)
	}
}

func bug061006DBNow(t *testing.T, pool *pgxpool.Pool) time.Time {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var now time.Time
	if err := pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		t.Fatalf("read database clock: %v", err)
	}
	return now
}

func bug061006CountArtifacts(t *testing.T, pool *pgxpool.Pool, content string) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM artifacts WHERE content_raw = $1`, content,
	).Scan(&count); err != nil {
		t.Fatalf("count artifacts for probe: %v", err)
	}
	return count
}

// bug061006CountBlankArtifactsSince counts rows whose captured text is empty or
// whitespace-only. Pre-conditions on created_at so a neighbouring test's rows
// can never satisfy or break the assertion.
func bug061006CountBlankArtifactsSince(t *testing.T, pool *pgxpool.Pool, since time.Time) int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM artifacts
		  WHERE created_at >= $1
		    AND coalesce(btrim(content_raw), '') = ''`, since,
	).Scan(&count); err != nil {
		t.Fatalf("count blank-text artifacts: %v", err)
	}
	return count
}

// bug061006WaitForArtifact polls until at least one row carries the probe text,
// then fails if none arrived within the budget.
func bug061006WaitForArtifact(t *testing.T, pool *pgxpool.Pool, content string, budget time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if bug061006CountArtifacts(t, pool, content) > 0 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("no artifact with the probe text landed within %s; the capture-as-fallback path did not persist the idea (BS-001 durability regression, or the silent capture hook was over-silenced into a no-op)", budget)
}

// TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea — SCN-061-006-01.
//
// Drives one capture-as-fallback turn through the real capture hook and the
// live /api/capture, and asserts the store holds EXACTLY ONE idea while the
// user received EXACTLY ONE acknowledgement carrying the saved-as-idea body.
//
// Adversarial: 0 rows means the silent hook stopped persisting — the
// over-silencing regression this change is most exposed to. 2 rows means two
// capture invocations remain on the fallback path, which is precisely the
// duplicate-sink shape BUG-061-006 removed. 2 acks is the original user-visible
// defect. Binding both in one test is what proves the store and the
// acknowledgement agree.
func TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea(t *testing.T) {
	stack := loadBUG061006Stack(t)

	probe := bug061006Probe("single")
	bug061006Turn(t, stack, probe)

	bug061006WaitForArtifact(t, stack.Pool, probe, 30*time.Second)

	// Hold the window open so a delayed second write cannot slip past the count.
	time.Sleep(bug061006SettleWindow)

	if got := bug061006CountArtifacts(t, stack.Pool, probe); got != 1 {
		t.Fatalf("ADVERSARIAL: capture-as-fallback persisted %d ideas for one turn; want exactly 1 (0 = the silent hook stopped persisting; 2 = two capture invocations remain on the fallback path)", got)
	}

	sent := stack.Sender.snapshot()
	if len(sent) != 1 {
		t.Fatalf("ADVERSARIAL: one capture turn produced %d outbound messages %q; want exactly 1 — the duplicate legacy-hook reply is the original BUG-061-006 defect", len(sent), sent)
	}
	if sent[0] != bug061006SavedAsIdeaBody {
		t.Fatalf("ack text = %q; want %q — a persisted idea must be acknowledged with the canonical saved-as-idea body", sent[0], bug061006SavedAsIdeaBody)
	}
}

// TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing — SCN-061-006-02.
//
// A bare "/ask" strips to an empty body, so captureIdeaSilent MUST report
// nothing-to-capture and MUST NOT reach /api/capture. Asserts no blank-text
// idea was written and the single acknowledgement never claims "saved", then
// proves in the same run and the same chat that the persist path is live — so
// the zero can never be produced by "nothing ran".
//
// Adversarial: the obvious wrong way to stop the acknowledgement lying is to
// make it true, by persisting an empty or placeholder idea for the bare
// shortcut. That fix lands a blank-text row and fails the first assertion. The
// control fails if the capture path is dead, so the pair cannot both be
// satisfied vacuously.
func TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing(t *testing.T) {
	stack := loadBUG061006Stack(t)

	since := bug061006DBNow(t, stack.Pool)

	bug061006Turn(t, stack, bug061006BareShortcut)

	// Give any (unwanted) write the same latency budget a real capture gets, so
	// the negative is a decision rather than a race we happened to win.
	time.Sleep(bug061006SettleWindow)

	if got := bug061006CountBlankArtifactsSince(t, stack.Pool, since); got != 0 {
		t.Fatalf("ADVERSARIAL: bare %q persisted %d blank-text idea(s); want 0 — nothing was said, so nothing may be saved and nothing may be acknowledged as saved", bug061006BareShortcut, got)
	}

	sent := stack.Sender.snapshot()
	if len(sent) != 1 {
		t.Fatalf("bare %q produced %d outbound messages %q; want exactly 1 honest line", bug061006BareShortcut, len(sent), sent)
	}
	if strings.Contains(strings.ToLower(sent[0]), "saved as an idea") {
		t.Fatalf("ADVERSARIAL: bare %q was acknowledged with %q; nothing was persisted, so the ack MUST NOT claim it was saved", bug061006BareShortcut, sent[0])
	}

	// Positive control — same chat, same run: a turn that DOES carry text still
	// persists. Proves the preceding zero is a refusal, not a dead pipeline.
	probe := bug061006Probe("control")
	bug061006Turn(t, stack, probe)
	bug061006WaitForArtifact(t, stack.Pool, probe, 30*time.Second)
	if got := bug061006CountArtifacts(t, stack.Pool, probe); got != 1 {
		t.Fatalf("control turn persisted %d ideas; want exactly 1 — the blank-text zero above is only meaningful while this control holds", got)
	}
}

// TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne — SCN-061-006-03.
//
// The fix gave captureIdeaSilent an errDuplicate -> nil branch: an idea that
// already exists is a benign success, so the single acknowledgement stays
// honest instead of becoming a failure line. Re-sends the same probe and
// asserts the store still holds exactly one row and neither turn was
// acknowledged as a failure.
//
// Adversarial: 2 rows means the duplicate branch regressed into a second
// insert, which would make the store disagree with the single acknowledgement
// the user sees. 0 rows means the duplicate branch started swallowing the
// original persist. A failure-line ack on the second turn means errDuplicate
// stopped being treated as benign. Only the fixed behaviour satisfies all three.
func TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne(t *testing.T) {
	stack := loadBUG061006Stack(t)

	probe := bug061006Probe("duplicate")

	bug061006Turn(t, stack, probe)
	bug061006WaitForArtifact(t, stack.Pool, probe, 30*time.Second)

	bug061006Turn(t, stack, probe)

	time.Sleep(bug061006SettleWindow)

	if got := bug061006CountArtifacts(t, stack.Pool, probe); got != 1 {
		t.Fatalf("ADVERSARIAL: two identical capture-fallback turns left %d rows; want exactly 1 (2 = the errDuplicate benign-success branch regressed into a second insert; 0 = it swallowed the original persist)", got)
	}

	sent := stack.Sender.snapshot()
	if len(sent) != 2 {
		t.Fatalf("two turns produced %d outbound messages %q; want exactly 2 (one honest ack per turn)", len(sent), sent)
	}
	for i, body := range sent {
		if body != bug061006SavedAsIdeaBody {
			t.Fatalf("ADVERSARIAL: turn %d was acknowledged with %q; want %q — a duplicate is a benign success and MUST NOT render the failure line", i+1, body, bug061006SavedAsIdeaBody)
		}
	}
}
