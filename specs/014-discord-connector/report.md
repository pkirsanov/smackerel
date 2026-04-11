# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

### Gaps-To-Doc Sweep — 2026-04-10

**Trigger:** `gaps` probe via stochastic-quality-sweep
**Mode:** `gaps-to-doc`
**Agent:** `bubbles.workflow` (child of stochastic sweep)

#### Findings (11 gaps identified)

| # | Gap | Spec Ref | Severity | Status |
|---|-----|----------|----------|--------|
| G1 | No discord connector section in `config/smackerel.yaml` | R-002 | Medium | Fixed |
| G2 | `DiscordMessage` missing Attachments, Reactions, MessageReference, Thread fields | R-003, R-004 | High | Fixed |
| G3 | Classification missing `discord/attachment`, `discord/reply`, `discord/thread`, `discord/capture` | R-003 | High | Fixed |
| G4 | Tier assignment missing reaction ≥5→full, code→standard, reply→standard | R-007 | High | Fixed |
| G5 | Metadata missing server_name, channel_name, thread_id, thread_name, reply_to_id, reaction_count, reactions, attachments, mentions | R-004 | High | Fixed |
| G6 | No `RateLimiter` implementation | R-006, Design | Medium | Fixed |
| G7 | No pinned message fetching in Sync() | R-003, Scope 2 | Medium | Fixed |
| G8 | No thread handling stubs in Sync() | R-009, Scope 5 | Medium | Fixed |
| G9 | No bot command capture logic | R-010, Scope 6 | Medium | Fixed |
| G10 | Config parsing missing EnableGateway, IncludeThreads, IncludePins, CaptureCommands | R-002 | Medium | Fixed |
| G11 | Missing test coverage for attachment, reply, thread, capture, reaction tier, rate limiter, bot command | — | High | Fixed |

#### Remediation Summary

**Files modified:**
- `internal/connector/discord/discord.go` — Added Attachment, Reaction, MessageRef types; complete R-004 metadata; all 8 content types in classification; R-007 tier rules (reactions, code blocks, replies); RateLimiter struct; pinned/thread/bot-command stubs in Sync(); ParseBotCommand(); full config parsing
- `internal/connector/discord/discord_test.go` — Added tests for attachment/reply/thread/capture classification, reaction tier, code→standard tier, thread metadata, reply metadata, totalReactions, ParseBotCommand, RateLimiter ShouldWait/Update
- `config/smackerel.yaml` — Added discord connector section with SST-compliant empty-string placeholders

#### Validation

- `./smackerel.sh test unit` — all tests pass (discord package: 0.058s)
- `./smackerel.sh check` — clean
- `./smackerel.sh lint` — all checks passed

---

### Simplify-To-Doc Sweep — 2026-04-10

**Trigger:** `simplify` probe via stochastic-quality-sweep
**Mode:** `simplify-to-doc`
**Agent:** `bubbles.workflow` (child of stochastic sweep)

#### Findings (2 simplification opportunities identified)

| # | Finding | Severity | Status |
|---|---------|----------|--------|
| S1 | Three redundant nested channel iteration loops in `Sync()` — messages, pins, and threads each had their own `MonitoredChannels → ChannelIDs` double loop | Low | Fixed |
| S2 | Redundant `continue` statements after fetch errors — fetch returns `nil` on error, so `for range` over nil is already a no-op; `continue` was dead control flow | Low | Fixed |

#### Remediation Summary

**Files modified:**
- `internal/connector/discord/discord.go` — Consolidated three separate `MonitoredChannels → ChannelIDs` loops (messages, pins, threads) into a single unified loop per channel. Removed redundant `continue` statements after fetch error logging. Net reduction: ~15 lines of duplicated loop boilerplate. Behavior is identical — all fetch types were already independent (errors in one type never affected another).

#### Validation

- `./smackerel.sh test unit` — all tests pass (discord package: 0.033s, ran fresh)
- No behavior changes — all existing tests pass unchanged

---

### Regression-To-Doc Sweep — 2026-04-10

**Trigger:** `regression` probe via stochastic-quality-sweep
**Mode:** `regression-to-doc`
**Agent:** `bubbles.workflow` (child of stochastic sweep)

#### Probes Executed

| Probe | Target | Result |
|-------|--------|--------|
| Unit test suite | All 31 Go packages + 44 Python tests | All pass |
| Static analysis | `./smackerel.sh check` | Clean (SST in sync) |
| Lint | `./smackerel.sh lint` | All checks passed |
| Cross-spec SourceID conflict | `discord` vs all other connector SourceIDs | No conflict — unique ID |
| Connector interface compliance | Discord `New/Connect/Sync/Health/Close` vs `connector.Connector` | Fully compliant |
| Config SST compliance | `config/smackerel.yaml` discord section | SST-compliant — empty-string placeholders, no hardcoded defaults in code |
| Gaps fix durability (G1–G11) | All 11 gap fixes from round 3 | All durable — verified in code and tests |
| Simplify fix durability (S1–S2) | Both simplification fixes from round 8 | All durable — unified loop confirmed, no dead control flow |
| Design–implementation alignment | Design doc vs actual implementation | Aligned — simplified internal types instead of raw discordgo types is a valid scaffold pattern |
| Peer connector pattern consistency | Discord vs Twitter/YouTube/Maps/Weather constructors | Consistent — same `New(id string)` pattern, same interface shape |

#### Findings

**Zero regression findings.** All prior fixes from gaps (round 3, 11 items) and simplify (round 8, 2 items) remain durable. No cross-spec conflicts, no baseline test regressions, no design contradictions, and no flow breakage detected.

#### Durability Evidence — Prior Fixes

| Fix | Round | Verified By | Status |
|-----|-------|-------------|--------|
| G1: Config in smackerel.yaml | gaps | `grep discord config/smackerel.yaml` — present at line 136 | Durable |
| G2: DiscordMessage types | gaps | Attachment, Reaction, MessageRef, Thread fields all present | Durable |
| G3: 8 content type classification | gaps | `TestClassifyMessage` — 10 cases covering all types | Durable |
| G4: R-007 tier rules | gaps | `TestAssignTier` — 10 cases covering reactions, code, reply | Durable |
| G5: R-004 metadata | gaps | `TestNormalizeMessage` — validates all metadata fields | Durable |
| G6: RateLimiter | gaps | `TestRateLimiter` — ShouldWait/Update/expired bucket | Durable |
| G7: Pinned messages in Sync | gaps | `IncludePins` guard + `fetchPinnedMessages` call in unified loop | Durable |
| G8: Thread handling in Sync | gaps | `IncludeThreads` guard + `fetchActiveThreads` call in unified loop | Durable |
| G9: Bot command capture | gaps | `TestParseBotCommand` — 5 cases, URL extraction + comment | Durable |
| G10: Full config parsing | gaps | `TestConnect_ValidConfig` — all fields parsed correctly | Durable |
| G11: Test coverage expansion | gaps | 15+ tests covering all content types, tiers, metadata, rate limiter | Durable |
| S1: Unified channel loop | simplify | Single `MonitoredChannels → ChannelIDs` loop in Sync() | Durable |
| S2: Dead continue removal | simplify | No redundant continue after fetch error logging | Durable |

#### Validation

- `./smackerel.sh test unit` — all 31 Go packages pass, 44 Python tests pass
- `./smackerel.sh check` — SST in sync, clean
- `./smackerel.sh lint` — all checks passed

---

### Stabilize-To-Doc Sweep — 2026-04-10

**Trigger:** `stabilize` probe via stochastic-quality-sweep
**Mode:** `stabilize-to-doc`
**Agent:** `bubbles.workflow` (child of stochastic sweep)

#### Findings (5 stability issues identified)

| # | Finding | Category | Severity | Status |
|---|---------|----------|----------|--------|
| ST1 | Data race: `Connect()` sets `c.health` without mutex while `Health()` reads under `RLock` | Race condition | High | Fixed |
| ST2 | Data race: `Close()` sets `c.health` without mutex while `Health()` reads under `RLock` | Race condition | High | Fixed |
| ST3 | `Sync()` never checks `ctx.Done()` — context cancellation ignored across entire channel iteration | Resource leak / timeout | Medium | Fixed |
| ST4 | `Sync()` swallows all fetch errors, returns `nil` even when every channel fails — caller unaware of failures | Error recovery | Medium | Fixed |
| ST5 | `RateLimiter.buckets` map grows unbounded — expired entries never pruned | Memory leak | Medium | Fixed |

#### Remediation Summary

**Files modified:**
- `internal/connector/discord/discord.go`:
  - ST1/ST2: `Connect()` and `Close()` now hold `c.mu.Lock()` when writing `c.health`. Eliminates data race with concurrent `Health()` readers.
  - ST3: `Sync()` now checks `ctx.Err()` at the start of each channel iteration. Returns partial results + cursor + error on cancellation.
  - ST4: `Sync()` now aggregates all fetch errors into `syncErrors` slice. Returns partial artifacts with a descriptive error when any channel fails. Cursor marshal error is also logged and returned instead of silently discarded.
  - ST5: `RateLimiter.Update()` now prunes expired buckets when the map exceeds 100 entries. Bounded cleanup prevents unbounded growth.

- `internal/connector/discord/discord_test.go`:
  - Added `TestRateLimiter_PruneExpired` — verifies expired bucket pruning triggers above 100 entries
  - Added `TestSync_ContextCancellation` — verifies cancelled context returns error
  - Added `TestConnect_HealthRaceSafe` — concurrent `Health()` reads during `Connect()` with `-race`
  - Added `TestClose_HealthRaceSafe` — concurrent `Health()` reads during `Close()` with `-race`

#### Validation

- `go test -count=1 -race ./internal/connector/discord/` — 19 tests pass, zero race conditions detected
- `./smackerel.sh build` — clean
- All prior tests (gaps G1–G11, simplify S1–S2) remain passing

---

### Security-To-Doc Sweep — 2026-04-11

**Trigger:** `security` probe via stochastic-quality-sweep
**Mode:** `security-to-doc`
**Agent:** `bubbles.workflow` (child of stochastic sweep)

#### Findings (6 security issues identified)

| # | Finding | OWASP | Severity | Status |
|---|---------|-------|----------|--------|
| SEC-1 | No snowflake ID validation on server/channel IDs — allows path traversal in API URLs and metadata injection | A03 Injection | High | Fixed |
| SEC-2 | No BackfillLimit upper bound — config value of MAX_INT causes unbounded API calls (resource exhaustion) | A05 Misconfig | Medium | Fixed |
| SEC-3 | Cursor deserialization accepts arbitrary channel IDs — attacker-controlled cursor could inject queries to unconfigured channels, values not validated as snowflakes | A08 Integrity | Medium | Fixed |
| SEC-4 | URL construction from unvalidated GuildID/ChannelID/MessageID — malformed IDs produce crafted or misleading discord.com URLs | A03 Injection | High | Fixed |
| SEC-5 | `ParseBotCommand` accepts SSRF-prone URLs (169.254.x.x, localhost, private ranges) — if captured URL is later fetched by the system, internal services are exposed | A10 SSRF | Medium | Fixed |
| SEC-6 | No CaptureCommands length/count validation — unbounded empty or oversized command prefixes accepted | A05 Misconfig | Low | Fixed |

#### Additional Hardening

| # | Improvement | Category | Status |
|---|-------------|----------|--------|
| H-1 | Processing tier validated against known values (full/standard/light/metadata) — rejects arbitrary strings | Input validation | Done |
| H-2 | `buildTitle()` strips ASCII control characters (except \n/\r/\t) — prevents log injection and downstream rendering issues | Output sanitization | Done |

#### Remediation Summary

**Files modified:**

- `internal/connector/discord/discord.go`:
  - Added `isValidSnowflake()` — validates Discord snowflake IDs as numeric uint64 strings
  - Added `isSafeURL()` — SSRF protection rejecting localhost, loopback, RFC 1918 private, link-local, and cloud metadata endpoints
  - Added `sanitizeControlChars()` — strips ASCII control chars from title output
  - Added constants: `maxBackfillLimit=10000`, `maxCaptureCommands=20`, `maxCaptureCommandLen=50`
  - `parseDiscordConfig()`: server_id and channel_id validated via `isValidSnowflake()`, processing_tier allowlisted, backfill_limit capped, capture_commands validated for UTF-8/length/count
  - `Sync()` cursor parsing: each key and value validated as valid snowflake before merging
  - `normalizeMessage()`: URL constructed only when all three IDs pass snowflake validation
  - `ParseBotCommand()`: extracted URLs checked via `isSafeURL()`
  - `buildTitle()`: strips control characters via `sanitizeControlChars()`

- `internal/connector/discord/discord_test.go`:
  - 13 new security tests: snowflake validation, SSRF protection, config bounds, URL safety, cursor injection, control char sanitization
  - Fixed `TestSync_ContextCancellation` to use valid snowflake IDs

#### Validation

- `./smackerel.sh build` — clean
- `./smackerel.sh test unit` — all packages pass (discord: 0.129s)
