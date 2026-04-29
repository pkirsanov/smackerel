# Bug: BUG-014-001 — DoD scenario fidelity gap (SCN-DC-NRM/REST/CONN/GW/THR/CMD)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** MEDIUM (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 014 — Discord Connector
- **Workflow Mode:** bugfix-fastlane
- **Status Ceiling:** done
- **Status:** Fixed (artifact-only)

## Problem Statement

Bubbles traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 8 of the 13 Gherkin scenarios in the parent feature's `scopes.md` had no faithful matching DoD item:

- `SCN-DC-NRM-001` Classify all message content types
- `SCN-DC-NRM-002` Assign processing tiers per R-007
- `SCN-DC-REST-001` Fetch message history with pagination
- `SCN-DC-REST-002` Per-channel cursor advancement
- `SCN-DC-CONN-001` Connector lifecycle
- `SCN-DC-GW-001` Real-time message capture
- `SCN-DC-THR-004` Thread ingestion disabled via config
- `SCN-DC-CMD-003` Capture command with unsafe URL rejected

The gate's content-fidelity matcher requires a DoD bullet to either (a) carry the same `SCN-DC-NNN-NNN` trace ID as the Gherkin scenario, or (b) share enough significant words. The pre-existing DoD entries described the implemented behavior but did not embed the trace ID, and the fuzzy matcher's significant-word threshold was not satisfied for these eight scenarios.

## Reproduction (Pre-fix)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/014-discord-connector 2>&1 | tail -12
❌ Scope 01: Normalizer & Message Classification Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-NRM-001 Classify all message content types
❌ Scope 01: Normalizer & Message Classification Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-NRM-002 Assign processing tiers per R-007
❌ Scope 02: REST Client & Backfill Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-REST-001 Fetch message history with pagination
❌ Scope 02: REST Client & Backfill Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-REST-002 Per-channel cursor advancement
❌ Scope 03: Discord Connector & Config Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-CONN-001 Connector lifecycle
❌ Scope 04: Gateway Event Handler Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-GW-001 Real-time message capture
❌ Scope 05: Thread Ingestion Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-THR-004 Thread ingestion disabled via config
❌ Scope 06: Bot Command Capture Gherkin scenario has no faithful DoD item preserving its behavioral claim: SCN-DC-CMD-003 Capture command with unsafe URL rejected
ℹ️  DoD fidelity: 13 scenarios checked, 5 mapped to DoD, 8 unmapped
❌ DoD content fidelity gap: 8 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)
RESULT: FAILED (9 failures, 0 warnings)
```

## Gap Analysis (per scenario)

For each missing scenario the bug investigator searched the production code (`internal/connector/discord/discord.go`, `gateway.go`) and the test files (`discord_test.go`, `gateway_test.go`). All eight behaviors are genuinely **delivered-but-undocumented at the trace-ID level** — there is no missing implementation and no missing test fixture; the only gap is that DoD bullets did not embed the `SCN-DC-NNN-NNN` ID that the guard uses for fidelity matching.

| Scenario | Behavior delivered? | Tests pass? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-DC-NRM-001 | Yes — `classifyMessage()` returns one of 8 content types based on attachments/embeds/URLs/code-fences/reply/thread/capture-prefix detection | Yes — `TestClassifyMessage` (10 cases), `TestNormalizeMessage` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::classifyMessage`, `normalizeMessage` |
| SCN-DC-NRM-002 | Yes — `assignTier()` follows R-007: pinned/high-reactions/links→full, attachments/code/replies/embeds→standard, short(<20 chars)→metadata, default→light | Yes — `TestAssignTier` (10 cases) PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::assignTier` |
| SCN-DC-REST-001 | Yes — `fetchChannelMessages()` paginates `GET /channels/{id}/messages?limit=100&after={cursor}` until backfill_limit reached, ordered ascending | Yes — `TestFetchChannelMessages_Basic`, `_Pagination`, `_RespectsBackfillLimit` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::fetchChannelMessages` |
| SCN-DC-REST-002 | Yes — `Sync()` advances the per-channel cursor in `ChannelCursors` to the highest snowflake seen; subsequent fetch uses `after={cursor}` and skips already-captured messages | Yes — `TestSyncEndToEnd_CursorPreventsRefetch`, `TestSyncEndToEnd_WithMessagesAndPins` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::Sync`, `ChannelCursors` |
| SCN-DC-CONN-001 | Yes — `Connect()` validates bot token via `GET /users/@me` (200→healthy, 401→unauthorized), `Sync()` fetches monitored channels with cursor JSON, `Close()` transitions health to disconnected | Yes — `TestConnect_ValidConfig`, `TestConnect_TokenValidationSuccess`, `TestConnect_TokenValidationUnauthorized`, `TestSync_HealthTransitionsDuringSyncLifecycle`, `TestClose` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::Connect`, `Sync`, `Close` |
| SCN-DC-GW-001 | Yes — `EventPoller` connects via Gateway with intents (GUILDS \| GUILD_MESSAGES \| MESSAGE_CONTENT), buffers MESSAGE_CREATE events on the events channel, `Sync()` calls `drainGatewayEvents` before REST fetch | Yes — `TestEventPoller_ConnectStartsPolling`, `TestEventPoller_SyncDrainsBufferedEvents`, `TestEventPoller_EventsFilterToMonitoredChannels` PASS | `internal/connector/discord/gateway_test.go` | `internal/connector/discord/gateway.go::EventPoller`, `discord.go::Sync` (drainGatewayEvents) |
| SCN-DC-THR-004 | Yes — `Sync()` gates `fetchActiveThreads()`/`fetchArchivedThreads()` on `IncludeThreads`; when false, no thread discovery or thread message fetching occurs | Yes — `TestSync_IncludeThreadsFalse_SkipsThreads` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::Sync` (IncludeThreads gate) |
| SCN-DC-CMD-003 | Yes — `ParseBotCommand()` calls `isSafeURL()` which blocks link-local (169.254.0.0/16), loopback (127.0.0.0/8), and RFC1918 ranges; on unsafe URL the capture_url field is omitted while capture_comment retains the original text | Yes — `TestParseBotCommand_SSRFProtection`, `TestIsSafeURL` PASS | `internal/connector/discord/discord_test.go` | `internal/connector/discord/discord.go::ParseBotCommand`, `isSafeURL` |

**Disposition:** All eight scenarios are **delivered-but-undocumented** — artifact-only fix.

## Acceptance Criteria

- [x] Parent `specs/014-discord-connector/scopes.md` has a DoD bullet that explicitly contains `SCN-DC-NRM-001` and `SCN-DC-NRM-002` (Scope 01) with restated scenario name and source/test pointers
- [x] Parent `scopes.md` has DoD bullets explicitly containing `SCN-DC-REST-001` and `SCN-DC-REST-002` (Scope 02)
- [x] Parent `scopes.md` has a DoD bullet explicitly containing `SCN-DC-CONN-001` (Scope 03)
- [x] Parent `scopes.md` has a DoD bullet explicitly containing `SCN-DC-GW-001` (Scope 04)
- [x] Parent `scopes.md` has a DoD bullet explicitly containing `SCN-DC-THR-004` (Scope 05)
- [x] Parent `scopes.md` has a DoD bullet explicitly containing `SCN-DC-CMD-003` (Scope 06)
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/014-discord-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/014-discord-connector` PASS (RESULT: PASSED, 13/13 mapped, 0 unmapped)
- [x] No production code changed (boundary)
