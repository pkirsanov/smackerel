# Bug: [BUG-015-001] Twitter API Polling Path Deprecated

## Summary
Twitter Scope 6 (API Client / Opt-In) DoD claims an API polling code path that does not exist in `internal/connector/twitter/twitter.go`. The connector's `Sync()` only invokes `syncArchive()`; there is no HTTP client, no `/2/users/:id/bookmarks` or `/2/users/:id/liked_tweets` request, and no rate-limit logging. After review, the team selected option (b) — formally deprecate the API path — rather than implement it. This bug captures the decision, the rationale, and the documentation amendments that retire Scope 6 and align spec/scopes/uservalidation/state with the certified archive-only surface.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Documentation drift between claimed and certified surface (P2)
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (reproduced via grep over `internal/connector/twitter/`)
- [x] In Progress
- [x] Fixed (deprecation applied)
- [x] Verified
- [x] Closed

## Reproduction Steps
1. Open `internal/connector/twitter/twitter.go` and inspect `Sync()` (line 184).
2. Run `grep -n "api.twitter.com\|x.com/2\|FetchBookmarks\|FetchLikes\|net/http\|x-rate-limit-remaining" internal/connector/twitter/twitter.go`.
3. Observe: 0 matches. The Sync method only calls `c.syncArchive(ctx, cursor)`.
4. Inspect `specs/015-twitter-connector/scopes.md` Scope 06 DoD: claims `FetchBookmarks polls /2/users/:id/bookmarks`, `Rate limit remaining logged after each API call`, etc.
5. Inspect `specs/015-twitter-connector/uservalidation.md` item 13: previously checked, then flipped to VERIFIED FAIL on 2026-04-26 by `bubbles.validate` replay.

## Expected Behavior
Either (a) the API path is implemented end-to-end with adversarial tests, OR (b) the API path is formally deprecated in spec/design/scopes/uservalidation/state and removed from the certified surface.

## Actual Behavior
Spec claimed an implemented API path. Implementation only had archive-import support plus an unused `BearerToken` config field. uservalidation.md item 13 was checked despite no API code path existing.

## Decision: Option (b) — Deprecate the API Path

**Rationale:**
1. **Twitter/X v2 free tier no longer reliably supports** the bookmarks/likes endpoints for individual users at the rate-limit tier described in the original spec (1,500 reads/month). The X API has been progressively restricted since 2023 and the free-tier eligibility for these endpoints is no longer stable.
2. **The archive-import path is fully implemented and tested** — 127/127 unit tests passing, security/chaos/devops/improve hardening complete, and it covers the real user value: bookmarks, likes, threads, tweet-link extraction, dedup, and metadata preservation are all driven from the official Twitter data export.
3. **Implementing untested paid-tier API integration introduces ongoing maintenance burden** — OAuth2 PKCE flows, rate-limit header parsing, 429 backoff, hybrid merge dedup against archive `tweet.ID`s, and refresh-token rotation would all need adversarial tests with limited marginal benefit over re-exporting the archive.
4. **Spec language already says "Optional API polling"** — formal deprecation aligns with the original intent and removes the confusion that triggered the user-validation regression.

## Environment
- Service: smackerel-core (Go), `internal/connector/twitter/`
- Version: HEAD as of 2026-04-26
- Platform: Linux / Docker Compose

## Root Cause
A previous workflow round marked Scope 6 DoD items checked without an implementation behind them. The certification gate did not catch the drift because the archive path tests were green and the API path had no separate test fixtures. The 2026-04-26 `bubbles.validate` replay caught the gap by grepping for HTTP-client identifiers.

## Related
- Feature: `specs/015-twitter-connector/`
- Category: spec-amendment / deprecation
- Blocks: nothing — archive path remains green and certified
- Triggered by: `bubbles.validate` replay 2026-04-26 (see `specs/015-twitter-connector/report.md` "bubbles.validate Replay" section)
