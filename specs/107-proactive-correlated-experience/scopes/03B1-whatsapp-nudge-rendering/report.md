# Report: SCOPE-03B1 WhatsApp Proactive Nudge Rendering

## Summary

Implementation record (`bubbles.implement`, 2026-07-25). Renders a
`permit`/`escalated` `ProactiveCardModel` as a WhatsApp interactive message over
the shipped spec-072 transport (`internal/whatsapp/assistant_adapter/`), routes a
chosen interactive/list `reply.id` through the SCOPE-01 `NudgeRegistry` to the one
`Acknowledge(content_key)` path, proves WhatsApp↔Telegram single-pair suppression
(consuming BOTH built channels — SCOPE-03A Telegram + this WhatsApp — no web), and
renders the honest WhatsApp states — never a fabricated card. Buildable on the
SCOPE-01 foundation (`internal/proactive`) + the delivered SCOPE-03A Telegram
render + the in-tree spec-078 controller + the shipped spec-072 transport — no web
(spec-106), spec-105, or `SCOPE-02` dependency. The cross-channel-everywhere
parity and the Telegram/WhatsApp→web reflection are `SCOPE-03B2` and stay gated.

**Boundary (verified):** ADDITIVE-ONLY. New files only
(`internal/whatsapp/assistant_adapter/render_nudge.go` + `nudge_reply_test.go` +
`nudge_render_golden_test.go`; `tests/integration/proactive/whatsapp_*`; one
additive test appended to the existing `tests/e2e/proactive_channel_parity_e2e_test.go`).
No destructive edit to spec-072's `adapter.go` / `render.go` / `webhook_handler.go`
/ `idempotency.go` / `ratelimit.go` / `decodeInteractivePayload`; the `a:n:`
reply-id is structurally distinct from spec-072's `d:`/`c:`/`r:` families and is
cleanly refused by spec-072's own dispatch (`ErrUnsupportedMessageType`) —
additive non-interference, spec-072's existing tests unbroken. `internal/intelligence/surfacing/types.go`
NOT edited (the `whatsapp` `Channel` value is a call-site
`surfacing.Channel("whatsapp")` reservation + coordination note, FR-107-012). No
`content_key`/label/query on any `reply.id` or telemetry (FR-107-028). Every card
originates from one spec-078 `controller.Propose` verdict; no second
budget/store/cache. No edits under specs/105, specs/106, specs/072, specs/078,
spec artifacts, `internal/intelligence/surfacing` internals, or `internal/web`. No
commit / push / deploy.

## Test-File Note

The authored test files match the Test Plan's referenced filenames exactly
(`internal/whatsapp/assistant_adapter/nudge_reply_test.go`,
`internal/whatsapp/assistant_adapter/nudge_render_golden_test.go`,
`tests/integration/proactive/whatsapp_nudge_ack_test.go`,
`tests/integration/proactive/whatsapp_telegram_pair_suppression_test.go`,
`tests/integration/proactive/whatsapp_honest_states_test.go`,
`tests/e2e/proactive_channel_parity_e2e_test.go`) — no Test-Plan filename
reconciliation was required; `traceability-guard` stays 0.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-006; FR-107-009/011/012/028; WhatsApp-observed portions of FR-107-007/008)
- Design source: `../../design.md` (`## Concrete Implementations` P2 WhatsApp, OQ2, OQ7, `## Single-Controller Routing`)
- Depends on: SCOPE-03A; external gate spec-072 WhatsApp transport usable (in-tree, `done`/certified). NOT gated on SCOPE-02/spec-106/spec-105.
- Planning owner: `bubbles.plan` (split of the former SCOPE-03B)

## Test Evidence (Executed — current session, 2026-07-25)

All evidence below is raw terminal output captured in the current session via the
repo CLI (`./smackerel.sh`). No mocked internal component: the integration lane
wires the REAL spec-078 controller + REAL `NudgeRegistry`/`NudgeAck` + REAL
WhatsApp renderer through an INJECTED CloudClient/reply seam (no real WhatsApp
network).

### Build / compile — `#build-check`

`./smackerel.sh check` (config SST + scenario lint; the Go packages compile under
the unit/integration lanes below):

```text
config-validate: /home/.../config/generated/dev.env.tmp.1676212 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### Unit + golden lane (shared raw run) — `#t107-006-u` `#t107-03b1-golden` `#t107-03b1-fallback`

`./smackerel.sh test unit --go --go-run 'TestSCN107006' --verbose` — white-box
`internal/whatsapp/assistant_adapter` tests. One `go test ./...` run exercises all
seven SCOPE-03B1 unit/golden assertions; each anchor below is proven by the named
test in this single raw block:

- **`#t107-006-u`** — `TestSCN107006_NudgeReplyIDCarriesOpaqueWireForm` (reply.id is exactly `a:n:<ref>:<a|s|d>`, never the content_key, ≤256 bytes, round-trips) + `TestSCN107006_NudgeReplyIDDoesNotCollideWithSpec072Families` (a:n: vs spec-072 d:/c:/r: non-collision; spec-072's `decodeInteractivePayload` refuses a:n: with `ErrUnsupportedMessageType` and still decodes its own confirm family — spec-072 unbroken) + `TestSCN107006_TapReplyRoutesToOneAck` (tap → exactly one `Acknowledge(content_key)`, idempotent, never mis-acks a confirm id, nil-ack fails loud).
- **`#t107-03b1-golden`** — `TestSCN107006_NudgeGoldenInteractiveRender` (body `"<title>\n\nWhy: From your alerts"` + 3 `a:n:` Act/Snooze/Dismiss buttons; escalated → `URGENT ESCALATION` marker) + `TestSCN107006_NonCardStateRendersHonestTextNoButtons` (every non-card state → no interactive card, distinct honest text) + `TestSCN107006_RenderNudgeRefusesNonCardAndSendsOnceViaSeam` (RenderNudge sends exactly one interactive via the injected seam; refuses non-card / nil cloud / empty phone).
- **`#t107-03b1-fallback`** — `TestSCN107006_ListAndNumberedTextFallbackPreserveActionsAndProvenance` (interactive-list 3 rows carrying the same `a:n:` ids + provenance; numbered plain-text `1|2|3 → Act|Snooze|Dismiss` + provenance; `NumberedFallbackReply` decode).

```text
=== RUN   TestSCN107006_NudgeGoldenInteractiveRender
--- PASS: TestSCN107006_NudgeGoldenInteractiveRender (0.00s)
=== RUN   TestSCN107006_NonCardStateRendersHonestTextNoButtons
--- PASS: TestSCN107006_NonCardStateRendersHonestTextNoButtons (0.00s)
=== RUN   TestSCN107006_RenderNudgeRefusesNonCardAndSendsOnceViaSeam
--- PASS: TestSCN107006_RenderNudgeRefusesNonCardAndSendsOnceViaSeam (0.00s)
=== RUN   TestSCN107006_NudgeReplyIDCarriesOpaqueWireForm
--- PASS: TestSCN107006_NudgeReplyIDCarriesOpaqueWireForm (0.00s)
=== RUN   TestSCN107006_NudgeReplyIDDoesNotCollideWithSpec072Families
--- PASS: TestSCN107006_NudgeReplyIDDoesNotCollideWithSpec072Families (0.00s)
=== RUN   TestSCN107006_TapReplyRoutesToOneAck
--- PASS: TestSCN107006_TapReplyRoutesToOneAck (0.00s)
=== RUN   TestSCN107006_ListAndNumberedTextFallbackPreserveActionsAndProvenance
--- PASS: TestSCN107006_ListAndNumberedTextFallbackPreserveActionsAndProvenance (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter     0.029s
[go-unit] go test ./... finished OK
UNIT_EXIT=0
```

### Integration-light lane (shared raw run) — `#t107-006-i` `#t107-03b1-watgsuppress` `#t107-03b1-honest`

`./smackerel.sh test integration-light --go-run 'TestSCN107006'` — the stores-only
(postgres+nats) ephemeral live stack. Each SCOPE-03B1 integration test wires the
REAL spec-078 `Controller` (`newController` → `controller.Propose`), the REAL
process-wide `InMemoryAck`, the REAL `NudgeRegistry`/`NudgeAck`, the REAL WhatsApp
renderer (`render_nudge.go`), and — for the pair test — the REAL SCOPE-03A Telegram
renderer, through an INJECTED `CloudClient`/reply seam (no live WhatsApp network,
no HTTP). No mocked internal component. Each anchor below is proven by the named
test in this single raw block:

- **`#t107-006-i`** — `TestSCN107006_WhatsAppInteractiveChoiceRoutesToOneAckPath`
  (producer → one `controller.Propose` → permit → WhatsApp interactive render via
  the seam → `Act` `reply.id = a:n:<ref>:a` → `HandleNudgeReply` → the ONE
  `Acknowledge(content_key)` on the sink the controller's `SuppressionWindow` reads;
  no ack before the reply, ack visible after — single ack path).
- **`#t107-03b1-watgsuppress`** — `TestSCN107006_ActOnWhatsAppSuppressesSameContentKeyOnTelegramRerender`
  (act on WhatsApp → Telegram re-`Propose` of the same `content_key` returns
  `suppressed`, projects NO card, honest text-only re-render, no keyboard) +
  `TestSCN107006_ActOnTelegramSuppressesSameContentKeyOnWhatsAppRerender` (the
  reverse direction: act on Telegram → WhatsApp re-`Propose` `suppressed`, no
  interactive card, honest text). Both directions of the WhatsApp↔Telegram pair.
- **`#t107-03b1-honest`** — `TestSCN107006_HonestWhatsAppStatesRenderDistinctlyNeverACard`
  (budget-exhausted [budget-1 controller defers a 2nd key], deduped [same key twice
  in the dedupe window], already-handled [2nd reply on a consumed ref is idempotent],
  expired [unknown ref resolves honestly, never a silent success] — each derived
  from a REAL controller verdict or REAL ack outcome, each a distinct non-empty
  text line, `BuildNudgeInteractive` draws NO card for any non-card state).

```text
 Container smackerel-test-nats-1       Up 5 seconds (healthy)   127.0.0.1:47002->4222/tcp
 Container smackerel-test-postgres-1   Up 5 seconds (healthy)   127.0.0.1:47001->5432/tcp
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
2026/07/25 18:01:08 INFO dbmigrate: all migrations applied
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: TestSCN107006
=== RUN   TestSCN107006_HonestWhatsAppStatesRenderDistinctlyNeverACard
--- PASS: TestSCN107006_HonestWhatsAppStatesRenderDistinctlyNeverACard (0.00s)
=== RUN   TestSCN107006_WhatsAppInteractiveChoiceRoutesToOneAckPath
--- PASS: TestSCN107006_WhatsAppInteractiveChoiceRoutesToOneAckPath (0.00s)
=== RUN   TestSCN107006_ActOnWhatsAppSuppressesSameContentKeyOnTelegramRerender
--- PASS: TestSCN107006_ActOnWhatsAppSuppressesSameContentKeyOnTelegramRerender (0.00s)
=== RUN   TestSCN107006_ActOnTelegramSuppressesSameContentKeyOnWhatsAppRerender
--- PASS: TestSCN107006_ActOnTelegramSuppressesSameContentKeyOnWhatsAppRerender (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/proactive      0.040s
PASS: go-integration-light
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
INTEGRATION_LIGHT_EXIT=0
```

**Claim Source:** executed (current session, 2026-07-25; stores-only ephemeral
stack created healthy, migrated, tests PASS, torn down — containers/volumes/network
all removed). Covers DoD Core Outcomes 2/3/4 (items 144/145/146) and Test Evidence
rows T107-006-I (152), T107-03B1-WATGSUPPRESS (156), T107-03B1-HONEST (157).

### T107-006-A — WhatsApp interactive nudge acknowledges through controller (e2e) — `#t107-006-a`

`./smackerel.sh test e2e --go-run 'TestSCN107006'` — the FULL disposable stack
(postgres + nats + ollama + ml + core, built with `-tags e2e`, brought up healthy,
torn down on exit). `TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController`
drives the REAL single spec-078 `Controller` (`NewController` → `Propose` → permit),
the REAL `NudgeRegistry`/`NudgeAck`, and the REAL WhatsApp renderer through an
injected `waE2ECloud` seam (no live WhatsApp Cloud API): permit → mint opaque ref →
project card → `RenderNudge` sends exactly one interactive with 3 `a:n:<ref>:<a|s|d>`
buttons → `Act` `reply.id` → `HandleNudgeReply` → the ONE `Acknowledge(content_key)`
on the SAME sink the controller reads (not acked before the reply, acked after).

```text
config-validate: .../config/generated/test.env.tmp OK
Smackerel pre-flight resource check: OK
  RAM  available: 37835 MB (required >= 6000 MB)
  Disk available: 603172 MB (required >= 15 GB)
#32 [smackerel-core builder 7/8] RUN ... go build -tags "e2e" ... -o /bin/smackerel-core ./cmd/core
#32 DONE 43.1s
go-e2e: applying -run selector: TestSCN107006
=== RUN   TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController
--- PASS: TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.111s
PASS: go-e2e
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-nats-1  Removed
 Container smackerel-test-ollama-1  Removed
 Network smackerel-test_default  Removed
E2E_EXIT=0
```

**Claim Source:** executed (current session, 2026-07-25; full disposable stack
built `-tags e2e`, brought up, test PASS, all containers/volumes/network removed on
teardown). Covers DoD Test Evidence row T107-006-A (item 153). (Note: an unrelated
`connector:qf-decisions` health probe reported `error` in the pre-test health dump
— a pre-existing external-connector config state, NOT a SCOPE-03B1 surface; the
WhatsApp e2e test itself PASSED and is independent of that connector.)

### Build Quality Gate — `#build-quality`

SCOPE-03B1's OWN build-quality gates are all green. The one repo-wide red
(`format --check` exit 1) is FOREIGN-owned (two files outside this scope's
boundary, unmodified in-tree, not touched, not bypassed).

```text
# lint (Go golangci + Python ruff + web manifests)
All checks passed!
Web validation passed
LINT_EXIT=0

# format --check (repo-wide) — FOREIGN-only failures, neither a SCOPE-03B1 file:
internal/api/graphapi/activation.go
internal/web/handler_test.go
FORMAT_CHECK_EXIT=1

# SCOPE-03B1 OWN files gofmt -l (read-only isolation; empty = all clean):
GOFMT_OWN_MISFORMATTED_EXIT=0   (0 own files listed → all 7 gofmt-clean)

# artifact-lint
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0

# traceability-guard
ℹ️  DoD fidelity: 20 scenarios checked, 20 mapped to DoD, 0 unmapped
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0

# spec-072 boundary — existing files unmodified (git status internal/whatsapp/):
?? internal/whatsapp/assistant_adapter/nudge_render_golden_test.go
?? internal/whatsapp/assistant_adapter/nudge_reply_test.go
?? internal/whatsapp/assistant_adapter/render_nudge.go
#   (NO M on adapter.go/render.go/webhook_handler.go/idempotency.go/ratelimit.go)

# spec-072 own tests still pass alongside the additive code (non-interference):
ok      github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter  0.0..s
SPEC072_UNIT_EXIT=0
```

**Foreign-owned format failures (recorded, NOT touched, NOT bypassed):**
`internal/api/graphapi/activation.go` and `internal/web/handler_test.go` are
pre-existing gofmt-dirty files from a concurrent sibling session's out-of-boundary
work (the same two SCOPE-03A recorded at its finalization). Neither is a SCOPE-03B1
file; both are outside this scope's Change Boundary. The task's named sibling
digest files (`internal/config/config.go`, `tests/integration/digest_typed_read_test.go`,
`DIGEST_STALE_AFTER_HOURS` wiring) were left entirely untouched — no
`DIGEST_STALE_AFTER_HOURS` was added anywhere. Per the Change-Boundary and
foreign-ownership rules, item 161 is checked on SCOPE-03B1's OWN quality, which is
green: lint 0, all 7 own `.go` files gofmt-clean, artifact-lint 0, traceability 0
(0 warnings, 20/20 DoD fidelity), spec-072 own tests green.

**Claim Source:** executed (current session, 2026-07-25).

## Completion Statement

SCOPE-03B1 (WhatsApp Proactive Nudge Rendering) is **DONE — 13/13 DoD [x]**. All
seven Test-Plan rows are proven with current-session raw evidence:

- Unit + adapter-golden (T107-006-U, T107-03B1-GOLDEN, T107-03B1-FALLBACK) —
  `test unit --go --go-run TestSCN107006` exit 0 (7 assertions PASS).
- Integration-light (T107-006-I, T107-03B1-WATGSUPPRESS, T107-03B1-HONEST) —
  `test integration-light --go-run TestSCN107006` exit 0 (4/4 PASS on the
  stores-only ephemeral stack, torn down).
- E2E (T107-006-A) — `test e2e --go-run TestSCN107006` exit 0
  (`TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController` PASS on
  the full disposable stack, torn down).

Build Quality Gate green on SCOPE-03B1's own surface (lint 0; 7 own files
gofmt-clean; artifact-lint 0; traceability 0/0-warn/20-of-20; spec-072 own tests
pass). Boundary intact: ADDITIVE-only (3 new `internal/whatsapp/assistant_adapter/`
files + 3 new `tests/integration/proactive/whatsapp_*` files + 1 additive test in
the existing `tests/e2e/proactive_channel_parity_e2e_test.go`); spec-072's
adapter/render/webhook/idempotency/ratelimit files unmodified;
`internal/intelligence/surfacing/types.go` unedited (the `whatsapp` `Channel` is a
call-site reservation + coordination note); no edits under specs/072/078/105/106,
`internal/web`, or the sibling BUG-002-007 digest files; no `DataProvenance`
fabrication, no `content_key`/label on any `reply.id`; no commit/push/deploy. The
spec remains `blocked` (SCOPE-03B2 web parity + SCOPE-02/04-09 gated on the
in-progress spec-106/spec-105).
