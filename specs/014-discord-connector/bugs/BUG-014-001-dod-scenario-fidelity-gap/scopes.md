# Scopes: BUG-014-001 — DoD scenario fidelity gap

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Restore Gherkin → DoD trace-ID fidelity for spec 014

**Status:** Done
**Priority:** P0
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-DC-FIX-001 Trace guard accepts SCN-DC-NRM/REST/CONN/GW/THR/CMD as faithfully covered
  Given specs/014-discord-connector/scopes.md DoD entries that name each Gherkin scenario by ID
  And specs/014-discord-connector/scenario-manifest.json mapping all 13 SCN-DC-* scenarios
  When the workflow runs `bash .github/bubbles/scripts/traceability-guard.sh specs/014-discord-connector`
  Then Gate G068 reports "13 scenarios checked, 13 mapped to DoD, 0 unmapped"
  And the overall result is PASSED
```

### Implementation Plan

1. Append `Scenario SCN-DC-NRM-001` and `Scenario SCN-DC-NRM-002` DoD bullets to Scope 01 DoD in `specs/014-discord-connector/scopes.md`, citing `TestClassifyMessage`/`TestNormalizeMessage`/`TestAssignTier` and source `discord.go::classifyMessage`/`assignTier`.
2. Append `Scenario SCN-DC-REST-001` and `Scenario SCN-DC-REST-002` DoD bullets to Scope 02 DoD, citing `TestFetchChannelMessages_*` and `TestSyncEndToEnd_*`, with source pointer to `discord.go::fetchChannelMessages`/`Sync`/`ChannelCursors`.
3. Append `Scenario SCN-DC-CONN-001` DoD bullet to Scope 03 DoD, citing `TestConnect_*`, `TestSync_HealthTransitionsDuringSyncLifecycle`, `TestClose`.
4. Append `Scenario SCN-DC-GW-001` DoD bullet to Scope 04 DoD, citing `TestEventPoller_*` and source `gateway.go::EventPoller`/`discord.go::Sync (drainGatewayEvents)`.
5. Append `Scenario SCN-DC-THR-004` DoD bullet to Scope 05 DoD, citing `TestSync_IncludeThreadsFalse_SkipsThreads` and source `discord.go::Sync` (IncludeThreads gate).
6. Append `Scenario SCN-DC-CMD-003` DoD bullet to Scope 06 DoD, citing `TestParseBotCommand_SSRFProtection`/`TestIsSafeURL` and source `discord.go::ParseBotCommand`/`isSafeURL`.
7. Run `bash .github/bubbles/scripts/artifact-lint.sh` against both the parent and bug folder; run `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/014-discord-connector` and confirm PASS.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-FIX-1-01 | traceability-guard.sh PASS | artifact | `.github/bubbles/scripts/traceability-guard.sh` | `RESULT: PASSED (0 warnings)` and `DoD fidelity: 13 mapped, 0 unmapped` | SCN-DC-FIX-001 |
| T-FIX-1-02 | artifact-lint.sh PASS (parent) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/014-discord-connector` | SCN-DC-FIX-001 |
| T-FIX-1-03 | artifact-lint.sh PASS (bug) | artifact | `.github/bubbles/scripts/artifact-lint.sh` | exit 0 against `specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap` | SCN-DC-FIX-001 |
| T-FIX-1-04 | Underlying behavior tests still pass | unit | `internal/connector/discord/discord_test.go`, `gateway_test.go` | `go test -count=1 -v ./internal/connector/discord/...` exit 0; the named tests for the 8 SCN-DC-* scenarios all PASS | SCN-DC-FIX-001 |

### Definition of Done

- [x] Scope 01 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-DC-NRM-001` and `Scenario SCN-DC-NRM-002` with restated scenario name and test/source pointers — **Phase:** implement
  > Evidence:
  > ```
  > $ grep -n "Scenario SCN-DC-NRM-001\|Scenario SCN-DC-NRM-002" specs/014-discord-connector/scopes.md
  > ```
  > Both bullets appear in the Scope 01 DoD section verbatim and are checked `[x]`.
- [x] Scope 02 DoD in parent `scopes.md` contains bullets citing `Scenario SCN-DC-REST-001` and `Scenario SCN-DC-REST-002` — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-DC-REST-001\|Scenario SCN-DC-REST-002" specs/014-discord-connector/scopes.md` returns two matches inside Scope 02 DoD section.
- [x] Scope 03 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-DC-CONN-001` — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-DC-CONN-001" specs/014-discord-connector/scopes.md` returns one match inside Scope 03 DoD section.
- [x] Scope 04 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-DC-GW-001` — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-DC-GW-001" specs/014-discord-connector/scopes.md` returns one match inside Scope 04 DoD section.
- [x] Scope 05 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-DC-THR-004` — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-DC-THR-004" specs/014-discord-connector/scopes.md` returns one match inside Scope 05 DoD section.
- [x] Scope 06 DoD in parent `scopes.md` contains a bullet citing `Scenario SCN-DC-CMD-003` — **Phase:** implement
  > Evidence: `grep -n "Scenario SCN-DC-CMD-003" specs/014-discord-connector/scopes.md` returns one match inside Scope 06 DoD section.
- [x] Underlying behavior tests still pass — **Phase:** test
  > Evidence:
  > ```
  > $ go test -count=1 -v -run 'TestClassifyMessage$|TestAssignTier$|TestFetchChannelMessages_Basic$|TestSyncEndToEnd_CursorPreventsRefetch$|TestConnect_TokenValidationSuccess$|TestEventPoller_ConnectStartsPolling$|TestSync_IncludeThreadsFalse_SkipsThreads$|TestIsSafeURL$|TestParseBotCommand_SSRFProtection$' ./internal/connector/discord/
  > === RUN   TestIsSafeURL
  > --- PASS: TestIsSafeURL (0.00s)
  > === RUN   TestParseBotCommand_SSRFProtection
  > --- PASS: TestParseBotCommand_SSRFProtection (0.00s)
  > === RUN   TestClassifyMessage
  > --- PASS: TestClassifyMessage (0.00s)
  > === RUN   TestAssignTier
  > --- PASS: TestAssignTier (0.00s)
  > === RUN   TestConnect_TokenValidationSuccess
  > --- PASS: TestConnect_TokenValidationSuccess (0.03s)
  > === RUN   TestFetchChannelMessages_Basic
  > --- PASS: TestFetchChannelMessages_Basic (0.03s)
  > === RUN   TestSyncEndToEnd_CursorPreventsRefetch
  > --- PASS: TestSyncEndToEnd_CursorPreventsRefetch (0.02s)
  > === RUN   TestSync_IncludeThreadsFalse_SkipsThreads
  > --- PASS: TestSync_IncludeThreadsFalse_SkipsThreads (0.01s)
  > === RUN   TestEventPoller_ConnectStartsPolling
  > --- PASS: TestEventPoller_ConnectStartsPolling (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       0.130s
  > ```
- [x] Traceability-guard PASSES against `specs/014-discord-connector` — **Phase:** validate
  > Evidence: see report.md `### Validation Evidence` for the full guard output. Final lines:
  > ```
  > ℹ️  DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped
  > ℹ️  Concrete test file references: 13
  > ℹ️  Report evidence references: 13
  > RESULT: PASSED (0 warnings)
  > ```
- [x] Artifact-lint PASSES against parent and bug folder — **Phase:** validate
  > Evidence: see report.md `### Audit Evidence` for both runs.
- [x] No production code changed (boundary preserved) — **Phase:** audit
  > Evidence: `git diff --name-only` (post-fix) shows changes confined to `specs/014-discord-connector/scopes.md` and `specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/*`. No files under `internal/`, `cmd/`, `ml/`, `config/` are touched.
