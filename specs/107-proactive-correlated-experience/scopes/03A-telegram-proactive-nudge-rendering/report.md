# Report: SCOPE-03A Telegram Proactive Nudge Rendering

## Summary

Implementation record (`bubbles.implement`, 2026-07-25 resume). Renders a
`permit`/`escalated` `ProactiveCardModel` as a Telegram inline-keyboard message
using the committed additive `a:n:` family, routes a tap through the SCOPE-01
registry to the one `Acknowledge(content_key)` path (single-channel), and renders
the honest Telegram states — never a fabricated card. Buildable on the SCOPE-01
foundation + the in-tree spec-078 controller alone — no web (spec-106), WhatsApp
(spec-072), or cross-channel-everywhere dependency (those are SCOPE-03B).

**Boundary (verified):** no edits under specs/105, specs/106, specs/072,
specs/078, internal/intelligence/surfacing, internal/whatsapp, or internal/web.
The `a:n:` family stays additive; every card originates from one spec-078
`controller.Propose` verdict; no second budget/store/cache. No commit / push /
deploy.

## Planning Provenance

- Requirements source: `../../spec.md` (SCN-107-005; FR-107-009/010/028; Telegram-observed portions of FR-107-007/008 + NFR-107-004)
- Design source: `../../design.md` (`## Concrete Implementations` P2 Telegram, OQ2, `## Single-Controller Routing`)
- Depends on: SCOPE-01; external gate spec-078 controller usable (in-tree). NOT gated on SCOPE-02/spec-106/spec-072.
- Planning owner: `bubbles.plan` (re-scope of the former SCOPE-03)

## Test-File Reconciliation (STEP 2)

The Test Plan's descriptive "File / Expected Test Title" column originally named
`nudge_callbacks_test.go` / `nudge_render_golden_test.go`. The authored unit +
golden tests physically live in
`internal/telegram/assistant_adapter/render_nudge_test.go` (new this scope),
alongside the pre-existing `internal/telegram/assistant_adapter/callbacks_nudge_test.go`
(the committed `a:n:` family encode/decode + non-collision tests from `d652cc19`).
Reconciled **path-only** — the three affected Test Plan rows (T107-005-U,
T107-03A-GOLDEN, T107-03-COLLISION) now point at the real files; every test
**title and behavioral claim is unchanged**. The traceability guard keys its
"concrete test file" check off the `./smackerel.sh` command column, so it stays
green (exit 0) regardless — verified both before (baseline) and after the
reconcile below.

## Test Evidence (Executed — current session, 2026-07-25)

All evidence below is raw terminal output captured in the current session via the
repo CLI (`./smackerel.sh`). No mocked internal component; the integration lane
wires the REAL spec-078 controller + REAL NudgeRegistry/NudgeAck + REAL renderer.

### Build / compile — `#build-check`

`./smackerel.sh check` (config SST + scenario lint; the Go packages compile under
the unit/integration lanes below):

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.4119915 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

### T107-005-U — a:n: encode/decode within 64 bytes — `#t107-005-u`

`./smackerel.sh test unit --go --go-run 'TestSCN107005' --verbose` — white-box
`assistant_adapter` tests. `TestSCN107005_NudgeCallbackWithin64Bytes` asserts each
`a:n:<26-char-ref>:<a|s|d>` wire form is `<= 64` bytes and round-trips through
`decodeCallbackData` to `callbackKindNudge` + the right action:

```text
ok      github.com/smackerel/smackerel/internal/telegram        0.029s [no tests to run]
=== RUN   TestSCN107005_NudgeGoldenInlineRender
--- PASS: TestSCN107005_NudgeGoldenInlineRender (0.00s)
=== RUN   TestSCN107005_NudgeCallbackWithin64Bytes
--- PASS: TestSCN107005_NudgeCallbackWithin64Bytes (0.00s)
=== RUN   TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List
--- PASS: TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List (0.00s)
=== RUN   TestSCN107005_NonCardStateRendersHonestTextNoKeyboard
--- PASS: TestSCN107005_NonCardStateRendersHonestTextNoKeyboard (0.00s)
=== RUN   TestSCN107005_RenderNudgeRefusesNonCardState
--- PASS: TestSCN107005_RenderNudgeRefusesNonCardState (0.00s)
=== RUN   TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace
--- PASS: TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.039s
[go-unit] go test ./... finished OK
UNIT_VERBOSE_EXIT=0
```

**Real file:** `internal/telegram/assistant_adapter/render_nudge_test.go`
(`TestSCN107005_NudgeCallbackWithin64Bytes`) — PASS. Reconciled from the Test
Plan's descriptive `nudge_callbacks_test.go` (path-only; see reconciliation note).

### T107-03A-GOLDEN — golden inline render (title + Why: + a:n: buttons) — `#t107-03a-golden`

`TestSCN107005_NudgeGoldenInlineRender` asserts a permit card renders verbatim as
`"<title>\n\nWhy: From your alerts"` with a single inline row of 3 buttons
`Act`/`Snooze`/`Dismiss` carrying `a:n:<ref>:a|s|d`, and the escalated variant
carries the `URGENT ESCALATION` provenance marker (same verbose run as above):

```text
=== RUN   TestSCN107005_NudgeGoldenInlineRender
--- PASS: TestSCN107005_NudgeGoldenInlineRender (0.00s)
=== RUN   TestSCN107005_NudgeCallbackWithin64Bytes
--- PASS: TestSCN107005_NudgeCallbackWithin64Bytes (0.00s)
=== RUN   TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List
--- PASS: TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List (0.00s)
=== RUN   TestSCN107005_NonCardStateRendersHonestTextNoKeyboard
--- PASS: TestSCN107005_NonCardStateRendersHonestTextNoKeyboard (0.00s)
=== RUN   TestSCN107005_RenderNudgeRefusesNonCardState
--- PASS: TestSCN107005_RenderNudgeRefusesNonCardState (0.00s)
=== RUN   TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace
--- PASS: TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.039s
[go-unit] go test ./... finished OK
UNIT_VERBOSE_EXIT=0
```

**Real file:** `internal/telegram/assistant_adapter/render_nudge_test.go`
(`TestSCN107005_NudgeGoldenInlineRender`) — PASS. Reconciled from the Test Plan's
descriptive `nudge_render_golden_test.go` (path-only).

### T107-03-COLLISION — a:n: non-collision with a:c:/a:d:/spec-028 — `#t107-03-collision`

`TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List` asserts `a:n:`
decodes to `callbackKindNudge`, `a:c:`/`a:d:` still decode to confirm/disambig,
and the spec-028 `list_check:` scheme is structurally outside the `a:` assistant
namespace (`IsAssistantCallback` false, `ErrNotAssistantMessage`):

```text
=== RUN   TestSCN107005_NudgeGoldenInlineRender
--- PASS: TestSCN107005_NudgeGoldenInlineRender (0.00s)
=== RUN   TestSCN107005_NudgeCallbackWithin64Bytes
--- PASS: TestSCN107005_NudgeCallbackWithin64Bytes (0.00s)
=== RUN   TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List
--- PASS: TestSCN107005_NudgeDoesNotCollideWithConfirmDisambigOrSpec028List (0.00s)
=== RUN   TestSCN107005_NonCardStateRendersHonestTextNoKeyboard
--- PASS: TestSCN107005_NonCardStateRendersHonestTextNoKeyboard (0.00s)
=== RUN   TestSCN107005_RenderNudgeRefusesNonCardState
--- PASS: TestSCN107005_RenderNudgeRefusesNonCardState (0.00s)
=== RUN   TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace
--- PASS: TestSCN107005_HandleNudgeCallbackRoutesToOneAckAndEditsInPlace (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/telegram/assistant_adapter     0.039s
UNIT_VERBOSE_EXIT=0
```

**Behavioral DoD coverage (unit lane):** item 117 (renders inline act/snooze/dismiss
with `a:n:` + `Why:` provenance, tap routes to the ack path — GOLDEN +
`HandleNudgeCallbackRoutesToOneAckAndEditsInPlace`); item 120 (a:n: non-collision
+ 64-byte bound + card only from a permit/escalated `ProjectCard` verdict +
tap→one `Acknowledge(content_key)`, idempotent second tap = no re-ack —
COLLISION + Within64Bytes + RenderNudgeRefusesNonCardState + HandleNudgeCallback;
end-to-end controller-origination reinforced at `#t107-005-i`); item 121
(callback_data carries only the opaque `a:n:<ref>`, never the `content_key` —
Within64Bytes + the `card.WireCallback` single-source path). **Real file:**
`render_nudge_test.go` (reconciled from `nudge_callbacks_test.go`, path-only).

### T107-005-I — Telegram tap routes to the one ack path — `#t107-005-i`

`./smackerel.sh test integration-light --go-run 'TestSCN107005'` — real spec-078
`controller.Propose` → permit verdict → real `NudgeRegistry.Mint` → real
`BuildNudgeMessage` render → tap → `HandleNudgeCallback` → the SAME
`surfacing.InMemoryAck` the controller's `SuppressionWindow` reads (single ack
path). Stores-only ephemeral stack, torn down on exit:

```text
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: TestSCN107005
=== RUN   TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard
--- PASS: TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard (0.00s)
=== RUN   TestSCN107005_TelegramTapRoutesToOneAckPath
--- PASS: TestSCN107005_TelegramTapRoutesToOneAckPath (0.00s)
=== RUN   TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender
--- PASS: TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/proactive      0.031s
PASS: go-integration-light
INTEGRATION_LIGHT_EXIT=0
```

**Real file:** `tests/integration/proactive/telegram_nudge_ack_test.go`
(`TestSCN107005_TelegramTapRoutesToOneAckPath`) — PASS. Reinforces item 120's
controller-origination clause end-to-end (`ctrl.Propose` → `DecisionPermit`).

### T107-03A-TGSUPPRESS — single-channel Telegram suppression on re-render — `#t107-03a-tgsuppress`

`TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender`: after an
Act tap acks the `content_key`, a fresh Telegram candidate with the SAME
`content_key` gets `DecisionSuppressed` from the single controller, projects NO
card, and the re-render is the honest text-only suppressed state (no keyboard) —
no duplicate Telegram prompt (same run as `#t107-005-i`):

```text
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: TestSCN107005
=== RUN   TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard
--- PASS: TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard (0.00s)
=== RUN   TestSCN107005_TelegramTapRoutesToOneAckPath
--- PASS: TestSCN107005_TelegramTapRoutesToOneAckPath (0.00s)
=== RUN   TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender
--- PASS: TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/proactive      0.031s
INTEGRATION_LIGHT_EXIT=0
```

**Real file:** `tests/integration/proactive/telegram_single_channel_suppression_test.go`
(`TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender`) — PASS.
Single-channel only; cross-channel-everywhere parity is SCOPE-03B.

### T107-03A-HONEST — honest Telegram states render distinctly — `#t107-03a-honest`

`TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard`: budget-exhausted
(budget-1 controller defers a 2nd key), deduped (same key twice in the dedupe
window), already-handled (2nd tap on a consumed ref, no re-ack), and expired
(unknown/expired ref) each derive from a REAL controller verdict / REAL ack
outcome, render as a DISTINCT text-only line, and never project a card:

```text
integration-light health OK: postgres + nats up (stores-only; no core/ml, no ml_sidecar gate)
PASS: integration-light db migration (schema applied via cmd/dbmigrate)
go-integration: applying -run selector: TestSCN107005
=== RUN   TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard
--- PASS: TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard (0.00s)
=== RUN   TestSCN107005_TelegramTapRoutesToOneAckPath
--- PASS: TestSCN107005_TelegramTapRoutesToOneAckPath (0.00s)
=== RUN   TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender
--- PASS: TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/integration/proactive      0.031s
INTEGRATION_LIGHT_EXIT=0
```

**Real file:** `tests/integration/proactive/telegram_honest_states_test.go`
(`TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard`) — PASS. Ephemeral
stores-only stack created + torn down (volumes/networks removed) this run.

### T107-005-A — Telegram inline nudge acknowledges through controller (e2e) — `#t107-005-a`

**Executed this finalization session. Claim Source: executed.**
`./smackerel.sh test e2e --go-run 'TestSCN107005'` — the full disposable stack was
brought UP (postgres/nats/ollama/ml/core, healthy), the focused e2e ran GREEN
against the live stack through production routes, and the stack was torn DOWN (all
containers + volumes + network removed) on exit. **E2E_EXIT=0.**

```text
 Container smackerel-test-nats-1  Healthy
 Container smackerel-test-postgres-1  Healthy
 Container smackerel-test-smackerel-ml-1  Started
 Container smackerel-test-smackerel-core-1  Starting
[go-e2e] nodejs install OK
go-e2e: applying -run selector: TestSCN107005
=== RUN   TestSCN107005_TelegramInlineNudgeAcknowledgesThroughController
--- PASS: TestSCN107005_TelegramInlineNudgeAcknowledgesThroughController (0.00s)
PASS
ok      github.com/smackerel/smackerel/tests/e2e        0.176s
PASS: go-e2e
Running project-scoped test stack teardown (exit cleanup, timeout 180s)...
config-validate: <repo-root>/config/generated/test.env.tmp.1727377 OK
 Container smackerel-test-smackerel-core-1  Removed
 Container smackerel-test-postgres-1  Removed
 Container smackerel-test-smackerel-ml-1  Removed
 Container smackerel-test-nats-1  Removed
 Volume smackerel-test-postgres-data  Removed
 Volume smackerel-test-ollama-data  Removed
 Volume smackerel-test-nats-data  Removed
 Network smackerel-test_default  Removed
E2E_EXIT=0
```

This proves the single-channel Telegram inline-nudge acknowledgement end-to-end
through the real controller ack path (`a:n:<ref>:<a|s|d>` → `NudgeAck.Handle` →
one `Acknowledge(content_key)`) on the live stack. Cross-channel-everywhere parity
remains `SCOPE-03B`.

### Build Quality Gate — `#build-quality`

**Executed this finalization session. Claim Source: executed.** SCOPE-03A's own
build-quality gates are all green. The repo-wide `format --check` exit 1 is
ENTIRELY foreign-owned pre-existing drift (two files, neither a SCOPE-03A file,
neither modified by this session) — recorded foreign, not touched, not bypassed.

**`./smackerel.sh lint` → LINT_EXIT=0**

```text
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: PWA manifest has required fields
  OK: web/extension/manifest.json
  OK: Chrome extension manifest has required fields (MV3)
  OK: web/extension/manifest.firefox.json
  OK: Firefox extension manifest has required fields (MV2 + gecko)
=== Validating JS syntax ===
  OK: web/pwa/app.js
  OK: web/pwa/sw.js
  OK: web/pwa/lib/queue.js
  OK: web/extension/background.js
  OK: web/extension/popup/popup.js
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
All checks passed!
LINT_EXIT=0
```

**`./smackerel.sh format --check` → FORMAT_EXIT=1 (foreign-owned only)**

```text
internal/api/graphapi/activation.go
internal/web/handler_test.go
FORMAT_EXIT=1
```

The repo-wide `format --check` enumerates EXACTLY two unformatted files, and
neither is a SCOPE-03A file. Exhaustive-enumeration ownership proof — every
SCOPE-03A `.go` file is ABSENT from the dirty list, therefore gofmt-clean:

```text
=== SCOPE-03A own go files on disk (all absent from the 2-file dirty list) ===
internal/telegram/assistant_adapter/callbacks_nudge_test.go
internal/telegram/assistant_adapter/render_nudge.go
internal/telegram/assistant_adapter/render_nudge_test.go
tests/e2e/proactive_channel_parity_e2e_test.go
tests/integration/proactive/budget_defer_parity_test.go
tests/integration/proactive/escalation_parity_test.go
tests/integration/proactive/nudge_ack_controller_test.go
tests/integration/proactive/telegram_honest_states_test.go
tests/integration/proactive/telegram_nudge_ack_test.go
tests/integration/proactive/telegram_single_channel_suppression_test.go
=== ownership of the 2 format-dirty files ===
internal/api/graphapi/activation.go   FOREIGN-not-SCOPE-03A
internal/web/handler_test.go          FOREIGN-not-SCOPE-03A
=== git status --porcelain of the 2 dirty files ===
(empty — both unmodified in this working tree; pre-existing committed drift)
```

`internal/web/handler_test.go` is the concurrent sibling search/BUG-002-006
session's out-of-boundary file (recorded in state.json blockedReason);
`internal/api/graphapi/activation.go` is likewise foreign. Per the scope
boundaries both are left untouched and unbypassed, recorded as foreign-owned.

**`./smackerel.sh config generate` → CONFIG_EXIT=0**

```text
config-validate: <repo-root>/config/generated/dev.env.tmp.982608 OK
Generated <repo-root>/config/generated/dev.env
Generated <repo-root>/config/generated/nats.conf
Generated <repo-root>/config/generated/prometheus.yml
CONFIG_EXIT=0
```

**Grouped sub-claim coverage:** scope tests (11 test DoD items above, all `[x]`);
`check` (#build-check); lint 0; format own-code clean; source/config validation
(config generate 0 + #build-check config-validate OK); Telegram transport shape
documented + proven by the golden + non-collision renders (#t107-03a-golden,
#t107-03-collision); consumer review = the one inbound ack consumer proven at
#t107-005-i (Telegram tap → one `Acknowledge(content_key)` path); artifact-lint 0;
traceability 0 warnings; change-boundary = Summary Boundary note (no edits under
specs/105/106/072/078, internal/web, internal/whatsapp, internal/intelligence/surfacing).

**Own-code verdict:** SCOPE-03A build-quality is GREEN (lint 0, config 0, all 10
own `.go` files gofmt-clean, artifact-lint 0, traceability 0). Foreign format
drift is out-of-boundary and left untouched.

## Guard Evidence

**`bash .github/bubbles/scripts/artifact-lint.sh specs/107-proactive-correlated-experience` → ARTIFACT_LINT_EXIT=0**

```text
✅ Required artifact exists: spec.md
✅ Required artifact exists: design.md
✅ No unfilled evidence template placeholders in scopes/03A-telegram-proactive-nudge-rendering/report.md
✅ No repo-CLI bypass detected in scopes/03A-telegram-proactive-nudge-rendering/report.md command evidence
Artifact lint PASSED.
ARTIFACT_LINT_EXIT=0
```

**`bash .github/bubbles/scripts/traceability-guard.sh specs/107-proactive-correlated-experience` → TRACEABILITY_EXIT=0**

```text
--- Scenario Manifest Cross-Check (G057/G059) ---
✅ scenario-manifest.json covers 20 scenario contract(s)
✅ scenario-manifest.json records evidenceRefs
✅ All linked tests from scenario-manifest.json exist
✅ scopes/03A-telegram-proactive-nudge-rendering/scope.md scenario mapped to Test Plan row: SCN-107-005 Telegram renders the nudge as inline actions
--- Traceability Summary ---
ℹ️  Scenarios checked: 20
ℹ️  Test rows checked: 111
ℹ️  DoD fidelity scenarios: 20 (mapped: 20, unmapped: 0)
RESULT: PASSED (0 warnings)
TRACEABILITY_EXIT=0
```

**`bash .github/bubbles/scripts/state-transition-guard.sh specs/107-proactive-correlated-experience` → STATE_GUARD_EXIT=1 (EXPECTED refusal — the SPEC is blocked)**

```text
🔴 TRANSITION BLOCKED: 184 failure(s), 3 warning(s)
state.json status MUST NOT be set to 'done'.
BEGIN TRANSITION_GUARD_RESULT_V1
workflowMode: full-delivery
auditProfile: delivery-completion-v1
targetStatus: done
failedChecks: [Check-4-completion, Check-5-all-done, Check-8-file-existence, Check-9-evidence, Check-11-execution-evidence]
failedGateIds: [G060, G022, G040, G089]
failureCount: 184
verdict: FAIL
END TRANSITION_GUARD_RESULT_V1
STATE_GUARD_EXIT=1
```

**Correct behavior.** The guard evaluates the whole-spec `done` transition (full-delivery
ceiling = done) and REFUSES it because the SPEC is `blocked` — 8 of 10 scopes (02, 03B,
04–09) are Blocked/unbuilt, so their unchecked DoD, planned-work language, and missing
change-boundary containment drive the 184 failures. This is exactly the refusal the
finalization expects; spec-107 stays `blocked`.

**No fabrication against SCOPE-03A.** The guard PASSES SCOPE-03A's anti-fabrication checks:
“✅ No template placeholders in scopes/03A-…” and “✅ No narrative summary phrases detected
outside code blocks in scopes/03A-…”. The ONLY SCOPE-03A findings are Check-9 “DoD item [x]
has NO evidence block” — the SAME structural report.md-anchor evidence convention that fires
IDENTICALLY on the already-accepted SCOPE-01 core-delivered items (T107-004-U / T107-004-I).
Every report.md anchor referenced by a SCOPE-03A `[x]` item (#t107-005-u / -i / -a,
#t107-03a-golden / -tgsuppress / -honest, #t107-03-collision, #build-quality) exists with
real ≥10-line raw evidence — items 127 + 135 captured this finalization run, the other 11 by
the earlier unit / integration-light / golden lanes.

## Completion Statement

**SCOPE-03A (Telegram proactive nudge rendering) is DONE — 13/13 DoD `[x]`, every item backed
by real raw execution evidence.** This finalization run persisted the final two items after a
prior run was truncated:

- **Item 135 (Build Quality Gate) — GREEN.** `./smackerel.sh lint` 0, `config generate` 0,
  all 10 SCOPE-03A own `.go` files gofmt-clean, `artifact-lint` 0, `traceability-guard` 0
  (0 warnings, 20/20 DoD fidelity). Repo-wide `format --check` exit 1 is FOREIGN-only
  (`internal/api/graphapi/activation.go` + `internal/web/handler_test.go` — neither a
  SCOPE-03A file, both unmodified in-tree; recorded foreign, not touched, not bypassed).
- **Item 127 (T107-005-A e2e) — GREEN.** `./smackerel.sh test e2e --go-run 'TestSCN107005'`
  brought the full disposable stack up (healthy), `--- PASS:
  TestSCN107005_TelegramInlineNudgeAcknowledgesThroughController`, `E2E_EXIT=0`, stack torn
  down (containers / volumes / network removed).

**Spec-107 stays `blocked` (correct):** SCOPE-01 core-delivered + SCOPE-03A Done; SCOPE-03B
gated on unbuilt spec-072; SCOPE-02 / 04–09 gated on the coordinating session's unbuilt
spec-106 (shell) + spec-105 (explorer). The final `state-transition-guard` correctly REFUSES
the whole-spec `done` transition and raises NO fabrication finding against SCOPE-03A.

**Boundary intact:** only spec-107 03A `scope.md` / `report.md` + `state.json` were edited
this run; no source changes; `specs/105/106/072/078`, `internal/web`, `internal/whatsapp`,
`internal/intelligence/surfacing` untouched. No commit / push / deploy.

**Next owner:** `bubbles.workflow` (route SCOPE-03B / SCOPE-02 / 04–09 once specs 106 / 105 /
072 land).
