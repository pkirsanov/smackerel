# Report: 012 — Hospitable Connector

> **Status:** Done

---

## Summary

Hospitable Connector delivered under delivery-lockdown mode. 5 scopes completed: (1) API Client, Types & Config; (2) Connector Implementation & Normalizer; (3) Edge Hints, Cross-Domain Linking & Hardening; (4) Message Sync Reliability & Client Hardening; (5) Normalizer Quality Fixes. Implementation: types.go, client.go, connector.go, normalizer.go in `internal/connector/hospitable/`. 52 unit tests across 2 test files — all pass. PAT authentication, paginated fetching, rate limit handling, 4 resource types (property, reservation, message, review), incremental cursor sync, knowledge graph edge hints, partial failure isolation, property name cache.

## Completion Statement

All 5 scopes are Done. All DoD items verified with passing tests. `./smackerel.sh test unit` passes all 25 Go packages including `internal/connector/hospitable` (2.952s). `./smackerel.sh lint` exits 0. `./smackerel.sh check` confirms config in sync. Delivery-lockdown certification complete.

## Test Evidence

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/api             0.129s
ok      github.com/smackerel/smackerel/internal/auth            1.546s
ok      github.com/smackerel/smackerel/internal/config          0.047s
ok      github.com/smackerel/smackerel/internal/connector       1.147s
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.227s
ok      github.com/smackerel/smackerel/internal/connector/browser       0.113s
ok      github.com/smackerel/smackerel/internal/connector/caldav        0.022s
ok      github.com/smackerel/smackerel/internal/connector/hospitable    2.952s
ok      github.com/smackerel/smackerel/internal/connector/imap  0.063s
ok      github.com/smackerel/smackerel/internal/connector/keep  0.362s
ok      github.com/smackerel/smackerel/internal/connector/maps  0.336s
ok      github.com/smackerel/smackerel/internal/connector/rss   0.275s
ok      github.com/smackerel/smackerel/internal/connector/youtube       0.026s
ok      github.com/smackerel/smackerel/internal/db              0.123s
ok      github.com/smackerel/smackerel/internal/digest          0.049s
ok      github.com/smackerel/smackerel/internal/extract         0.082s
ok      github.com/smackerel/smackerel/internal/graph           0.116s
ok      github.com/smackerel/smackerel/internal/intelligence    0.054s
ok      github.com/smackerel/smackerel/internal/nats            0.172s
ok      github.com/smackerel/smackerel/internal/pipeline        0.297s
ok      github.com/smackerel/smackerel/internal/scheduler       0.049s
ok      github.com/smackerel/smackerel/internal/telegram        14.499s
ok      github.com/smackerel/smackerel/internal/topics          0.007s
ok      github.com/smackerel/smackerel/internal/web             0.018s
ok      github.com/smackerel/smackerel/internal/web/icons       0.014s
44 passed in 1.12s
```

Hospitable-specific tests (52 tests across 2 files):

- `connector_test.go` (32 tests): TestClientAuthHeader, TestClientValidateSuccess, TestClientValidateUnauthorized, TestClientValidateForbidden, TestClientPaginatesProperties, TestClientRetryOn429, TestClientMaxRetriesOn429, TestClientRetryOnServerError, TestClientURLConstruction, TestConnectorID, TestConnectValidConfig, TestConnectInvalidToken, TestConfigValidationMissingToken, TestConfigValidationDefaults, TestSyncCursorMarshal, TestCursorEmptyAppliesLookback, TestHealthTransitions, TestDisabledResourceSkipped, TestSyncFullLifecycle, TestPartialFailureReturnsSuccessful, TestAllFailuresSetHealthError, TestPropertyNameCacheEnrichesTitle, TestConnectEmptyToken, TestActiveReservationMessageSync, TestParseRetryAfterSeconds, TestParseRetryAfterHTTPDate, TestParseRetryAfterEmpty, TestParseRetryAfterInvalid, TestRetryAfterUsedOn429, TestPropertyNameCachePersistsInCursor, TestPropertyNameCacheLoadedFromCursor, TestMessageCursorNotAdvancedOnFailure
- `normalizer_test.go` (20 tests): TestNormalizeProperty, TestNormalizeReservation, TestNormalizeReservationFallbackPropertyID, TestNormalizeMessage, TestNormalizeReview, TestNormalizeReviewFallbackPropertyID, TestNormalizeAllTiers, TestClassifySenderGuest, TestClassifySenderHost, TestClassifySenderAutomated, TestClassifySenderDefaultGuest, TestNormalizeMessageHostSender, TestNormalizePropertyURL, TestNormalizePropertyNoURL, TestNormalizeReservationURLProduction, TestNormalizeReservationURLTest, TestFormatRatingWhole, TestFormatRatingFractional, TestFormatRatingZero, TestNormalizeReviewFractionalRating

### Validation Evidence

**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `./smackerel.sh test unit`, `./smackerel.sh lint`, `./smackerel.sh check`

```
$ ./smackerel.sh lint
All checks passed

$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh format --check
(exit 0 — no formatting issues)
```

### Audit Evidence

**Phase Agent:** bubbles.audit
**Executed:** YES
**Command:** `./smackerel.sh check`, `./smackerel.sh lint`

Code quality review of `internal/connector/hospitable/`:

- **Pattern compliance:** Follows existing connector patterns (Keep, Maps, Browser) — implements `connector.Connector` interface (ID, Connect, Sync, Health, Close)
- **Config SST:** All config values sourced from `config/smackerel.yaml` → `scripts/commands/config.sh` → `config/generated/dev.env`. No hardcoded ports, URLs, or fallback defaults
- **NATS contract:** No modifications to existing NATS streams or subjects
- **Database:** Uses existing `artifacts`, `edges`, `sync_state` tables — no new migrations
- **Security:** PAT stored in config SST pipeline. Bearer token auth over TLS. No user input in API URL paths without validation. Rate limiting respects API contract

### Chaos Evidence

**Phase Agent:** bubbles.chaos
**Executed:** YES
**Command:** `./smackerel.sh test unit`

Resilience verification from unit tests:

- **Partial failure isolation:** TestPartialFailureReturnsSuccessful — message sync error does not block property/reservation sync, partial artifacts returned, health remains healthy
- **All failures set error:** TestAllFailuresSetHealthError — all resource types returning 500 → zero artifacts, health=error
- **Empty token handling:** TestConnectEmptyToken — empty access_token → clear error "access_token is required", health=error
- **Property name cache miss:** TestNormalizeReservationFallbackPropertyID, TestNormalizeReviewFallbackPropertyID — unknown property ID falls back to raw ID, no crash, no empty title
- **Rate limit exhaustion:** TestClientMaxRetriesOn429 — 3 consecutive 429s → rate limit error returned cleanly
- **Retry-After edge cases:** TestParseRetryAfterEmpty, TestParseRetryAfterInvalid — missing or malformed Retry-After header → falls back to exponential backoff without crash
- **Message cursor isolation:** TestMessageCursorNotAdvancedOnFailure — one reservation message failure does not advance cursor, preventing message loss

---

## Execution Evidence

### Delivery Lockdown Certification

- **Scopes completed:** 5/5 (Scope 01–05)
- **Unit tests:** 52 tests across 2 test files — all pass
- **Lint:** Pass
- **Format:** Pass
- **Check:** Pass
