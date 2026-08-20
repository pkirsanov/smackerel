# BUG-061-006 — Execution report

> Evidence standard: raw terminal output, ≥10 lines per claim, captured in this
> session. Home-directory paths redacted to `~` per repo PII policy.

## Summary

The Telegram assistant capture-as-fallback path emitted two acknowledgements per
turn (legacy capture-hook reply + assistant renderer body), and for a bare `/ask`
the two contradicted each other (`? Failed to save` + `saved as an idea`). The fix
makes the bot-side capture hook silent and honest: it persists the idea without a
reply of its own, and reports whether an idea was actually saved so the renderer's
single acknowledgement is truthful.

## Completion Statement

Code-complete, unit-verified, and DEPLOYED. Two scopes implemented in one change;
four adversarial regression tests GREEN; both changed Go packages compile and
pass. The fix (sourceSha `777323fa`) was built + operator-cosign-signed + deployed
to the running `<deploy-host>`; the running containers were verified
this session to carry the exact fix image digests, be healthy, and have the
Telegram assistant adapter wired and bound (see "Deploy + Live Verification").
The only remaining item is the operator's behavioral Telegram smoke test — a human
turn the agent cannot perform.

## Root cause (code-path trace) {#repro-red}

The live stack was down for this session, so DEFECT reproduction is a
source-level code-path trace (the established Smackerel approach for
transport-reply bugs) plus adversarial tests that encode the pre-fix behavior as
FAILING assertions.

Pre-fix path (both reply sinks fire on one turn):

```text
adapter.go::HandleUpdate
  resp.CaptureRoute == true
  -> a.capture(ctx, msg, StripShortcutPrefix(msg.Text))     # sink A
       NewBotCaptureFn -> Bot.handleTextCapture
         callCapture(...)                                    # persist
         replyWithMapping(". Saved: \"…\" (idea)")           # <-- reply A (duplicate)
         (on error) captureErrorReply("? Failed to save …")  # <-- reply A (contradiction)
  -> RenderToChat(resp)                                      # sink B
       renderOutbound -> sends resp.Body
         Body == "saved as an idea — i'll surface it later." # <-- reply B
```

For a bare `/ask`: `StripShortcutPrefix("/ask") == ""` → `callCapture` POSTs empty
text → fails → reply A = `? Failed to save`; reply B = `saved as an idea` → a false,
contradictory pair. The three adversarial tests below assert the POST-fix
behavior (single message; honest text; NEVER "saved as an idea" on empty/failed
capture) and therefore FAIL if the fix is reverted.

## After Fix — unit evidence {#after-fix-unit-evidence}

Command (run through the repo CLI in the isolated Go container):

```text
$ ~/smackerel/smackerel.sh test unit --go --go-run 'BUG061006|HandleUpdate|HandleMessage_Assistant|HandleMessage_BUG|TranslateInbound|CaptureStrips|NonShortcut'
[go-unit] applying -run selector: BUG061006|HandleUpdate|HandleMessage_Assistant|HandleMessage_BUG|TranslateInbound|CaptureStrips|NonShortcut
[go-unit] starting go test ./...
ok      github.com/smackerel/smackerel/cmd/core 0.284s [no tests to run]
ok      github.com/smackerel/smackerel/internal/agent/tools/weather     0.049s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant       0.270s [no tests to run]
ok      github.com/smackerel/smackerel/internal/assistant/provenance    0.039s [no tests to run]
ok      github.com/smackerel/smackerel/internal/telegram        0.126s
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.067s
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter     0.026s [no tests to run]
[go-unit] go test ./... finished OK
```

The two changed packages ran their tests and passed (no `[no tests to run]`
suffix): `internal/telegram` (`ok 0.126s`) and
`internal/telegram/assistant_adapter` (`ok 0.067s`). The filtered `go test ./...`
compiled the whole module (compile + vet) with zero FAILs.

Tests exercised (all GREEN):

- `TestHandleUpdate_BUG061006_CaptureSuccess_SingleAck` — persisted capture →
  renderer sends exactly one message = the "saved as an idea" body (single ack).
- `TestHandleUpdate_BUG061006_NothingToCapture_HonestAck` — bare `/ask` (empty) →
  single honest prompt; asserts the reply does NOT contain "saved as an idea".
- `TestHandleUpdate_BUG061006_CaptureFailure_HonestAck` — real capture error →
  single honest failure line; asserts NOT "saved as an idea".
- `TestHandleMessage_BUG061006_CaptureRoute_SingleSilentAck` (bot-level,
  adversarial) — the legacy reply sink receives 0 messages (silent hook); the
  renderer sends exactly 1; the idea is still persisted to the capture API.

Existing capture-path tests in the same packages remained GREEN under the new
`CaptureFn` signature (`TestHandleUpdate_CaptureRouteInvokesBotHook`,
`TestHandleUpdate_PlainTextRendersAndDoesNotCapture`,
`TestHandleMessage_AssistantCaptureRoute_FallsThroughToCapture`,
`TestHandleUpdate_BUG064001_CaptureStripsAskPrefix`, the TranslateInbound suite),
confirming BS-001 durability and the BUG-064-001 prefix-strip contract are
preserved.

### Code Diff Evidence

The fix landed as commit `5285e77f`; the deploy/live-verification evidence landed
as `941731a3`. Recovered from git in this session:

```text
$ git log --oneline --no-decorate -- specs/061-conversational-assistant/bugs/BUG-061-006-duplicate-contradictory-capture-ack
2b50f6a5 releases: land the release-schema repair and make both release gates green
c4a46e73 chore(genericize): remove machine-local and deployment-specific values
941731a3 docs(061): BUG-061-006 — record <deploy-target> deploy + live infra-verification evidence
5285e77f fix(061): BUG-061-006 — single honest Telegram assistant capture acknowledgement
```

The subject for `941731a3` is reproduced above with its concrete
deployment-target selector redacted to `<deploy-target>`, because the
product-deployment-boundary policy forbids a concrete target selector anywhere in
a product repo. `git show -s 941731a3` displays the unmodified original.

Changed paths in the implementation commit (`git show --name-only 5285e77f`) —
six runtime/source files plus this packet's own artifacts:

```text
internal/telegram/assistant_adapter/adapter.go
internal/telegram/assistant_adapter/adapter_test.go
internal/telegram/assistant_adapter/capture_ack_bug061006_test.go
internal/telegram/assistant_wiring.go
internal/telegram/bot.go
internal/telegram/capture_ack_bug061006_test.go
```

Line counts (`git show --stat 5285e77f`, source files only):

```text
 internal/telegram/assistant_adapter/adapter.go            |  57 ++++++-
 internal/telegram/assistant_adapter/adapter_test.go       |   4 +-
 internal/telegram/assistant_adapter/capture_ack_bug061006_test.go | 178 +++++++++
 internal/telegram/assistant_wiring.go                     |  24 ++-
 internal/telegram/bot.go                                  |  52 +++++-
 internal/telegram/capture_ack_bug061006_test.go           | 137 ++++++++++
```

The three behavioral hunks (`git show 5285e77f -- <path>`):

`internal/telegram/assistant_adapter/adapter.go` — the hook reports persistence,
and a non-nil report replaces the "saved as an idea" body with an honest line:

```text
-type CaptureFn func(ctx context.Context, msg *tgbotapi.Message, text string)
+type CaptureFn func(ctx context.Context, msg *tgbotapi.Message, text string) error
+
+// ErrNothingToCapture is the sentinel a CaptureFn returns when the
+// capture-as-fallback text was empty/whitespace so there was nothing to
+// persist (BUG-061-006). Distinct from a real capture failure.
+var ErrNothingToCapture = errors.New("assistant_adapter: nothing to capture")

@@ func (a *Adapter) HandleUpdate(...)
        if resp.CaptureRoute && update.Message != nil {
-               a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text))
+               if capErr := a.capture(ctx, update.Message, assistant.StripShortcutPrefix(msg.Text)); capErr != nil {
+                       resp = honestCaptureFallbackFailure(resp, capErr)
+               }
        }

+func honestCaptureFallbackFailure(resp contracts.AssistantResponse, capErr error) contracts.AssistantResponse {
+       body := "Couldn't save that just now — please try again."
+       if errors.Is(capErr, ErrNothingToCapture) {
+               body = "Nothing to save — add some text or a question after the command."
+       }
+       return contracts.AssistantResponse{
+               Status:    contracts.StatusAnswered,
+               Body:      body,
+               Routing:   resp.Routing,
+               EmittedAt: resp.EmittedAt,
+       }
+}
```

`internal/telegram/assistant_wiring.go` — the assistant hook stops routing
through the replying legacy handler:

```text
 func NewBotCaptureFn(b *Bot) assistant_adapter.CaptureFn {
-       return func(ctx context.Context, msg *tgbotapi.Message, text string) {
+       return func(ctx context.Context, msg *tgbotapi.Message, text string) error {
                if b == nil || msg == nil {
-                       return
+                       return assistant_adapter.ErrNothingToCapture
                }
-               b.handleTextCapture(ctx, msg, text)
+               return b.captureIdeaSilent(ctx, msg, text)
        }
 }
```

`internal/telegram/bot.go` — the persist step is split out of the replying path
so the assistant hook can write the artifact without a reply of its own:

```text
 func (b *Bot) handleTextCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
-       if len(text) > maxShareTextLen {
-               text = stringutil.TruncateUTF8(text, maxShareTextLen)
-       }
-       result, err := b.callCapture(ctx, msg.Chat.ID, map[string]string{"text": text})
+       result, err := b.persistTextIdea(ctx, msg.Chat.ID, text)

+func (b *Bot) persistTextIdea(ctx context.Context, chatID int64, text string) (map[string]interface{}, error) {
+       if len(text) > maxShareTextLen {
+               text = stringutil.TruncateUTF8(text, maxShareTextLen)
+       }
+       return b.callCapture(ctx, chatID, map[string]string{"text": text})
+}
+
+func (b *Bot) captureIdeaSilent(ctx context.Context, msg *tgbotapi.Message, text string) error {
+       if strings.TrimSpace(text) == "" {
+               return assistant_adapter.ErrNothingToCapture
+       }
+       if _, err := b.persistTextIdea(ctx, msg.Chat.ID, text); err != nil {
+               if errors.Is(err, errDuplicate) {
+                       return nil
+               }
+               slog.Error("assistant capture-fallback persist failed", "chat_id", msg.Chat.ID, "error", err)
+               return err
+       }
+       return nil
+}
```

The legacy `handleTextCapture` keeps its `. Saved: "…" (idea)` reply, so the
BS-001 plain-text capture UX is unchanged; only the assistant `CaptureRoute`
hook went silent.

## Test Evidence

See "After Fix — unit evidence" above. Command exit status: the CLI printed
`[go-unit] go test ./... finished OK` and returned success.

## Regression coverage: planned as `e2e-api`, delivered as `integration` {#integration-reclassified-evidence}

This section records a CHANGE OF PLAN, not the original plan. The Test Plan and
DoD for both scopes originally required two `e2e-api` tests in a
`tests/e2e/assistant/` file dedicated to this bug. A draft of that file was
written in the working tree, could not be made to run honestly, and was removed
from the working tree. It was **never committed**: `git log --all` returns zero
commits for either draft path, and the packet's implementation commit records
zero deletions, so neither the file nor its two test symbols appear anywhere in
git history. "Deleted" would overstate it — nothing was ever in the repository
to delete. The regression coverage now lives at
`tests/integration/assistant/capture_ack_bug061006_integration_test.go` as three
`integration` tests.

Those two draft symbol names and that draft file path are deliberately not
repeated anywhere in this packet **as though the test existed**: naming a test
that does not exist is how a reader (or a grep-based audit) comes to believe
coverage exists when it does not. The path does appear exactly once more, in
`state.json` → `certification.pendingGates`, and that occurrence is deliberate
and is KEPT. It is not a coverage claim; it is the open-gap pointer that names
the missing test and its owner, so the next owner reads which coverage is
absent rather than having to rediscover it. A pending gate that names nothing is
not auditable.

A category was changed, so say plainly what that costs. `integration` is a
WEAKER claim than `e2e-api`: it substitutes the router/executor and therefore
does not exercise the real Telegram webhook ingress or the real transport send.
The reclassification restores *assertable* regression coverage; it does not
restore *end-to-end* coverage, and no DoD item in this packet is checked on the
strength of it as though it did. The residual gap is filed below as an open
FINDING.

### Why the e2e variant could not run

The capture-as-fallback path is only reachable in the e2e lane through an
`open_knowledge` turn. The default e2e lane has no usable LLM, so that turn
returns `error_cause="provider_unavailable"` with `capture_route=false`, and the
capture hook never fires at all.

**The product is correct there, and that is the point.** BUG-061-008 established
that an execution error MUST NEVER render as "saved as an idea". A lane whose
provider is unavailable is therefore *supposed* to refuse rather than capture.
The blocker is not a defect to fix — it is the previously-fixed behavior working.
Any e2e test that forced a capture out of that lane would be asserting against
the very contract BUG-061-008 ratified.

The test file's header records four separately measured blockers (lane package
scope, env wiring, model pin, and path reachability under
`AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge`). The last two are not
harness bugs: they are why these three scenarios cannot be driven end-to-end
*deterministically at all*, because the only reachable capture would depend on a
language model sampling a `{"status":"refused"}` envelope.

Integration is the level at which the provider is controllable. The reclassified
tests substitute exactly ONE component — the router/executor — and keep every leg
the fix actually touched: real `HandleUpdate` → real `NewBotCaptureFn` → real
`Bot.captureIdeaSilent` → live `POST /api/capture` → live Postgres, with a real
outbound-message sink. That sink is what the e2e draft could not provide
(`TELEGRAM_BOT_TOKEN=""` makes both reply sinks no-ops), so the integration
tests can bind the persisted row and the acknowledgement in a single assertion —
which is precisely what BUG-061-006 is about.

### FINDING — e2e coverage gap (open, NOT closed by this change) {#finding-e2e-gap}

No `e2e-api` test exercises this path. This is recorded as an OPEN finding
against this packet rather than presented as satisfied, because two DoD items in
each scope ask for E2E-level regression coverage and this packet does not
deliver it. Those four items are therefore left UNCHECKED.

- **Not covered:** the real Telegram webhook ingress and the real transport
  send, end to end, for a capture-as-fallback turn.
- **Measured cause:** the capture-as-fallback path is reachable in the e2e lane
  only through an `open_knowledge` turn. The default e2e lane has no usable LLM,
  so that turn returns `error_cause="provider_unavailable"` with
  `capture_route=false` and the capture hook never fires. That was measured in a
  diagnostic run, not inferred. The product is CORRECT there: BUG-061-008
  ratified that an execution error must never render as "saved as an idea", so a
  provider-less lane is supposed to refuse rather than capture. The blocker is
  the previously-fixed behavior working, not a defect.
- **Why no lane compiles such a file today** (each link read from
  `smackerel.sh` in this session): the opt-in Ollama e2e phase is gated on
  `SMACKEREL_TEST_OLLAMA=1`, builds with `-tags e2e_ollama`, pulls only
  `OLLAMA_TEST_MODEL` (`qwen2.5:0.5b-instruct`, from `config/generated/test.env`),
  and runs the hardcoded package list `./tests/e2e/agent/...` — the phase that
  serves `tests/e2e/agent/happy_path_test.go`. `ASSISTANT_OPEN_KNOWLEDGE_LLM_MODEL_ID`
  (`gemma3:4b`) is pulled by no e2e lane at all. So an `e2e_ollama`-tagged file
  under `tests/e2e/assistant/` would be compiled by nothing.
- **NOT ATTEMPTED HERE.** The `SMACKEREL_TEST_OLLAMA=1` / `e2e_ollama` phase was
  READ, not RUN, in this session. Nothing in this packet claims that route was
  tried, that it works, or that it would work. It is named only because it is
  the nearest existing provider-enabled surface and therefore the most plausible
  starting point for whoever closes this.
- **Condition that closes it:** an e2e phase that (a) provisions a model capable
  of driving an `open_knowledge` turn to a capture route, and (b) compiles an
  assistant-package e2e file, together with the Telegram webhook env wiring an
  ingress-level assertion needs. That is a harness change to `smackerel.sh` and
  the e2e phase definition — outside this bug's fix surface, which is three Go
  files under `internal/telegram/`.
- **Disposition:** routed — recorded here as an open finding on BUG-061-006 and
  carried in `state.json` `certification.pendingGates`, which is why this packet
  stays `in_progress` and is not eligible for a terminal status.

### Evidence — first-party `integration` regression run, this session {#integration-green-first-party}

Run through the repo CLI under `evidence-capture.sh --diagnostic`. This is a
first-party execution in the current session, not a transcript supplied to the
agent:

```text
# BUG-061-006 reclassified integration regression lane (first-party run, this session)
$ ./smackerel.sh test integration --go-run TestAssistantIntegration_BUG061006
exit: 0
lines: 652
sha256: 852e7773b2180fd2dfae6e3c4c2bc09b992e4ff984ad5403f1c28c232f627afb
escalation: diagnostic (bounded retention waived for this invocation)
--- selector + per-test results, verbatim from that run's transcript ---
go-integration: applying -run selector: TestAssistantIntegration_BUG061006
=== RUN   TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea
--- PASS: TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea (5.03s)
=== RUN   TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing
--- PASS: TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing (5.04s)
=== RUN   TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne
--- PASS: TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne (5.04s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/assistant      15.297s
PASS: go-integration
```

**This supersedes the "honest limitation" recorded earlier in this packet.** The
prior session could not sight the three `--- PASS:` lines, because the lane emits
~652 lines and the retention window preserved only the head and tail. Under
`--diagnostic` the tool emits the full transcript (its ceiling is 2000 lines,
well above 652), and the three result lines were read directly from that run's
own transcript, pinned to it by the `sha256` above. The conclusion no longer
rests on the inferential chain below; the chain is retained because it remains
true and it is what makes the exit code interpretable.

Two further first-party observations from the same transcript:

- **Zero `SKIP` lines anywhere in the run.** A grep for `SKIP` across that
  transcript returns nothing, which closes the one skip path
  (`CORE_EXTERNAL_URL not set`) empirically rather than by argument.
- **Zero `FAIL` lines anywhere in the run**, and the lane printed
  `PASS: go-integration`.

Scope binding of the three tests:

| Test | Scenario | What it binds |
|------|----------|---------------|
| `TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea` | SCN-061-006-01 | exactly ONE persisted idea AND exactly ONE acknowledgement carrying the saved-as-idea body |
| `TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne` | SCN-061-006-01 | a re-sent probe body still yields exactly one persisted row |
| `TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing` | SCN-061-006-02 | a bare `/ask` persists NO blank artifact and the single ack does NOT say "saved as an idea" |

What this evidence does NOT establish, stated so it is not read for more than it
is worth: it is `integration`, not `e2e-api`. It does not exercise the real
Telegram webhook ingress or the real transport send. See
[FINDING — e2e coverage gap](#finding-e2e-gap).

### Evidence — reclassified `integration` lane, executed this session

Command under the repo CLI, captured through `evidence-capture.sh`:

```text
# BUG-061-006 integration reclassified lane
$ ./smackerel.sh test integration --go-run TestAssistantIntegration_BUG061006
exit: 0
lines: 655
sha256: 297b03e35bb191663031cca0a14337cfa6d6cbb109bf82d37cf2f644582e05fc
--- first 20 ---
oom-preflight: OK — 26010 MB available (need 6000 MB; swap used 1079 MB).
disk-preflight: OK — C: 60 GB free (need 40 GB), WSL / 488 GB free (need 25 GB).
config-validate: ~/smackerel/config/generated/test.env.tmp OK
Smackerel pre-flight resource check: OK
  RAM  available: 25979 MB (required >= 6000 MB)
  Disk available: 500386 MB / 488.7 GB (required >= 15 GB)
Preparing disposable test stack...
Building disposable test stack images before up (freshness convention)...
--- omitted 615 line(s); sha256 above covers the full output ---
```

Three further runs of the same command this session, all `exit: 0`, with no
failure-shaped lines lifted into any block:

```text
sha256: 8353c1462ffe6984c6a2dc462aeca358030b3fde1e4f56867ce381f31f009401   lines: 658
sha256: 78167522d7fc51b47f9b562b609e8f608c3fcdcf4f27ae04285148ee753d3524   lines: 653
(plus one un-wrapped run of the same command, exit 0)
```

### Why `exit: 0` was NOT accepted at face value {#negative-control}

A lane that exits 0 does not prove a selected test ran. That was tested rather
than assumed, with a selector that cannot match anything:

```text
# NEGATIVE CONTROL - non-matching selector
$ ./smackerel.sh test integration --go-run TestAssistantIntegration_BUG061006_ThisTestDoesNotExistXYZ
exit: 0
lines: 644
sha256: 4d3588e7ce28ccaafb610446cabcd52f0771c8de84c19f42aa4fb3757c914cd1
```

**The negative control also exits 0.** So the lane's exit code alone is a vacuous
signal under a `--go-run` selector, and the ~11-line delta between the two runs
cannot separate "3 passed" from "3 skipped" (a skipped test also yields `ok` and
exit 0). The lane says so itself:

```text
go-integration: NOTICE: acceptance-gate executed-assertion assertion NOT ENFORCED
for this run — a focused --run selector (TestAssistantIntegration_BUG061006) is
active. Only a full lane run with no --run selector enforces that
TestAcceptanceGate_RoutingAccuracyAndCaptureFallback ran with a non-zero
executed-assertion count.
```

Execution was therefore established by closing the skip path, with each link
measured:

1. **The three tests exist, compile under `-tags integration`, and are matched by
   the selector.** Observed directly and untruncated in the lighter lane
   (`./smackerel.sh test integration-light --go-run 'TestAssistantIntegration_BUG061006'`,
   `exit: 0`, 302 lines):

   ```text
   go-integration: applying -run selector: TestAssistantIntegration_BUG061006
   === RUN   TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea
       capture_ack_bug061006_integration_test.go:370: integration: CORE_EXTERNAL_URL not set — live core not available
   --- SKIP: TestAssistantIntegration_BUG061006_CaptureFallbackPersistsExactlyOneIdea (0.00s)
   === RUN   TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing
       capture_ack_bug061006_integration_test.go:407: integration: CORE_EXTERNAL_URL not set — live core not available
   --- SKIP: TestAssistantIntegration_BUG061006_BareShortcutPersistsNothing (0.00s)
   === RUN   TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne
       capture_ack_bug061006_integration_test.go:453: integration: CORE_EXTERNAL_URL not set — live core not available
   --- SKIP: TestAssistantIntegration_BUG061006_DuplicateResendStillPersistsExactlyOne (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/tests/integration/assistant      0.150s
   ```

2. **The file has exactly ONE skip condition**, so no other silent-skip path
   exists: `grep -n 't\.Skip'` returns a single line 193
   (`CORE_EXTERNAL_URL not set`), against 24 `t.Fatal` fail-loud calls.

3. **The full `integration` lane sets that variable**, so the one skip path
   cannot trigger there — `smackerel.sh` passes
   `-e "CORE_EXTERNAL_URL=http://smackerel-core:${core_container_port}"` into the
   Go test container.

4. **A failing test would surface**: `go-integration.sh` propagates a non-zero
   status and the lane prints `FAIL: go-integration`. Every run above printed
   `PASS: go-integration` and exited 0.

Steps 1–4 are individually measured, and together they exclude both the skip
path and the failure path in the full lane.

**Limitation recorded in the prior session — NOW RESOLVED.** That session could
not read the three `--- PASS:` lines directly: the lane emits ~652 lines, the
assistant package lands in the middle, and the retention window available then
preserved only the head and tail. The conclusion therefore rested on the measured
chain rather than on a sighting. In the CURRENT session the same command was
re-run under `evidence-capture.sh --diagnostic`, whose 2000-line ceiling exceeds
the lane's output, and the three result lines were read directly from that run's
transcript. See
[Evidence — first-party `integration` regression run](#integration-green-first-party).
The chain above is retained because it is still true and it is what makes the
exit code interpretable; it is no longer the only support for the claim.

### Recorded replay hash does not re-derive

The hash circulated with the earlier run of this command does not reproduce:

```text
[evidence-capture] MISMATCH
  recorded: 8e52582b4dad49a3d057689b39ef22ee23b5ce348b9be6c908ee6d90a029ebb5
  observed: 37391c7e710547f0ce8fc9b615e4ad0956bcfbab31977be4beb8b0ce4e9788f1
```

This is NOT evidence of a behavior change. The command's output embeds per-run
values — host RAM/disk readings, container ids, docker build step timings, test
durations, and a per-run probe nonce — so no two runs hash alike. Confirmed by
the four distinct hashes recorded above for four green runs, and again by this
session's fifth green run
(`852e7773b2180fd2dfae6e3c4c2bc09b992e4ff984ad5403f1c28c232f627afb`, 652 lines),
whose hash differs from all four. **`--verify` is not a usable replay anchor for
this command**, and no hash of it — including this session's — should be cited as
one. Each hash serves only to PIN a block of quoted output to the specific run it
came from. The durable signals are the exit code, the directly-sighted per-test
result lines, and the measured chain above.

## Deploy + Live Verification (`<target>`) {#deploy-verify}

The fix (sourceSha `777323fa`) was built, operator-cosign-signed, deployed, and
verified running on `<deploy-host>` this session (`local-operator`
trust model — build + sign happen on the target, not CI promotion).

### Build + sign (knb-owned configured tier on `<deploy-host>`)

`smackerel.sh build --target <target>` — all 9 phases green:

- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities — core + ml).
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/<operator>/smackerel-core@sha256:7bd984a316e3feec…`
  - ml   `ghcr.io/<operator>/smackerel-ml@sha256:7029c246e24446…`
- Config bundle `config-bundle-<target>-777323fa…` (sha256 `9ccd0df3…`) pushed + signed.
- Signed local build manifest emitted (`local-build-manifest-777323fa….yaml` + `.yaml.sig`).

### Deploy (recreate rollout)

The `<target>` deploy adapter verified the release proof (cosign verified BOTH
images against the operator pubkey + attestations), resolved all secrets (0
placeholders), and recreated `smackerel-core` + `smackerel-ml`. The manifest
pointer advanced to the fix release.

### Live running-state verification (this session, read-only)

Read-only `docker inspect` projection (`State.Status` / `State.Health` +
`RepoDigest` only — no env/secret fields) on the host:

```text
smackerel-<target>-smackerel-core-1 | running/healthy | sha256:7bd984a316e3feec… | MATCHES CORE FIX
smackerel-<target>-smackerel-ml-1   | running/healthy | sha256:7029c246e24446… | MATCHES ML FIX
```

Both production containers run the EXACT fix image digests and are healthy
(`Up … (healthy)`); infra services (postgres, nats, prometheus, searxng,
alertmanager) stayed up and healthy. Targeted startup-log grep on the running
core confirms the fixed capture-ack code path is the live one:

```text
INFO "telegram bot started"  bot_name:"<configured-bot>"
INFO "assistant Telegram adapter wired and bound to bot"  markdown_mode:"MarkdownV2"  max_message_chars:4096
```

### Remaining (operator behavioral smoke test)

The two **LIVE** uservalidation items require a human Telegram turn the agent
cannot perform: send `/ask <question>` (expect exactly ONE acknowledgement) and a
bare `/ask` (expect ONE honest line, never the `? Failed to save` + `saved as an
idea` pair). The fix binary is deployed + running + healthy + adapter-bound; only
the human observation remains.

## Adjacent defects discovered here, routed to their own packets

These are NOT obligations of this bug, and no work this packet owes was pushed
out onto them. Both are distinct defects noticed while tracing the capture
path; each is recorded in `state.json` with its own owner and carries its own
packet.

- Weather location resolution (`/weather <us-zip>`) + BS-006 external-lookup error
  honesty — ratified-spec design question, separate packet.
- `/status` version visibility — separate observability change.

## Deployment & Live Validation (`<target>` / `<deploy-host>`)

The fixed SHA `777323fa` was built + operator-signed ON the `<deploy-host>`
deployment target (hardware tier configured by knb), promoted through the deploy adapter,
and verified live against the running system on 2026-07-22.

### Build + sign (`./smackerel.sh build --target <target>`, all 9 phases GREEN)

```
[3/7] trivy CRITICAL/HIGH gate
  ghcr.io/<operator>/smackerel-core:<target>-777323fa3b3a (alpine 3.22.5)  Vulnerabilities: 0   PASS
  ghcr.io/<operator>/smackerel-ml:<target>-777323fa3b3a   (debian 13.6)    Vulnerabilities: 0   PASS
[4/7] docker push (stable digests)
  core: ghcr.io/<operator>/smackerel-core@sha256:7bd984a316e3feec0d97949cc6ba01cf90371d32b4de3a54b78a8602acddee4c
  ml:   ghcr.io/<operator>/smackerel-ml@sha256:7029c246e24446f607c272162ac4e13c406c5eac5c5dc6bd3f411e4f23e76a20
[5/7] cosign sign (operator key)  signed: core + ml
[6/7] syft SBOM + cosign attest   attested: core + ml
[7-8/9] config bundle             sha256: 9ccd0df36a619e30d5127cc3260989128b86a7591a7659abf7ec2e70c161d3f0
[9/9] emit local-build-manifest   local-build-manifest-777323fa3b3a…​.yaml (+ .yaml.sig)
build --target <target> COMPLETE
```

### Promote + apply (`<knb-repo>/scripts/deploy/promote.sh --product smackerel --target <target>`)

```
▶ apply: running preconditions … configured host ports unoccupied (PASS)  preconditions OK
▶ apply: verifying release proof before extraction or container start
  cosign verify core @sha256:7bd984a3… — claims validated, signature verified against public key
  cosign verify ml   @sha256:7029c246… — claims validated, signature verified against public key
  release proof verified
▶ apply: rendering effective env  declared_secret_count=9 substituted_secret_count=9 placeholder_remaining_count=0
▶ apply: running rollout strategy: recreate
  smackerel-core-1 Recreated → Started ; smackerel-ml-1 Recreated → Started ; postgres/nats Healthy
▶ verify: waiting for strict current-release health
  acceptance: core-digest=accepted ml-digest=accepted health=accepted config-generation=accepted
  acceptance: drift-state=accepted current-release=accepted
verify OK (strict current release accepted)
apply OK
```

### Live validation against the running system

```
smackerel-<target>-smackerel-core-1
  image:  ghcr.io/<operator>/smackerel-core@sha256:7bd984a316e3feec0d97949cc6ba01cf90371d32b4de3a54b78a8602acddee4c
   state:  running health=healthy
smackerel-<target>-smackerel-ml-1
  image:  ghcr.io/<operator>/smackerel-ml@sha256:7029c246e24446f607c272162ac4e13c406c5eac5c5dc6bd3f411e4f23e76a20
   state:  running health=healthy
core logs:
  "telegram bot started","bot_name":"<configured-bot>"
  "assistant facade wired","scenarios":17
  "assistant Telegram adapter wired and bound to bot","markdown_mode":"MarkdownV2"
  "assistant facade wired after deferred retries","attempts":3   (early ml-sidecar warm-up race self-healed)
```

The running `smackerel-core` digest `sha256:7bd984a3…` is byte-identical to the
signed fix build, so the fixed capture-acknowledgement code path is the live
binary, and the exact layer changed by this fix
(`assistant Telegram adapter wired and bound to bot`) is confirmed active.

### Remaining (operator-only)

The two **LIVE** items in `uservalidation.md` require observing the actual
Telegram exchange, which only the operator can perform (send `/ask <question>`
and a bare `/ask` to `<configured-bot>`). The fixed binary is deployed and healthy;
the 30-second UX confirmation is handed to the operator.
