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
to the running self-hosted home-lab host; the running containers were verified
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

## Test Evidence

See "After Fix — unit evidence" above. Command exit status: the CLI printed
`[go-unit] go test ./... finished OK` and returned success.

## Deploy + Live Verification (self-hosted home-lab) {#deploy-verify}

The fix (sourceSha `777323fa`) was built, operator-cosign-signed, deployed, and
verified running on the self-hosted home-lab host this session (`local-operator`
trust model — build + sign happen on the target, not CI promotion).

### Build + sign (accel tier, on the target)

`smackerel.sh build --target self-hosted` — all 9 phases green:

- Trivy CRITICAL/HIGH gate: PASS (0 vulnerabilities — core + ml).
- Pushed + cosign-signed (operator key) + SBOM-attested:
  - core `ghcr.io/pkirsanov/smackerel-core@sha256:7bd984a316e3feec…`
  - ml   `ghcr.io/pkirsanov/smackerel-ml@sha256:7029c246e24446…`
- Config bundle `config-bundle-self-hosted-777323fa…` (sha256 `9ccd0df3…`) pushed + signed.
- Signed local build manifest emitted (`local-build-manifest-777323fa….yaml` + `.yaml.sig`).

### Deploy (recreate rollout)

The home-lab deploy adapter verified the release proof (cosign verified BOTH
images against the operator pubkey + attestations), resolved all secrets (0
placeholders), and recreated `smackerel-core` + `smackerel-ml`. The manifest
pointer advanced to the fix release.

### Live running-state verification (this session, read-only)

Read-only `docker inspect` projection (`State.Status` / `State.Health` +
`RepoDigest` only — no env/secret fields) on the host:

```text
smackerel-home-lab-smackerel-core-1 | running/healthy | sha256:7bd984a316e3feec… | MATCHES CORE FIX
smackerel-home-lab-smackerel-ml-1   | running/healthy | sha256:7029c246e24446… | MATCHES ML FIX
```

Both production containers run the EXACT fix image digests and are healthy
(`Up … (healthy)`); infra services (postgres, nats, prometheus, searxng,
alertmanager) stayed up and healthy. Targeted startup-log grep on the running
core confirms the fixed capture-ack code path is the live one:

```text
INFO "telegram bot started"  bot_name:"smackerel_bot"
INFO "assistant Telegram adapter wired and bound to bot"  markdown_mode:"MarkdownV2"  max_message_chars:4096
```

### Remaining (operator behavioral smoke test)

The two **LIVE** uservalidation items require a human Telegram turn the agent
cannot perform: send `/ask <question>` (expect exactly ONE acknowledgement) and a
bare `/ask` (expect ONE honest line, never the `? Failed to save` + `saved as an
idea` pair). The fix binary is deployed + running + healthy + adapter-bound; only
the human observation remains.

## Discovered follow-ups (not this bug)

- Weather location resolution (`/weather <us-zip>`) + BS-006 external-lookup error
  honesty — ratified-spec design question, separate packet.
- `/status` version visibility — separate observability change.

## Deployment & Live Validation (home-lab / `<deploy-host>`)

The fixed SHA `777323fa` was built + operator-signed ON the `<deploy-host>`
home-lab target (hardware tier `accel`), promoted through the deploy adapter,
and verified live against the running system on 2026-07-22.

### Build + sign (`./smackerel.sh build --target self-hosted`, all 9 phases GREEN)

```
[3/7] trivy CRITICAL/HIGH gate
  ghcr.io/pkirsanov/smackerel-core:self-hosted-777323fa3b3a (alpine 3.22.5)  Vulnerabilities: 0   PASS
  ghcr.io/pkirsanov/smackerel-ml:self-hosted-777323fa3b3a   (debian 13.6)    Vulnerabilities: 0   PASS
[4/7] docker push (stable digests)
  core: ghcr.io/pkirsanov/smackerel-core@sha256:7bd984a316e3feec0d97949cc6ba01cf90371d32b4de3a54b78a8602acddee4c
  ml:   ghcr.io/pkirsanov/smackerel-ml@sha256:7029c246e24446f607c272162ac4e13c406c5eac5c5dc6bd3f411e4f23e76a20
[5/7] cosign sign (operator key)  signed: core + ml
[6/7] syft SBOM + cosign attest   attested: core + ml
[7-8/9] config bundle             sha256: 9ccd0df36a619e30d5127cc3260989128b86a7591a7659abf7ec2e70c161d3f0
[9/9] emit local-build-manifest   local-build-manifest-777323fa3b3a…​.yaml (+ .yaml.sig)
build --target self-hosted COMPLETE
```

### Promote + apply (`knb scripts/deploy/promote.sh --product smackerel --target home-lab`)

```
▶ apply: running preconditions … port 41001/41002 unoccupied (PASS)  preconditions OK
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
smackerel-home-lab-smackerel-core-1
   image:  ghcr.io/pkirsanov/smackerel-core@sha256:7bd984a316e3feec0d97949cc6ba01cf90371d32b4de3a54b78a8602acddee4c
   state:  running health=healthy
smackerel-home-lab-smackerel-ml-1
   image:  ghcr.io/pkirsanov/smackerel-ml@sha256:7029c246e24446f607c272162ac4e13c406c5eac5c5dc6bd3f411e4f23e76a20
   state:  running health=healthy
core logs:
  "telegram bot started","bot_name":"smackerel_bot"
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
and a bare `/ask` to `@smackerel_bot`). The fixed binary is deployed and healthy;
the 30-second UX confirmation is handed to the operator.
