# Report: 073 Web/Mobile Assistant Frontend Client

## Summary

Planning-only scaffold reconciled by `bubbles.plan` for the web client and one shared mobile assistant codebase that targets iPhone/iOS and Android. The packet defines four sequential scopes, a scenario manifest, and a structured test plan covering SCN-073-A01 through SCN-073-A11.

## Scope Inventory

| Scope | Name | Status |
|---|---|---|
| SCOPE-073-01 | Shared Schema, Mobile Foundation, Auth, And Fail-Loud Config | Not Started |
| SCOPE-073-02 | Web Chat Vertical Slice | Not Started |
| SCOPE-073-03 | Shared Mobile Chat Vertical Slice | Not Started |
| SCOPE-073-04 | Cross-Surface Response Controls, Capture, And Parity | Not Started |

## Test Evidence

### SCOPE-073-01 — Gap-fill (`bubbles.implement`, 2026-06-01)

**Phase:** implement
**Agent:** bubbles.implement
**Claim Source:** executed

#### TP-073-03 — Cross-language renderer canary (SCN-073-A02)

Implemented under the ratified Render-Descriptor JSON Schema declared in
`design.md` § *Shared Schema And Renderer Core*. Artifacts:

- Canonical schema: [`tests/fixtures/assistant_response_v1/render-descriptor-v1.json`](../../tests/fixtures/assistant_response_v1/render-descriptor-v1.json)
- Seven input + golden descriptor pairs under
  [`tests/fixtures/assistant_response_v1/`](../../tests/fixtures/assistant_response_v1/):
  `text_only`, `with_sources`, `disambiguation`, `confirm_accept_decline`,
  `capture_acknowledgement`, `error_retry`, `unknown_shape`.
- JS renderer (web surface): [`web/pwa/lib/render_descriptor_v1.js`](../../web/pwa/lib/render_descriptor_v1.js) +
  CLI [`web/pwa/lib/render_descriptor_v1_cli.js`](../../web/pwa/lib/render_descriptor_v1_cli.js).
- Dart renderer (shared mobile core for iOS + Android adapter projections):
  [`clients/mobile/assistant/lib/core/render_descriptor_v1.dart`](../../clients/mobile/assistant/lib/core/render_descriptor_v1.dart) +
  CLI [`clients/mobile/assistant/tool/render_descriptor_v1_cli.dart`](../../clients/mobile/assistant/tool/render_descriptor_v1_cli.dart).
- Go canary test: [`tests/unit/clients/render_descriptor_canary_test.go`](../../tests/unit/clients/render_descriptor_canary_test.go).

Command:
```text
$ go test -count=1 -timeout 180s -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/
ok      github.com/smackerel/smackerel/tests/unit/clients       9.635s
```

The canary spawns `node` and `dart` to invoke both CLIs against every
fixture, validates each output against the closed render-descriptor-v1
vocabulary, and asserts `js == golden`, `dart == golden`, and
`js == dart` for all seven scenarios.

#### TP-073-07 — Web and mobile client config fail-loud tests (SCN-073-A11)

Existing implementation at [`internal/config/assistant_frontend.go`](../../internal/config/assistant_frontend.go)
plus tests at [`internal/config/assistant_frontend_validators_test.go`](../../internal/config/assistant_frontend_validators_test.go)
were re-executed under this session and pass:

```text
$ go test -count=1 -timeout 60s -run 'WebAssistant|MobileAssistant|AssistantFrontend' ./internal/config/
ok      github.com/smackerel/smackerel/internal/config  0.022s
```

The tests assert `[F073-SST-MISSING]` for every empty required key
(`web.assistant.{enabled,backend_base_url,schema_version}`,
`mobile.assistant.{enabled,backend_base_url,schema_version,platforms,auth_mode}`)
and `[F073-SST-INVALID]` for schema-version drift, non-`same-origin`
non-https web base URL, and platforms missing `ios` or `android`.

### SCOPE-073-01 — Scope-1 row verification (`bubbles.implement`, 2026-06-01)

**Phase:** implement
**Agent:** bubbles.implement
**Claim Source:** executed

Inventory of pre-authored Scope-1 test artifacts re-verified under this
current session. Each test below was authored in prior planning/implementation
passes; this pass executes them against the current SHA to capture
current-session evidence.

#### Build fix prerequisite — `tests/unit/clients/render_descriptor_canary_test.go`

The file had a duplicate `package clients_test` declaration (line 1 and
line 26) inherited from a prior edit; `go test` failed with
`expected declaration, found 'package'`. Removed the leading duplicate
package line. Diff is the single-line removal of `package clients_test`
before the file's leading doc comment.

#### TP-073-01 — Dart wire-schema codegen drift (SCN-073-A02)

Test: [`clients/mobile/assistant/test/codegen_drift_test.dart`](../../clients/mobile/assistant/test/codegen_drift_test.dart)

```text
$ cd clients/mobile/assistant && flutter test test/codegen_drift_test.dart \
                                               test/platform_declaration_test.dart \
                                               test/core_storage_guard_test.dart
00:13 +10: All tests passed!
```

Combined run executed `TP-073-01` (2 sub-tests: committed artifact
matches regeneration, regeneration is deterministic), `TP-073-04`
platform declaration (3 sub-tests), and `TP-073-26` mobile core storage
guard (3 sub-tests, including adversarial). All ten passed.

#### TP-073-02 — Web generated assistant schema drift (SCN-073-A02)

Test: [`web/pwa/tests/assistant_codegen_drift_test.go`](../../web/pwa/tests/assistant_codegen_drift_test.go)
+ adversarial sibling.

```text
$ go test -count=1 -timeout 90s -run 'TestWebAssistantCodegen|TestWebAssistantStorageGuard' ./web/pwa/tests/
ok      github.com/smackerel/smackerel/web/pwa/tests    0.031s
RC=0
```

Covers `TestWebAssistantCodegen_NoDrift_TP_073_02`,
`TestWebAssistantCodegen_Adversarial_TP_073_02`,
`TestWebAssistantStorageGuard_TP_073_06`,
`TestWebAssistantStorageGuard_Adversarial_TP_073_06`.

#### TP-073-03 — Cross-language renderer canary (SCN-073-A02)

```text
$ go test -count=1 -timeout 300s -run TestRenderDescriptorV1_CrossLanguageCanary ./tests/unit/clients/
ok      github.com/smackerel/smackerel/tests/unit/clients       8.319s
```

Dart-side renderer canary independently:

```text
$ cd clients/mobile/assistant && flutter test test/renderer_canary_test.dart
00:05 +2: All tests passed!
```

#### TP-073-04 — Shared mobile platform declaration (SCN-073-A02)

Covered by the combined Flutter test run above
(`test/platform_declaration_test.dart` produced 3 passing sub-tests in
the +10 summary).

#### TP-073-06 — Web + shared mobile sensitive storage guard (SCN-073-A11)

Web half covered by `TestWebAssistantStorageGuard_*` runs above.
Mobile half (`TP-073-26` in the Flutter run) covered by
`test/core_storage_guard_test.dart` (3 passing sub-tests including
adversarial).

#### TP-073-05 — Live transport-hint integration (SCN-073-A08)

Test: [`tests/integration/api/assistant_transport_hint_test.go`](../../tests/integration/api/assistant_transport_hint_test.go)

Status under this session: **queued, pending** —
`./smackerel.sh test integration --go-run "^TestAssistantTransportHint_"` is
serialized behind another integration run holding
`/tmp/smackerel-1000-test-test-suite.lock` and had not produced
`/tmp/s073-tp05.done` by the close of this implement pass. Re-invoke
this command for current-session evidence before marking TP-073-05.

#### TP-073-08 — Live transport-hint parity e2e (SCN-073-A08)

Test: [`tests/e2e/assistant/transport_hint_parity_test.go`](../../tests/e2e/assistant/transport_hint_parity_test.go)

Status under this session: **not run** — depends on the test-suite
lock being released; should be executed via
`./smackerel.sh test e2e --go-run "^TestAssistantTransportHintParity_"`
after TP-073-05.

### Remaining Scope-1 work

TP-073-05 and TP-073-08 are the only Scope-1 rows without current-session
execution evidence. SCOPE-073-01 DoD remains unchecked until both
queued live-stack rows pass.

### SCOPE-073-02 — Web Chat Vertical Slice

**Not started under this pass.** Authoring `web/pwa/assistant.html`,
`web/pwa/assistant.js`, and the three new live e2e tests
(`tests/e2e/assistant/web_pwa_{chat,retry,accessibility}_e2e_test.go`)
is queued for a follow-up implement invocation. No source files or
tests for Scope 2 were modified or added in this pass.

## Completion Statement

Status remains `in_progress` with `planMaturityOnly=true`. No terminal status or scope completion is claimed by this planning pass.

## Uncertainty Declarations

- Scope DoD items are unchecked because implementation and validation have not executed for this feature.
- Planned test file paths and titles are handoff targets for implementation/test agents; they are not claims that tests already exist.

## Validation Summary

Artifact lint is captured in the invoking agent result envelope for this planning pass.