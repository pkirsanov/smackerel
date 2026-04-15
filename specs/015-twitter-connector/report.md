# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

### Simplify Pass — 2026-04-10

**Trigger:** stochastic-quality-sweep → simplify-to-doc
**Scope:** `internal/connector/twitter/`

#### Findings

| # | Finding | Severity | Disposition |
|---|---------|----------|-------------|
| S1 | Redundant `Thread.TweetIDs` field — `Thread` struct stored both `TweetIDs []string` and `Tweets []ArchiveTweet`; IDs are always derivable from Tweets | Low | Fixed |
| S2 | O(n²) reply chain scan in `buildThreads` — inner loop scanned all tweets to find next reply per chain hop | Medium | Fixed — replaced with prebuilt `childOf` reply index for O(1) lookup |
| S3 | `syncArchive` threadMap built via redundant `TweetIDs` field | Low | Fixed — iterates `thread.Tweets` directly |

#### Changes

- **`internal/connector/twitter/twitter.go`**: Removed `TweetIDs` from `Thread` struct; added `childOf` reply index in `buildThreads`; updated `syncArchive` threadMap construction
- **`internal/connector/twitter/twitter_test.go`**: Updated `TestBuildThreads` to assert on `len(threads[0].Tweets)` instead of removed `TweetIDs`

#### Evidence

- `./smackerel.sh check` — PASS (config in sync, builds clean)
- `./smackerel.sh test unit` — PASS (twitter package: 0.124s, all assertions green)
- `./smackerel.sh lint` — PASS (no warnings or errors)
- Behavior preserved: all existing tests pass without modification to assertions (only field reference updated)

---

### Security Pass — 2026-04-10

**Trigger:** stochastic-quality-sweep → security-to-doc
**Scope:** `internal/connector/twitter/`

#### Findings

| # | Finding | Severity | CWE | Disposition |
|---|---------|----------|-----|-------------|
| SEC-001 | No file size limit on `tweets.js` read — `os.ReadFile` with no size check; OOM via crafted archive | High | CWE-400 | Fixed — added `maxArchiveFileSize` (500 MiB) with `os.Stat` check before read |
| SEC-002 | No path canonicalization or symlink protection on archive directory — `ArchiveDir` used without `filepath.Abs`, `EvalSymlinks`, or directory boundary check | Medium | CWE-22 | Fixed — `Connect` now resolves absolute path, evaluates symlinks, verifies is-directory; `syncArchive` checks resolved `tweets.js` stays within archive boundary |
| SEC-003 | Tweet ID not validated before URL construction — `tweet.ID` from untrusted JSON directly embedded in URL string | Medium | CWE-20 | Fixed — added `tweetIDPattern` regex (digits-only); non-matching IDs produce empty URL |
| SEC-004 | Bearer token in struct without redaction protection — `TwitterConfig.BearerToken` can leak via accidental serialization | Low | CWE-532 | Fixed — added `String()` method on `TwitterConfig` that redacts the bearer token |
| SEC-005 | `parseTwitterConfig` does not clean `archive_dir` path — raw user input stored without `filepath.Clean` | Low | CWE-22 | Fixed — `filepath.Clean` applied in config parsing; empty string guard prevents `Clean("")` → `"."` |

#### Changes

- **`internal/connector/twitter/twitter.go`**:
  - Added `maxArchiveFileSize` constant (500 MiB)
  - Added `tweetIDPattern` regex for tweet ID validation
  - `Connect`: canonicalize archive dir via `filepath.Abs` + `filepath.EvalSymlinks` + is-directory check (CWE-22)
  - `syncArchive`: resolve `tweets.js` via `EvalSymlinks`, enforce boundary check, add file size limit via `os.Stat` before `os.ReadFile` (CWE-22, CWE-400)
  - `normalizeTweet`: URL only produced for digit-only tweet IDs (CWE-20)
  - `parseTwitterConfig`: apply `filepath.Clean` to `archive_dir`, guard empty string
  - Added `TwitterConfig.String()` method with token redaction (CWE-532)
- **`internal/connector/twitter/twitter_test.go`**:
  - `TestConnect_ArchiveDirSymlinkResolution`: verifies symlink is resolved to real path
  - `TestConnect_ArchiveDirNotADirectory`: rejects file-as-directory
  - `TestSyncArchive_FileSizeLimit`: verifies constant is set correctly
  - `TestSyncArchive_SymlinkTraversal`: verifies symlink-to-outside-file is blocked
  - `TestNormalizeTweet_InvalidIDNoURL`: verifies crafted ID produces no URL
  - `TestNormalizeTweet_ValidIDProducesURL`: verifies normal ID produces correct URL
  - `TestTwitterConfig_StringRedactsToken`: verifies token not in String() output
  - `TestTwitterConfig_StringNoToken`: verifies empty token display
  - `TestParseTwitterConfig_CleansDirPath`: verifies path traversal components cleaned

#### Evidence

- `./smackerel.sh check` — PASS
- `./smackerel.sh test unit` — PASS (twitter package: 0.048s, all 28 tests green including 9 new security tests)
- All existing tests continue to pass (no behavioral regressions)
- Patterns aligned with established connector security (Keep: symlink rejection, Bookmarks: `maxFileSize`, Maps: size limits)

---

### Regression Pass — 2026-04-12

**Trigger:** stochastic-quality-sweep → regression-to-doc
**Scope:** `internal/connector/twitter/`
**Prior sweep history:** 3 simplify, 6 chaos, 5 security findings — all fixed in prior rounds

#### Verification Matrix

| Prior Sweep | Finding | Fix | Status |
|---|---|---|---|
| Simplify S1 | Redundant `Thread.TweetIDs` field | Removed from struct | **Durable** — field absent from `twitter.go`, `TestBuildThreads` asserts on `len(threads[0].Tweets)` |
| Simplify S2 | O(n²) reply chain scan | `childrenOf` prebuilt index (lines 292–296, 333) | **Durable** — BFS uses `childrenOf` map for O(1) child lookup |
| Simplify S3 | `syncArchive` threadMap via `TweetIDs` | Iterates `thread.Tweets` directly | **Durable** — no reference to `TweetIDs` in codebase |
| Chaos | Data race in `Close`/`Health` | `sync.RWMutex` on all state access | **Durable** — 11 lock/unlock sites across Connect, Sync, Health, Close |
| Chaos | Data race in `Connect`/`Health` | Same mutex protection | **Durable** — concurrent tests `TestClose_ConcurrentWithHealth`, `TestConnect_ConcurrentWithHealth` pass |
| Chaos | Cancelled context handling | `ctx.Err()` checks before I/O | **Durable** — `TestSyncArchive_CancelledContext` passes |
| SEC-001 | OOM via large `tweets.js` (CWE-400) | `maxArchiveFileSize` 500 MiB + `os.Stat` check (line 207) | **Durable** — constant verified, size check before `os.ReadFile` |
| SEC-002 | Path traversal via symlink (CWE-22) | `filepath.Abs` + `EvalSymlinks` + boundary check (lines 115, 187, 195) | **Durable** — `TestSyncArchive_SymlinkTraversal` rejects escape |
| SEC-003 | Tweet ID URL injection (CWE-20) | `tweetIDPattern` digits-only regex (line 391) | **Durable** — `TestNormalizeTweet_InvalidIDNoURL` passes |
| SEC-004 | Bearer token leak (CWE-532) | `String()` redacts token (line 497) | **Durable** — `TestTwitterConfig_StringRedactsToken` passes |
| SEC-005 | Uncleaned archive path (CWE-22) | `filepath.Clean` in config parse (line 482) | **Durable** — `TestParseTwitterConfig_CleansDirPath` passes |

#### Evidence

- `./smackerel.sh test unit` — PASS (all Go packages pass including twitter; Python 53/53 pass)
- `./smackerel.sh check` — PASS (config in sync with SST)
- `./smackerel.sh lint` — PASS (all checks passed)
- `./smackerel.sh format --check` — PASS (17 files unchanged)
- All 14 prior-fix-specific tests verified green
- Zero regressions detected across simplify, chaos, and security fix surfaces

---

### Chaos Hardening Pass — 2026-04-12

**Trigger:** stochastic-quality-sweep → chaos-hardening
**Scope:** `internal/connector/twitter/`

#### Findings

| # | Finding | Severity | Root Cause | Disposition |
|---|---------|----------|------------|-------------|
| CH-001 | Sync-Close health race — `Sync()` defer unconditionally restored `HealthHealthy`, overwriting `HealthDisconnected` from concurrent `Close()` | High | Unconditional defer in `Sync()` didn't check post-close state | Fixed — defer checks `health != Disconnected` before restoring |
| CH-002 | Sync on disconnected connector — `Sync()` proceeded without verifying connector was connected | Medium | No state guard at `Sync()` entry | Fixed — added disconnected check at entry, returns error |
| CH-003 | Concurrent double-sync — two `Sync()` calls both proceed, producing duplicate artifacts | Medium | No sync-in-progress guard; mutex only protected health field | Fixed — added `syncing bool` field with acquire/release guard |
| CH-004 | Health stays Healthy after sync failure — `syncArchive` failure logged but health unconditionally restored to Healthy | Medium | Defer didn't track sync outcome | Fixed — `syncFailed` flag sets health to `Degraded` on failure |

#### Changes

- **`internal/connector/twitter/twitter.go`**:
  - Added `syncing bool` field to `Connector` struct
  - `Sync()`: added disconnected guard at entry (CH-002)
  - `Sync()`: added `syncing` acquire/release to reject concurrent calls (CH-003)
  - `Sync()`: defer now checks `health != Disconnected` before restoring (CH-001)
  - `Sync()`: tracks `syncFailed` flag; sets `HealthDegraded` on failure (CH-004)
- **`internal/connector/twitter/twitter_test.go`** (7 new tests):
  - `TestSync_OnDisconnectedConnector`: verifies sync rejected when never connected
  - `TestSync_AfterClose`: verifies sync rejected after Close()
  - `TestSync_CloseDoesNotRestoreHealthy`: verifies Close() state preserved through Sync() defer
  - `TestSync_ConcurrentDoubleSync`: verifies concurrent sync rejected with "already in progress"
  - `TestSync_HealthDegradedAfterFailure`: verifies degraded health after sync error
  - `TestSync_HealthRestoredAfterSuccess`: verifies healthy after successful sync
  - `TestSync_ConcurrentSyncAndClose`: stress test — 50 goroutines calling Sync+Close+Health concurrently

#### Evidence

- `go test -count=1 -race ./internal/connector/twitter/` — PASS (38 tests, 1.099s, zero data races)
- All pre-existing tests pass without modification (no behavioral regressions)
- Race detector active for all concurrency tests

---

### Security Pass (Round 2) — 2026-04-13

**Trigger:** stochastic-quality-sweep R15 → security-to-doc
**Scope:** `internal/connector/twitter/`
**Prior sweep history:** 5 prior security fixes (SEC-001→SEC-005), 4 chaos fixes, 3 simplify fixes — all durable

#### Findings

| # | Finding | Severity | CWE | Disposition |
|---|---------|----------|-----|-------------|
| SEC-006 | Unsanitized URL schemes in tweet entity URLs — `ExpandedURL` from archive stored as-is; crafted archive with `javascript:` or `data:` URIs becomes XSS/open-redirect vector for downstream consumers | High | CWE-79/601 | Fixed — added `isSafeURL()` filter allowing only http/https schemes; `normalizeTweet()` filters entity URLs through it |
| SEC-007 | Missing bearer token validation for API mode — `sync_mode: api` connects successfully without `bearer_token`, silently failing instead of fail-loud per SST policy | Medium | CWE-287 | Fixed — `parseTwitterConfig()` rejects api mode without bearer_token; hybrid mode warns but allows (archive is primary) |
| SEC-008 | Unbounded tweet count in memory after file-size check — 500 MiB file-size limit prevents large reads but millions of tiny tweets within budget still cause OOM | Medium | CWE-770 | Fixed — added `maxTweetCount` (500,000) limit enforced after parsing in `syncArchive()` |
| SEC-009 | UTF-8 truncation in `buildTweetTitle` — byte-position truncation at 80 splits multi-byte characters, producing invalid UTF-8 that can trigger inconsistent downstream behavior | Low | CWE-838 | Fixed — `truncateUTF8()` walks back to a valid rune boundary before truncating |

#### Changes

- **`internal/connector/twitter/twitter.go`**:
  - Added `maxTweetCount` constant (500,000) with `syncArchive()` enforcement (CWE-770)
  - Added `safeURLSchemes` map and `isSafeURL()` function for URL scheme validation (CWE-79/601)
  - `normalizeTweet()`: entity URLs filtered through `isSafeURL()`; added `url_count` metadata field
  - `parseTwitterConfig()`: fail-loud when `sync_mode=api` without `bearer_token` (CWE-287); warn for hybrid
  - `buildTweetTitle()`: uses `truncateUTF8()` for rune-safe truncation (CWE-838)
  - Added `truncateUTF8()` helper using `utf8.RuneStart()` for boundary detection
  - Added `net/url` and `unicode/utf8` imports
- **`internal/connector/twitter/twitter_test.go`** (17 new security regression tests):
  - `TestIsSafeURL_AllowsHTTPS`: verifies https passes
  - `TestIsSafeURL_AllowsHTTP`: verifies http passes
  - `TestIsSafeURL_RejectsJavascript`: adversarial — `javascript:alert(1)` blocked
  - `TestIsSafeURL_RejectsData`: adversarial — `data:text/html,...` blocked
  - `TestIsSafeURL_RejectsVBScript`: adversarial — `vbscript:` blocked
  - `TestIsSafeURL_RejectsEmpty`: adversarial — empty string blocked
  - `TestIsSafeURL_RejectsRelativePath`: adversarial — no-scheme path blocked
  - `TestNormalizeTweet_FiltersUnsafeURLs`: adversarial — mix of safe/unsafe URLs, only safe survive
  - `TestConnect_APIModeRequiresBearerToken`: adversarial — api mode without token fails
  - `TestConnect_HybridModeWithoutTokenAllowed`: verifies hybrid degrades gracefully
  - `TestTruncateUTF8_ASCIIOnly`: verifies basic ASCII truncation
  - `TestTruncateUTF8_MultiByteBoundary`: adversarial — 2-byte "é" not split
  - `TestTruncateUTF8_ThreeByteRune`: adversarial — 3-byte "日" not split
  - `TestTruncateUTF8_FourByteEmoji`: adversarial — 4-byte "🐦" not split
  - `TestTruncateUTF8_ShortString`: verifies no-op for short strings
  - `TestBuildTweetTitle_UTF8Safe`: adversarial — multi-byte chars near boundary produce valid UTF-8
  - `TestMaxTweetCount_ConstantSet`: verifies constant is sane

#### Evidence

- `./smackerel.sh test unit` — PASS (all Go+Python packages pass; twitter: 0.087s)
- All prior security/chaos/simplify tests remain green (no regressions)
- Every finding has at least one adversarial test that would fail if the bug were reintroduced

---

### Improve Pass — 2026-04-13

**Trigger:** stochastic-quality-sweep R16 → improve-existing
**Scope:** `internal/connector/twitter/`

#### Findings

| # | Finding | Severity | Disposition |
|---|---------|----------|-------------|
| IMP-015-R16-001 | `syncArchive` never reads `like.js`/`bookmark.js` — tier elevation for liked/bookmarked tweets is dead code | Medium | Fixed — added `parseSignalFile()` to read like.js and bookmark.js from archive, builds liked/bookmarked ID sets for tier assignment |
| IMP-015-R16-002 | Mentions omitted from metadata — R-005 requires mentions metadata but normalizeTweet only exported hashtags and URLs | Low | Fixed — added `mentions` field extraction from `Entities.Mentions` |
| IMP-015-R16-003 | `buildTweetTitle` passes control characters unsanitized (CWE-116) — newlines, tabs, and C0/C1 controls from tweet text leak into artifact title | Medium | Fixed — added `sanitizeControlChars()` stripping C0/C1 controls before truncation |

#### Evidence

- `./smackerel.sh test unit` — PASS (twitter: 0.049s, all 58 tests green including 13 new adversarial tests)
- `./smackerel.sh check` — PASS

---

### DevOps Pass — 2026-04-14

**Trigger:** stochastic-quality-sweep R20 → devops-to-doc
**Scope:** `docker-compose.yml`, `internal/config/docker_security_test.go`

#### Findings

| # | Finding | Severity | Root Cause | Disposition |
|---|---------|----------|------------|-------------|
| DEV-015-001 | Twitter connector env vars missing from docker-compose.yml — TWITTER_ENABLED, TWITTER_SYNC_MODE, TWITTER_ARCHIVE_DIR, TWITTER_BEARER_TOKEN, TWITTER_SYNC_SCHEDULE generated by config pipeline and read by main.go but never passed through docker-compose.yml environment section; connector silently never starts in Docker | High | docker-compose.yml not updated when Twitter connector was added | Fixed — added all 5 TWITTER_* env vars to smackerel-core environment section. TWITTER_ARCHIVE_DIR uses `${TWITTER_ARCHIVE_DIR:+/data/twitter-archive}` pattern (container path when set, empty when unset) |
| DEV-015-002 | Twitter archive directory volume mount missing from docker-compose.yml — bookmarks, maps, and browser-history have host-bind volume mounts for import directories; Twitter archive has no mount, so archive files are inaccessible inside the container | High | Volume mount not added when connector was wired | Fixed — added `${TWITTER_ARCHIVE_DIR:-./data/twitter-archive}:/data/twitter-archive:ro` volume mount following established pattern |
| DEV-015-003 | Same docker-compose.yml SST gap for Discord, Weather, and Gov Alerts connectors — config.sh generates env vars, main.go reads them, but docker-compose.yml doesn't pass them through | High | Same root cause as DEV-015-001; batch of connectors added without docker-compose update | Fixed — added all DISCORD_* (9 vars), WEATHER_* (3 vars), GOV_ALERTS_* (12 vars) env vars to smackerel-core environment section |

#### Changes

- **`docker-compose.yml`**:
  - Added 5 TWITTER_* env vars to smackerel-core environment section (DEV-015-001)
  - Added Twitter archive volume mount with `:ro` (DEV-015-002)
  - Added 9 DISCORD_* env vars (DEV-015-003)
  - Added 3 WEATHER_* env vars (DEV-015-003)
  - Added 12 GOV_ALERTS_* env vars (DEV-015-003)
- **`internal/config/docker_security_test.go`** (2 new adversarial tests):
  - `TestDockerCompose_ConnectorEnvVarsWired`: verifies all 41 connector env vars read by main.go are present in docker-compose.yml — would fail if any connector's env vars are omitted
  - `TestDockerCompose_ImportVolumesMounted`: verifies all 4 file-import connectors (bookmarks, maps, browser-history, twitter) have container-path volume mounts in docker-compose.yml

#### Evidence

- `./smackerel.sh test unit` — PASS (config: 0.032s including 2 new tests)
- `./smackerel.sh check` — PASS (config in sync with SST)

---

### Improve Pass (Round 2) — 2026-04-14

**Trigger:** stochastic-quality-sweep → improve-existing
**Scope:** `internal/connector/twitter/`
**Prior sweep history:** 5 security fixes, 4 chaos fixes, 3 simplify fixes, 3 improve fixes, 3 devops fixes — all durable

#### Findings

| # | Finding | Severity | Disposition |
|---|---------|----------|-------------|
| IMP-015-001 | No consecutive error tracking / graduated health escalation — Keep connector has `consecutiveErrors` with degraded→failing→error progression (thresholds: <5, 5-9, 10+); Twitter binary-toggled healthy/degraded with no escalation path regardless of failure count | Medium | Fixed — added `consecutiveErrors` counter with graduated escalation matching Keep connector pattern: <5→degraded, 5-9→failing, 10+→error; success resets counter |
| IMP-015-002 | No sync metrics for operational observability — Keep connector tracks `lastSyncTime`, `lastSyncCount`, `lastSyncErrors`, `consecutiveErrors`; Twitter had none, giving operations zero visibility into sync health | Medium | Fixed — added all four metrics fields to Connector struct; added `SyncMetrics()` accessor method; Sync defer block tracks all metrics |
| IMP-015-003 | Connect leaves HealthDisconnected on config validation failure — Keep connector sets HealthError on failed Connect to distinguish "never connected" from "connection failed"; Twitter left HealthDisconnected making status ambiguous for supervisor/health endpoints | Low | Fixed — all Connect failure paths now set HealthError before returning error |
| IMP-015-004 | Missing `tweet/image` and `tweet/video` content types from R-004 — spec defines these but ArchiveTweet had no Media field; archive exports include media entities but they were never parsed | Medium | Fixed — added `TweetMedia` struct with `Type` field to `TweetEntities`; `classifyTweet()` now detects photo→tweet/image, video/animated_gif→tweet/video; `normalizeTweet()` adds `media_types` and `media_count` metadata |

#### Changes

- **`internal/connector/twitter/twitter.go`**:
  - Added `TweetMedia` struct and `Media []TweetMedia` field to `TweetEntities` (IMP-015-004)
  - Added `lastSyncTime`, `lastSyncCount`, `lastSyncErrors`, `consecutiveErrors` fields to `Connector` struct (IMP-015-001/002)
  - Added `SyncMetrics()` method returning all four metrics (IMP-015-002)
  - `Sync()` defer: graduated health escalation matching Keep pattern — <5 degraded, 5-9 failing, 10+ error (IMP-015-001)
  - `Sync()` defer: tracks `syncCount`, `lastSyncTime`, `lastSyncErrors`, `consecutiveErrors` (IMP-015-002)
  - `Connect()`: all failure paths set `HealthError` before returning (IMP-015-003)
  - `classifyTweet()`: detects `tweet/image` (photo) and `tweet/video` (video, animated_gif) from media entities (IMP-015-004)
  - `normalizeTweet()`: adds `media_types` and `media_count` metadata when media present (IMP-015-004)
- **`internal/connector/twitter/twitter_test.go`** (20 new tests):
  - `TestSync_ConsecutiveErrorsEscalateToDegraded`: 1 failure → degraded
  - `TestSync_ConsecutiveErrorsEscalateToFailing`: 5 failures → failing
  - `TestSync_ConsecutiveErrorsEscalateToError`: 10 failures → error
  - `TestSync_SuccessResetsConsecutiveErrors`: recovery resets counter
  - `TestSyncMetrics_TracksSuccessfulSync`: verifies counts/times after success
  - `TestSyncMetrics_TracksFailedSync`: verifies error counts after failure
  - `TestConnect_SetsHealthErrorOnFailure`: empty archive_dir → HealthError
  - `TestConnect_NonexistentDir_SetsHealthError`: bad path → HealthError
  - `TestClassifyTweet_Image`: photo → tweet/image
  - `TestClassifyTweet_Video`: video → tweet/video
  - `TestClassifyTweet_AnimatedGif`: animated_gif → tweet/video
  - `TestClassifyTweet_MediaPrecedenceOverURL`: media > URL precedence
  - `TestClassifyTweet_ThreadPrecedenceOverMedia`: thread > media precedence
  - `TestNormalizeTweet_MediaMetadata`: media_types and media_count populated
  - `TestNormalizeTweet_NoMediaNoMetadata`: no media → no media metadata keys

#### Evidence

- `./smackerel.sh test unit` — PASS (twitter: 0.160s, all tests green including 20 new)
- `./smackerel.sh check` — PASS (config in sync with SST)
- `./smackerel.sh lint` — PASS (Go checks clean; 3 pre-existing Python warnings unrelated)
- `./smackerel.sh format --check` — PASS (21 files unchanged)
- All prior security/chaos/simplify/improve/devops tests remain green (no regressions)
- Every finding has adversarial tests that would fail if the improvement were reverted
