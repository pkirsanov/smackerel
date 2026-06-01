# Report: 075 Legacy-Surface Deprecation Telemetry & User Comms

## Summary

Planning-only scaffold created by `bubbles.plan` for legacy retirement telemetry and user communications. The packet defines five sequential scopes, a scenario manifest, and a structured test plan covering SCN-075-A01 through SCN-075-A11.

## Scope Inventory

| Scope | Name | Status |
|---|---|---|
| SCOPE-075-01 | Retirement Safety Foundation, Config, And Privacy | Not Started |
| SCOPE-075-02 | Open-Window Notice Dedup And Intent Serving | Not Started |
| SCOPE-075-03 | Residual Usage Telemetry And Dashboard | Not Started |
| SCOPE-075-04 | Automatic Pause And Resume | Not Started |
| SCOPE-075-05 | Closed-Window Response And Observation Gate | Not Started |
| SCOPE-075-06 | Facade Policy Dispatch Rollout And Telegram Coexistence (5 sub-scopes: 6.1 facade contract, 6.2 construction wiring, 6.3 PWA renderer + live Playwright, 6.4 WhatsApp/Mobile renderers + Telegram short-circuit, 6.5 live-stack TP execution) | Not Started |

## Test Evidence

No implementation, build, lint, runtime, UI, report, or test evidence is recorded in this planning scaffold. Evidence must be added only after commands execute in the current session and raw output is available.

### SCOPE-075-06.1 (Facade Policy Dispatch Contract) — implement phase

**Phase:** implement  
**Claim Source:** executed

TP-075-19 (facade-level 5-branch unit test + nil-Policy containment):

```text
$ cd ~/smackerel && go test -count=1 -timeout 90s -run TestFacadeLegacyRetirement ./internal/assistant/
ok      github.com/smackerel/smackerel/internal/assistant       0.494s
EXIT=0
```

Regression sweep (no transport changes; verifies no collateral break in assistant + telegram packages, which contain the only existing `legacyretirement.Policy` consumer + the existing `FacadeConfig`/`AssistantResponse` consumers):

```text
$ cd ~/smackerel && go test -count=1 -timeout 180s ./internal/assistant/... ./internal/telegram/...
ok  github.com/smackerel/smackerel/internal/assistant                          0.866s
ok  github.com/smackerel/smackerel/internal/assistant/legacyretirement         0.122s
ok  github.com/smackerel/smackerel/internal/telegram                          28.370s
ok  github.com/smackerel/smackerel/internal/telegram/assistant_adapter         0.071s
ok  github.com/smackerel/smackerel/internal/telegram/render                    0.082s
(plus 22 additional assistant subpackages all `ok`)
```

Files changed:

- `internal/assistant/contracts/legacy_retirement_notice.go` (new) — `NoticePayload{Command, ReplacementExample, CopyKey, WindowID}` per scopes.md §"New Types & Signatures".
- `internal/assistant/contracts/response.go` — `AssistantResponse.LegacyRetirementNotice *NoticePayload` (omitempty-equivalent: nil pointer).
- `internal/assistant/facade.go` — `FacadeConfig.Policy legacyretirement.Policy` (nil-safe) and Step 1.6 pre-routing dispatch handling all five SCN-075-A12 branches.
- `internal/assistant/legacyretirement/policy.go`, `policyimpl.go` — additive `RetirementDecision.WindowID` so the facade can populate the payload without depending on the concrete `policyImpl`.
- `internal/assistant/facade_legacy_retirement_dispatch_test.go` (new) — TP-075-19, 6 tests (5 branches + nil-Policy).

Sub-scopes 6.3–6.5 remain Not Started.

### SCOPE-075-06.2b (Wire-Schema Notice Propagation) — implement phase

**Phase:** implement
**Claim Source:** executed

TP-075-25 — golden contract round-trips notice-present + notice-absent at `schema_version="v1"`:

```text
$ cd ~/smackerel && go test -count=1 -timeout 90s ./internal/assistant/schema/... ./internal/assistant/httpadapter/...
ok      github.com/smackerel/smackerel/internal/assistant/schema                0.008s
?       github.com/smackerel/smackerel/internal/assistant/schema/webcodegen     [no test files]
ok      github.com/smackerel/smackerel/internal/assistant/httpadapter           0.041s
EXIT=0
```

TP-075-26 — regenerated PWA bindings + codegen-drift test green:

```text
$ cd ~/smackerel && go run ./cmd/web-assistant-codegen
wrote web/pwa/generated/assistant_turn_v1.d.ts
wrote web/pwa/generated/assistant_turn_v1.js
$ cd ~/smackerel && go test -count=1 -timeout 60s ./web/pwa/tests/
ok      github.com/smackerel/smackerel/web/pwa/tests    0.013s
```

Typed optional `notice` surface in regenerated PWA TypeScript:

```text
$ grep -nE 'Notice|notice' web/pwa/generated/assistant_turn_v1.d.ts
45:  notice?: NoticePayload;
50:export interface NoticePayload {
56:export function validateNoticePayload(obj: unknown): NoticePayload;
```

TP-075-27 — regenerated Flutter shared-core bindings + codegen-drift test green:

```text
$ cd ~/smackerel/clients/mobile/assistant && dart run tool/gen_dart_models.dart
wrote ~/smackerel/clients/mobile/assistant/lib/core/generated/assistant_turn_v1.dart (12294 bytes)
$ cd ~/smackerel/clients/mobile/assistant && flutter test test/codegen_drift_test.dart
00:09 +3: All tests passed!
EXIT=0
```

Regression sweep (assistant + PWA packages, including the relaxed-helper schema golden test and every other consumer of the v1 wire types):

```text
$ cd ~/smackerel && go test -count=1 -timeout 180s ./internal/assistant/... ./web/pwa/tests/
ok  github.com/smackerel/smackerel/internal/assistant                          0.599s
ok  github.com/smackerel/smackerel/internal/assistant/httpadapter              0.149s
ok  github.com/smackerel/smackerel/internal/assistant/legacyretirement         0.018s
ok  github.com/smackerel/smackerel/internal/assistant/schema                   0.017s
ok  github.com/smackerel/smackerel/web/pwa/tests                               0.012s
(… 19 other assistant subpackages all `ok` …)
EXIT=0
```

Files changed in this sub-scope:

- `internal/assistant/schema/assistant_turn_v1.json` — optional `notice` property + `NoticePayload` sub-def; docstring policy update (additive optional → no schema_version bump).
- `internal/assistant/schema/types.go` — `TurnResponse.Notice *NoticePayload \`json:"notice,omitempty"\`` and `NoticePayload` struct.
- `internal/assistant/schema/golden_contract_test.go` — relaxed helpers (properties ⊇ required + omitempty enforcement); new `NoticePayload_pins_Go_type` and `response_v1_notice_fixture_round_trip` subtests; adversarial drift checks still fire.
- `internal/assistant/schema/testdata/response_v1_notice.json` (new) — notice-present golden.
- `internal/assistant/schema/webcodegen/generator.go` — `NoticePayload` added to `definitionOrder`; optional-field iteration in JS validator + DTS (`field?: Type` with `hasOwnProperty && != null` guard).
- `internal/assistant/httpadapter/schema.go` — `TurnResponse.Notice *NoticeJSON \`json:"notice,omitempty"\`` + `NoticeJSON` struct.
- `internal/assistant/httpadapter/adapter.go` — `RenderJSON` copies `AssistantResponse.LegacyRetirementNotice` → `TurnResponse.Notice` nil-safely.
- `internal/assistant/httpadapter/golden_contract_test.go` — `response_v1_notice` subtest pins TP-075-25; notice-absent subtest asserts the wire body omits `notice` entirely.
- `internal/assistant/httpadapter/testdata/response_v1_notice.json` (new) — notice-present httpadapter golden.
- `web/pwa/generated/assistant_turn_v1.{js,d.ts}` — regenerated.
- `clients/mobile/assistant/lib/src/codegen.dart` — `NoticePayload` added to `definitionOrder`; optional-field iteration with `containsKey && != null` guard.
- `clients/mobile/assistant/lib/core/generated/assistant_turn_v1.dart` — regenerated.

Containment: no renderer code (PWA, WhatsApp, Mobile, Telegram), no `schema_version` change, no other `TurnResponse` field touched. Renderer work owned by 6.3/6.4 will consume this typed optional `notice` surface.

Outstanding: `design.md` v1-compatibility decision record for SCOPE-075-06.2b is owned by `bubbles.design` — not edited by this agent. Routed via the result envelope.

Sub-scopes 6.3–6.5 remain Not Started.

## Completion Statement

Status remains `in_progress` with `planMaturityOnly=true`. No terminal status or scope completion is claimed by this planning pass.

## Uncertainty Declarations

- Scope DoD items are unchecked because implementation and validation have not executed for this feature.
- Planned test file paths and titles are handoff targets for implementation/test agents; they are not claims that tests already exist.

## Validation Summary

Artifact lint is captured in the invoking agent result envelope for this planning pass.