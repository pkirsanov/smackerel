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
