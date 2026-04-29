# Report: BUG-014-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard Gate G068 (Gherkin → DoD Content Fidelity) reported that 8 of 13 Gherkin scenarios in `specs/014-discord-connector` had no faithful matching DoD item: `SCN-DC-NRM-001`, `SCN-DC-NRM-002`, `SCN-DC-REST-001`, `SCN-DC-REST-002`, `SCN-DC-CONN-001`, `SCN-DC-GW-001`, `SCN-DC-THR-004`, `SCN-DC-CMD-003`. Investigation confirmed the gap is artifact-only — every scenario is fully delivered in production code (`internal/connector/discord/discord.go`, `gateway.go`) and exercised by passing unit tests. The DoD bullets simply did not embed the `SCN-DC-NNN-NNN` trace IDs that the guard's content-fidelity matcher requires. The fix added 8 trace-ID-bearing DoD bullets to `specs/014-discord-connector/scopes.md`, one per affected scenario, distributed across Scopes 01–06 to preserve scope locality. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. The traceability-guard's pre-fix state (8 unmapped scenarios, 9 failures) has been replaced with a clean `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. The underlying behavior tests for the previously-flagged scenarios still pass with no regressions.

## Test Evidence

> Phase agent: bubbles.bug (test phase)
> Executed: YES

### Underlying behavior tests (regression-protection for the artifact fix)

```
$ go test -count=1 -v -run 'TestClassifyMessage$|TestAssignTier$|TestFetchChannelMessages_Basic$|TestSyncEndToEnd_CursorPreventsRefetch$|TestConnect_TokenValidationSuccess$|TestEventPoller_ConnectStartsPolling$|TestSync_IncludeThreadsFalse_SkipsThreads$|TestIsSafeURL$|TestParseBotCommand_SSRFProtection$' ./internal/connector/discord/
=== RUN   TestIsSafeURL
--- PASS: TestIsSafeURL (0.00s)
=== RUN   TestParseBotCommand_SSRFProtection
--- PASS: TestParseBotCommand_SSRFProtection (0.00s)
=== RUN   TestClassifyMessage
--- PASS: TestClassifyMessage (0.00s)
=== RUN   TestAssignTier
--- PASS: TestAssignTier (0.00s)
=== RUN   TestConnect_TokenValidationSuccess
--- PASS: TestConnect_TokenValidationSuccess (0.03s)
=== RUN   TestFetchChannelMessages_Basic
--- PASS: TestFetchChannelMessages_Basic (0.03s)
=== RUN   TestSyncEndToEnd_CursorPreventsRefetch
--- PASS: TestSyncEndToEnd_CursorPreventsRefetch (0.02s)
=== RUN   TestSync_IncludeThreadsFalse_SkipsThreads
--- PASS: TestSync_IncludeThreadsFalse_SkipsThreads (0.01s)
=== RUN   TestEventPoller_ConnectStartsPolling
--- PASS: TestEventPoller_ConnectStartsPolling (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/discord       0.130s
```

**Claim Source:** executed.

### Validation Evidence

> Phase agent: bubbles.validate
> Executed: YES

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/014-discord-connector 2>&1 | tail -22
✅ Scope 01: Normalizer & Message Classification scenario maps to DoD item: SCN-DC-NRM-001 Classify all message content types
✅ Scope 01: Normalizer & Message Classification scenario maps to DoD item: SCN-DC-NRM-002 Assign processing tiers per R-007
✅ Scope 02: REST Client & Backfill scenario maps to DoD item: SCN-DC-REST-001 Fetch message history with pagination
✅ Scope 02: REST Client & Backfill scenario maps to DoD item: SCN-DC-REST-002 Per-channel cursor advancement
✅ Scope 03: Discord Connector & Config scenario maps to DoD item: SCN-DC-CONN-001 Connector lifecycle
✅ Scope 04: Gateway Event Handler scenario maps to DoD item: SCN-DC-GW-001 Real-time message capture
✅ Scope 05: Thread Ingestion scenario maps to DoD item: SCN-DC-THR-001 Auto-follow active threads in monitored channels
✅ Scope 05: Thread Ingestion scenario maps to DoD item: SCN-DC-THR-002 Thread starter gets discord/thread content type
✅ Scope 05: Thread Ingestion scenario maps to DoD item: SCN-DC-THR-003 Archived thread backfill
✅ Scope 05: Thread Ingestion scenario maps to DoD item: SCN-DC-THR-004 Thread ingestion disabled via config
✅ Scope 06: Bot Command Capture scenario maps to DoD item: SCN-DC-CMD-001 Capture command with URL and comment
✅ Scope 06: Bot Command Capture scenario maps to DoD item: SCN-DC-CMD-002 Capture command without URL
✅ Scope 06: Bot Command Capture scenario maps to DoD item: SCN-DC-CMD-003 Capture command with unsafe URL rejected
ℹ️  DoD fidelity: 13 scenarios checked, 13 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 13
ℹ️  Test rows checked: 40
ℹ️  Scenario-to-row mappings: 13
ℹ️  Concrete test file references: 13
ℹ️  Report evidence references: 13
ℹ️  DoD fidelity scenarios: 13 (mapped: 13, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed. Pre-fix run on the same revision (with the unfixed artifacts) reported `RESULT: FAILED (9 failures, 0 warnings)` including `DoD fidelity: 13 scenarios checked, 5 mapped to DoD, 8 unmapped` — see Section "Pre-fix Reproduction" below.

### Audit Evidence

> Phase agent: bubbles.audit
> Executed: YES

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/014-discord-connector 2>&1 | tail -10
✅ No unfilled evidence template placeholders in scopes.md
✅ No unfilled evidence template placeholders in report.md
✅ No repo-CLI bypass detected in report.md command evidence
✅ All 15 evidence blocks in report.md contain legitimate terminal output
✅ No narrative summary phrases detected in report.md
✅ Spec-review phase recorded for 'reconcile-to-doc' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap 2>&1 | tail -5

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ git diff --name-only
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/design.md
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/report.md
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/scopes.md
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/spec.md
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/state.json
specs/014-discord-connector/bugs/BUG-014-001-dod-scenario-fidelity-gap/uservalidation.md
specs/014-discord-connector/scopes.md
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path.

## Pre-fix Reproduction

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

**Claim Source:** executed (initial guard invocation captured at the start of this bug investigation, before any artifact edits).
